package zConsistency

import (
	"context"
	"sort"
	"time"
)

func (m *MemoryOutbox) Enqueue(ctx context.Context, message OutboxMessage) error {
	if err := validateConsistencyRequest(ctx, message.RequestID); err != nil {
		return err
	}
	m.v2Mu.Lock()
	defer m.v2Mu.Unlock()
	if _, exists := m.messages.Load(message.RequestID); exists {
		return ErrConsistencyEntryExists
	}
	now := time.Now()
	if message.CreatedAt.IsZero() {
		message.CreatedAt = now
	}
	if message.NextRetryAt.IsZero() {
		// 与 SQL store 一致：新行默认按退避曲线调度下次重投资格，
		// 不与首次投递在 ACK 在途窗口内竞争重发。
		message.NextRetryAt = message.CreatedAt.Add(retryDelay(m.retryBackoff, m.maxRetryDelay, 0))
	}
	setOutboxState(&message, OutboxStateEnqueued)
	m.messages.Store(message.RequestID, cloneOutboxMessage(message))
	return nil
}

func (m *MemoryOutbox) Get(ctx context.Context, requestID uint64) (OutboxMessage, error) {
	if err := validateConsistencyRequest(ctx, requestID); err != nil {
		return OutboxMessage{}, err
	}
	m.v2Mu.RLock()
	defer m.v2Mu.RUnlock()
	message, exists := m.messages.Load(requestID)
	if !exists {
		return OutboxMessage{}, ErrConsistencyEntryNotFound
	}
	return cloneOutboxMessage(message), nil
}

func (m *MemoryOutbox) MarkTransported(ctx context.Context, requestID uint64) error {
	return m.transition(ctx, requestID, OutboxStateTransported)
}

func (m *MemoryOutbox) MarkApplied(ctx context.Context, requestID uint64) error {
	return m.transition(ctx, requestID, OutboxStateApplied)
}

func (m *MemoryOutbox) transition(ctx context.Context, requestID uint64, target OutboxState) error {
	if err := validateConsistencyRequest(ctx, requestID); err != nil {
		return err
	}
	m.v2Mu.Lock()
	defer m.v2Mu.Unlock()
	message, exists := m.messages.Load(requestID)
	if !exists {
		return ErrConsistencyEntryNotFound
	}
	current := outboxState(message)
	if current == target {
		return nil
	}
	allowed := target == OutboxStateTransported && current == OutboxStateEnqueued
	allowed = allowed || target == OutboxStateApplied && current == OutboxStateTransported
	if !allowed {
		return transitionError(requestID, current, target)
	}
	setOutboxState(&message, target)
	m.messages.Store(requestID, message)
	return nil
}

func (m *MemoryOutbox) RecordAttempt(ctx context.Context, requestID uint64, cause error) error {
	if err := validateConsistencyRequest(ctx, requestID); err != nil {
		return err
	}
	m.v2Mu.Lock()
	defer m.v2Mu.Unlock()
	message, exists := m.messages.Load(requestID)
	if !exists {
		return ErrConsistencyEntryNotFound
	}
	state := outboxState(message)
	if state == OutboxStateApplied || state == OutboxStateDeadLetter {
		return transitionError(requestID, state, OutboxStateEnqueued)
	}
	message.Attempts++
	message.LastAttemptAt = time.Now()
	// 成功发送同样调度下次重投资格（与 SQL store 一致）：next_retry_at 不得停留在
	// 入队时刻，否则重投扫描会在 ACK 在途窗口内重复投递。
	message.NextRetryAt = message.LastAttemptAt.Add(retryDelay(m.retryBackoff, m.maxRetryDelay, message.Attempts-1))
	if cause != nil {
		message.LastError = cause.Error()
		setOutboxState(&message, OutboxStateEnqueued)
	}
	m.messages.Store(requestID, message)
	return nil
}

func (m *MemoryOutbox) MoveToDeadLetter(ctx context.Context, requestID uint64, reason string) error {
	if err := validateConsistencyRequest(ctx, requestID); err != nil {
		return err
	}
	m.v2Mu.Lock()
	defer m.v2Mu.Unlock()
	message, exists := m.messages.Load(requestID)
	if !exists {
		return ErrConsistencyEntryNotFound
	}
	state := outboxState(message)
	if state == OutboxStateDeadLetter {
		return nil
	}
	if state == OutboxStateApplied {
		return transitionError(requestID, state, OutboxStateDeadLetter)
	}
	message.LastError = reason
	message.LastAttemptAt = time.Now()
	setOutboxState(&message, OutboxStateDeadLetter)
	m.messages.Store(requestID, message)
	return nil
}

func (m *MemoryOutbox) Complete(ctx context.Context, requestID uint64, terminal OutboxState) error {
	if err := validateConsistencyRequest(ctx, requestID); err != nil {
		return err
	}
	if terminal != OutboxStateTransported && terminal != OutboxStateApplied {
		return transitionError(requestID, OutboxStateUnknown, terminal)
	}
	m.v2Mu.Lock()
	defer m.v2Mu.Unlock()
	message, exists := m.messages.Load(requestID)
	if !exists {
		return ErrConsistencyEntryNotFound
	}
	current := outboxState(message)
	if current != terminal {
		return transitionError(requestID, current, terminal)
	}
	m.messages.Delete(requestID)
	return nil
}

func (m *MemoryOutbox) ListUnresolved(ctx context.Context, limit int) ([]OutboxMessage, error) {
	return m.listV2(ctx, limit, func(message OutboxMessage) bool {
		state := outboxState(message)
		return state == OutboxStateEnqueued || state == OutboxStateTransported
	})
}

func (m *MemoryOutbox) ListRetryableContext(ctx context.Context, now time.Time, limit int) ([]OutboxMessage, error) {
	return m.listV2(ctx, limit, func(message OutboxMessage) bool {
		state := outboxState(message)
		return (state == OutboxStateEnqueued || state == OutboxStateTransported) &&
			message.Attempts < m.maxRetries && (message.NextRetryAt.IsZero() || !now.Before(message.NextRetryAt))
	})
}

func (m *MemoryOutbox) ListDeadLettersContext(ctx context.Context, limit int) ([]OutboxMessage, error) {
	return m.listV2(ctx, limit, func(message OutboxMessage) bool {
		return outboxState(message) == OutboxStateDeadLetter
	})
}

func (m *MemoryOutbox) listV2(ctx context.Context, limit int, include func(OutboxMessage) bool) ([]OutboxMessage, error) {
	if err := checkConsistencyContext(ctx); err != nil {
		return nil, err
	}
	m.v2Mu.RLock()
	defer m.v2Mu.RUnlock()
	result := make([]OutboxMessage, 0, normalizedLimit(limit))
	m.messages.Range(func(_ uint64, message OutboxMessage) bool {
		if include(message) {
			result = append(result, cloneOutboxMessage(message))
		}
		return true
	})
	sort.Slice(result, func(a, b int) bool {
		if result[a].CreatedAt.Equal(result[b].CreatedAt) {
			return result[a].RequestID < result[b].RequestID
		}
		return result[a].CreatedAt.Before(result[b].CreatedAt)
	})
	if len(result) > normalizedLimit(limit) {
		result = result[:normalizedLimit(limit)]
	}
	return result, nil
}

func (m *MemoryOutbox) CountUnresolved(ctx context.Context) (int, error) {
	return m.countV2(ctx, func(message OutboxMessage) bool {
		state := outboxState(message)
		return state == OutboxStateEnqueued || state == OutboxStateTransported
	})
}

func (m *MemoryOutbox) CountRetryableContext(ctx context.Context, now time.Time) (int, error) {
	return m.countV2(ctx, func(message OutboxMessage) bool {
		state := outboxState(message)
		return (state == OutboxStateEnqueued || state == OutboxStateTransported) &&
			message.Attempts < m.maxRetries && (message.NextRetryAt.IsZero() || !now.Before(message.NextRetryAt))
	})
}

func (m *MemoryOutbox) CountDeadLettersContext(ctx context.Context) (int, error) {
	return m.countV2(ctx, func(message OutboxMessage) bool {
		return outboxState(message) == OutboxStateDeadLetter
	})
}

func (m *MemoryOutbox) countV2(ctx context.Context, include func(OutboxMessage) bool) (int, error) {
	if err := checkConsistencyContext(ctx); err != nil {
		return 0, err
	}
	m.v2Mu.RLock()
	defer m.v2Mu.RUnlock()
	count := 0
	m.messages.Range(func(_ uint64, message OutboxMessage) bool {
		if include(message) {
			count++
		}
		return true
	})
	return count, nil
}

func (m *MemoryOutbox) PurgeDeadLettersContext(ctx context.Context, olderThan time.Duration) (int, error) {
	if err := checkConsistencyContext(ctx); err != nil {
		return 0, err
	}
	m.v2Mu.Lock()
	defer m.v2Mu.Unlock()
	now := time.Now()
	var requestIDs []uint64
	m.messages.Range(func(requestID uint64, message OutboxMessage) bool {
		if outboxState(message) != OutboxStateDeadLetter {
			return true
		}
		ageBase := message.LastAttemptAt
		if ageBase.IsZero() {
			ageBase = message.CreatedAt
		}
		if olderThan <= 0 || now.Sub(ageBase) >= olderThan {
			requestIDs = append(requestIDs, requestID)
		}
		return true
	})
	for _, requestID := range requestIDs {
		m.messages.Delete(requestID)
	}
	return len(requestIDs), nil
}

func (m *MemoryInbox) Acquire(ctx context.Context, requestID uint64) (InboxAcceptResult, error) {
	if err := validateConsistencyRequest(ctx, requestID); err != nil {
		return InboxAcceptUnknown, err
	}
	m.v2Mu.Lock()
	defer m.v2Mu.Unlock()
	entry, exists := m.entries.Load(requestID)
	if exists {
		if entry.acked {
			return InboxProcessed, nil
		}
		return InboxInProgress, nil
	}
	m.entries.Store(requestID, inboxEntry{processedAt: time.Now()})
	m.count.Add(1)
	return InboxAccepted, nil
}

func (m *MemoryInbox) Status(ctx context.Context, requestID uint64) (InboxState, error) {
	if err := validateConsistencyRequest(ctx, requestID); err != nil {
		return InboxStateUnknown, err
	}
	m.v2Mu.Lock()
	defer m.v2Mu.Unlock()
	entry, exists := m.entries.Load(requestID)
	if !exists {
		return InboxStateUnknown, ErrConsistencyEntryNotFound
	}
	if entry.acked {
		return InboxStateProcessed, nil
	}
	return InboxStateInProgress, nil
}

func (m *MemoryInbox) MarkProcessed(ctx context.Context, requestID uint64) error {
	if err := validateConsistencyRequest(ctx, requestID); err != nil {
		return err
	}
	m.v2Mu.Lock()
	defer m.v2Mu.Unlock()
	entry, exists := m.entries.Load(requestID)
	if !exists {
		return ErrConsistencyEntryNotFound
	}
	if entry.acked {
		return nil
	}
	entry.acked = true
	entry.processedAt = time.Now()
	m.entries.Store(requestID, entry)
	return nil
}

func (m *MemoryInbox) Abandon(ctx context.Context, requestID uint64) error {
	if err := validateConsistencyRequest(ctx, requestID); err != nil {
		return err
	}
	m.v2Mu.Lock()
	defer m.v2Mu.Unlock()
	entry, exists := m.entries.Load(requestID)
	if !exists {
		return ErrConsistencyEntryNotFound
	}
	if entry.acked {
		return ErrInvalidConsistencyTransition
	}
	m.entries.Delete(requestID)
	m.count.Add(-1)
	return nil
}

func (m *MemoryInbox) CleanupContext(ctx context.Context, olderThan time.Duration) (int, error) {
	if err := checkConsistencyContext(ctx); err != nil {
		return 0, err
	}
	m.v2Mu.Lock()
	defer m.v2Mu.Unlock()
	now := time.Now()
	var requestIDs []uint64
	m.entries.Range(func(requestID uint64, entry inboxEntry) bool {
		if entry.acked && (olderThan <= 0 || now.Sub(entry.processedAt) > olderThan) {
			requestIDs = append(requestIDs, requestID)
		}
		return true
	})
	for _, requestID := range requestIDs {
		m.entries.Delete(requestID)
	}
	m.count.Add(-int64(len(requestIDs)))
	return len(requestIDs), nil
}
