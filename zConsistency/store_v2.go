package zConsistency

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidConsistencyContext    = errors.New("zConsistency: invalid context")
	ErrInvalidConsistencyRequestID  = errors.New("zConsistency: invalid request id")
	ErrConsistencyEntryExists       = errors.New("zConsistency: entry already exists")
	ErrConsistencyEntryNotFound     = errors.New("zConsistency: entry not found")
	ErrInvalidConsistencyTransition = errors.New("zConsistency: invalid state transition")
	ErrConsistencyStoreUnavailable  = errors.New("zConsistency: store unavailable")
	ErrUnsupportedConsistencyStore  = errors.New("zConsistency: operation unsupported by legacy store")
)

// OutboxState separates durable enqueue, transport completion and business
// application. A socket write is Transported, never Applied.
type OutboxState uint8

const (
	OutboxStateUnknown OutboxState = iota
	OutboxStateEnqueued
	OutboxStateTransported
	OutboxStateApplied
	OutboxStateDeadLetter
)

func (s OutboxState) String() string {
	switch s {
	case OutboxStateEnqueued:
		return "enqueued"
	case OutboxStateTransported:
		return "transported"
	case OutboxStateApplied:
		return "applied"
	case OutboxStateDeadLetter:
		return "dead-letter"
	default:
		return "unknown"
	}
}

// InboxAcceptResult tells callers whether they own this attempt, another
// attempt is active, or the idempotent business effect was already completed.
type InboxAcceptResult uint8

const (
	InboxAcceptUnknown InboxAcceptResult = iota
	InboxAccepted
	InboxInProgress
	InboxProcessed
)

func (r InboxAcceptResult) String() string {
	switch r {
	case InboxAccepted:
		return "accepted"
	case InboxInProgress:
		return "in-progress"
	case InboxProcessed:
		return "processed"
	default:
		return "unknown"
	}
}

type InboxState uint8

const (
	InboxStateUnknown InboxState = iota
	InboxStateInProgress
	InboxStateProcessed
)

// OutboxStoreV2 is the context/error-aware consistency contract. Complete is
// an explicit retention decision: transport-only callers complete from
// Transported; callers with a business ACK complete from Applied.
//
// Delivery semantics are at-least-once delivery plus an idempotent business
// effect. This interface does not promise transport-level exactly-once.
type OutboxStoreV2 interface {
	Enqueue(context.Context, OutboxMessage) error
	Get(context.Context, uint64) (OutboxMessage, error)
	MarkTransported(context.Context, uint64) error
	MarkApplied(context.Context, uint64) error
	RecordAttempt(context.Context, uint64, error) error
	MoveToDeadLetter(context.Context, uint64, string) error
	Complete(context.Context, uint64, OutboxState) error
	ListUnresolved(context.Context, int) ([]OutboxMessage, error)
	ListRetryableContext(context.Context, time.Time, int) ([]OutboxMessage, error)
	ListDeadLettersContext(context.Context, int) ([]OutboxMessage, error)
	CountUnresolved(context.Context) (int, error)
	CountRetryableContext(context.Context, time.Time) (int, error)
	CountDeadLettersContext(context.Context) (int, error)
	PurgeDeadLettersContext(context.Context, time.Duration) (int, error)
}

// InboxStoreV2 distinguishes admission from completion and propagates durable
// store failures so reliable commands can fail closed.
type InboxStoreV2 interface {
	Acquire(context.Context, uint64) (InboxAcceptResult, error)
	Status(context.Context, uint64) (InboxState, error)
	MarkProcessed(context.Context, uint64) error
	Abandon(context.Context, uint64) error
	CleanupContext(context.Context, time.Duration) (int, error)
}

type ConsistencyTransitionError struct {
	RequestID uint64
	From      OutboxState
	To        OutboxState
}

func (e *ConsistencyTransitionError) Error() string {
	return fmt.Sprintf("%v: request %d: %s -> %s", ErrInvalidConsistencyTransition, e.RequestID, e.From, e.To)
}

func (e *ConsistencyTransitionError) Unwrap() error { return ErrInvalidConsistencyTransition }

func checkConsistencyContext(ctx context.Context) error {
	if ctx == nil {
		return ErrInvalidConsistencyContext
	}
	return ctx.Err()
}

func validateConsistencyRequest(ctx context.Context, requestID uint64) error {
	if err := checkConsistencyContext(ctx); err != nil {
		return err
	}
	if requestID == 0 {
		return ErrInvalidConsistencyRequestID
	}
	return nil
}

func outboxState(message OutboxMessage) OutboxState {
	if message.DeadLetter {
		return OutboxStateDeadLetter
	}
	if message.Acked {
		return OutboxStateApplied
	}
	if message.Sent {
		return OutboxStateTransported
	}
	return OutboxStateEnqueued
}

func setOutboxState(message *OutboxMessage, state OutboxState) {
	message.State = state
	switch state {
	case OutboxStateEnqueued:
		message.Sent = false
		message.Acked = false
		message.DeadLetter = false
	case OutboxStateTransported:
		message.Sent = true
		message.Acked = false
		message.DeadLetter = false
	case OutboxStateApplied:
		message.Sent = true
		message.Acked = true
		message.DeadLetter = false
	case OutboxStateDeadLetter:
		message.Acked = false
		message.DeadLetter = true
	}
}

func cloneOutboxMessage(message OutboxMessage) OutboxMessage {
	message.State = outboxState(message)
	message.Payload = append([]byte(nil), message.Payload...)
	return message
}

func transitionError(requestID uint64, from, to OutboxState) error {
	return &ConsistencyTransitionError{RequestID: requestID, From: from, To: to}
}

func normalizedLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	return limit
}

func retryDelay(base, maximum time.Duration, attempts int) time.Duration {
	if base <= 0 {
		return 0
	}
	delay := base
	for n := 0; n < attempts; n++ {
		if maximum > 0 && delay >= maximum/2 {
			return maximum
		}
		delay *= 2
	}
	if maximum > 0 && delay > maximum {
		return maximum
	}
	return delay
}
