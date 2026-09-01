package zConsistency

import (
	"context"
	"time"
)

// AdaptOutboxStore keeps custom implementations of the original interface
// source-compatible while callers migrate. Built-in stores implement V2
// directly and are returned unchanged. A legacy store cannot persist the
// intermediate Transported/Applied states; Complete maps the caller-selected
// terminal state to the corresponding original deletion method.
func AdaptOutboxStore(store OutboxStore) OutboxStoreV2 {
	if store == nil {
		return nil
	}
	if checked, ok := store.(OutboxStoreV2); ok {
		return checked
	}
	return &legacyOutboxStoreV2{store: store}
}

type legacyOutboxStoreV2 struct {
	store OutboxStore
}

func (a *legacyOutboxStoreV2) Enqueue(ctx context.Context, message OutboxMessage) error {
	if err := validateConsistencyRequest(ctx, message.RequestID); err != nil {
		return err
	}
	a.store.Add(cloneOutboxMessage(message))
	return nil
}

func (a *legacyOutboxStoreV2) Get(ctx context.Context, requestID uint64) (OutboxMessage, error) {
	if err := validateConsistencyRequest(ctx, requestID); err != nil {
		return OutboxMessage{}, err
	}
	return OutboxMessage{}, ErrUnsupportedConsistencyStore
}

func (a *legacyOutboxStoreV2) MarkTransported(ctx context.Context, requestID uint64) error {
	return validateConsistencyRequest(ctx, requestID)
}

func (a *legacyOutboxStoreV2) MarkApplied(ctx context.Context, requestID uint64) error {
	return validateConsistencyRequest(ctx, requestID)
}

func (a *legacyOutboxStoreV2) RecordAttempt(ctx context.Context, requestID uint64, cause error) error {
	if err := validateConsistencyRequest(ctx, requestID); err != nil {
		return err
	}
	a.store.MarkAttempt(requestID, cause)
	return nil
}

func (a *legacyOutboxStoreV2) MoveToDeadLetter(ctx context.Context, requestID uint64, reason string) error {
	if err := validateConsistencyRequest(ctx, requestID); err != nil {
		return err
	}
	a.store.MarkDeadLetter(requestID, reason)
	return nil
}

func (a *legacyOutboxStoreV2) Complete(ctx context.Context, requestID uint64, terminal OutboxState) error {
	if err := validateConsistencyRequest(ctx, requestID); err != nil {
		return err
	}
	switch terminal {
	case OutboxStateTransported:
		a.store.MarkSent(requestID)
	case OutboxStateApplied:
		a.store.MarkAcked(requestID)
	default:
		return transitionError(requestID, OutboxStateUnknown, terminal)
	}
	return nil
}

func (a *legacyOutboxStoreV2) ListUnresolved(ctx context.Context, limit int) ([]OutboxMessage, error) {
	if err := checkConsistencyContext(ctx); err != nil {
		return nil, err
	}
	return cloneOutboxMessages(a.store.ListPending(limit)), nil
}

func (a *legacyOutboxStoreV2) ListRetryableContext(ctx context.Context, now time.Time, limit int) ([]OutboxMessage, error) {
	if err := checkConsistencyContext(ctx); err != nil {
		return nil, err
	}
	return cloneOutboxMessages(a.store.ListRetryable(now, limit)), nil
}

func (a *legacyOutboxStoreV2) ListDeadLettersContext(ctx context.Context, limit int) ([]OutboxMessage, error) {
	if err := checkConsistencyContext(ctx); err != nil {
		return nil, err
	}
	return cloneOutboxMessages(a.store.ListDeadLetters(limit)), nil
}

func (a *legacyOutboxStoreV2) CountUnresolved(ctx context.Context) (int, error) {
	if err := checkConsistencyContext(ctx); err != nil {
		return 0, err
	}
	return a.store.CountPending(), nil
}

func (a *legacyOutboxStoreV2) CountRetryableContext(ctx context.Context, _ time.Time) (int, error) {
	if err := checkConsistencyContext(ctx); err != nil {
		return 0, err
	}
	return a.store.CountRetryable(), nil
}

func (a *legacyOutboxStoreV2) CountDeadLettersContext(ctx context.Context) (int, error) {
	if err := checkConsistencyContext(ctx); err != nil {
		return 0, err
	}
	return a.store.CountDeadLetters(), nil
}

func (a *legacyOutboxStoreV2) PurgeDeadLettersContext(ctx context.Context, olderThan time.Duration) (int, error) {
	if err := checkConsistencyContext(ctx); err != nil {
		return 0, err
	}
	return a.store.PurgeDeadLetters(olderThan), nil
}

// AdaptInboxStore provides source compatibility for custom original stores.
// The original bool API cannot distinguish in-progress from processed; a false
// result is therefore mapped through IsProcessed and may conservatively report
// Processed. Reliable paths that require all three states must provide V2.
func AdaptInboxStore(store InboxStore) InboxStoreV2 {
	if store == nil {
		return nil
	}
	if checked, ok := store.(InboxStoreV2); ok {
		return checked
	}
	return &legacyInboxStoreV2{store: store}
}

type legacyInboxStoreV2 struct {
	store InboxStore
}

func (a *legacyInboxStoreV2) Acquire(ctx context.Context, requestID uint64) (InboxAcceptResult, error) {
	if err := validateConsistencyRequest(ctx, requestID); err != nil {
		return InboxAcceptUnknown, err
	}
	if a.store.TryAccept(requestID) {
		return InboxAccepted, nil
	}
	if a.store.IsProcessed(requestID) {
		return InboxProcessed, nil
	}
	return InboxInProgress, nil
}

func (a *legacyInboxStoreV2) Status(ctx context.Context, requestID uint64) (InboxState, error) {
	if err := validateConsistencyRequest(ctx, requestID); err != nil {
		return InboxStateUnknown, err
	}
	if !a.store.IsProcessed(requestID) {
		return InboxStateUnknown, ErrConsistencyEntryNotFound
	}
	return InboxStateProcessed, nil
}

func (a *legacyInboxStoreV2) MarkProcessed(ctx context.Context, requestID uint64) error {
	if err := validateConsistencyRequest(ctx, requestID); err != nil {
		return err
	}
	if !a.store.Ack(requestID) {
		return ErrConsistencyEntryNotFound
	}
	return nil
}

func (a *legacyInboxStoreV2) Abandon(ctx context.Context, requestID uint64) error {
	if err := validateConsistencyRequest(ctx, requestID); err != nil {
		return err
	}
	a.store.Release(requestID)
	return nil
}

func (a *legacyInboxStoreV2) CleanupContext(ctx context.Context, olderThan time.Duration) (int, error) {
	if err := checkConsistencyContext(ctx); err != nil {
		return 0, err
	}
	a.store.Cleanup(olderThan)
	return 0, nil
}

func cloneOutboxMessages(messages []OutboxMessage) []OutboxMessage {
	result := make([]OutboxMessage, len(messages))
	for index, message := range messages {
		result[index] = cloneOutboxMessage(message)
	}
	return result
}
