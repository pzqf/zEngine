package zConsistency

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/pzqf/zEngine/zLog"
	"github.com/pzqf/zUtil/zMap"
	"go.uber.org/zap"
)

type OutboxMessage struct {
	RequestID      uint64
	Topic          string
	TargetServerID string
	TargetMapID    int32
	ProtoID        int32
	Payload        []byte
	Sent           bool
	Acked          bool
	Attempts       int
	LastError      string
	DeadLetter     bool
	CreatedAt      time.Time
	LastAttemptAt  time.Time
	NextRetryAt    time.Time
	// State is populated by OutboxStoreV2. Sent/Acked/DeadLetter remain for
	// source compatibility and are kept in sync by the built-in stores.
	State OutboxState
}

type OutboxStore interface {
	Add(msg OutboxMessage)
	MarkSent(requestID uint64)
	MarkAcked(requestID uint64)
	MarkAttempt(requestID uint64, err error)
	MarkDeadLetter(requestID uint64, reason string)
	ListPending(limit int) []OutboxMessage
	ListRetryable(now time.Time, limit int) []OutboxMessage
	ListDeadLetters(limit int) []OutboxMessage
	CountPending() int
	CountRetryable() int
	CountDeadLetters() int
	PurgeDeadLetters(olderThan time.Duration) int
}

type InboxStore interface {
	TryAccept(requestID uint64) bool
	Ack(requestID uint64) bool
	IsProcessed(requestID uint64) bool
	// Release 撤销一次 TryAccept 的占用（INF-2）。当处理在 TryAccept 之后失败、需要允许后续
	// 重投重新处理时调用；否则失败请求会被永久判为"重复"、重投误返成功。
	Release(requestID uint64)
	Cleanup(olderThan time.Duration)
}

type MemoryOutbox struct {
	v2Mu          sync.RWMutex
	messages      *zMap.TypedMap[uint64, OutboxMessage]
	maxRetries    int
	retryBackoff  time.Duration
	maxRetryDelay time.Duration
}

type OutboxOption func(*MemoryOutbox)

func WithMaxRetries(n int) OutboxOption {
	return func(o *MemoryOutbox) {
		o.maxRetries = n
	}
}

func WithRetryBackoff(d time.Duration) OutboxOption {
	return func(o *MemoryOutbox) {
		o.retryBackoff = d
	}
}

func WithMaxRetryDelay(d time.Duration) OutboxOption {
	return func(o *MemoryOutbox) {
		o.maxRetryDelay = d
	}
}

func NewMemoryOutbox(opts ...OutboxOption) *MemoryOutbox {
	o := &MemoryOutbox{
		messages:      zMap.NewTypedMap[uint64, OutboxMessage](),
		maxRetries:    5,
		retryBackoff:  500 * time.Millisecond,
		maxRetryDelay: 30 * time.Second,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

func (m *MemoryOutbox) Add(msg OutboxMessage) {
	m.v2Mu.Lock()
	defer m.v2Mu.Unlock()
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}
	if msg.NextRetryAt.IsZero() {
		msg.NextRetryAt = msg.CreatedAt
	}
	m.messages.Store(msg.RequestID, cloneOutboxMessage(msg))
}

// MarkSent 标记已发送。GS-1：直接删除条目而非仅置标志——本系统跨服发送为 fire-and-forget
// 无 ack 回执（MarkAcked 无任何调用方），"已发送"即终态，保留会导致 outbox map 随每次
// move/attack 无界增长最终 OOM。发送失败走 MarkAttempt（保留待重试），不受影响。
func (m *MemoryOutbox) MarkSent(requestID uint64) {
	m.v2Mu.Lock()
	defer m.v2Mu.Unlock()
	m.messages.Delete(requestID)
}

// MarkAcked 收到确认（当前无调用方；语义上确认送达即可删除）。
func (m *MemoryOutbox) MarkAcked(requestID uint64) {
	m.v2Mu.Lock()
	defer m.v2Mu.Unlock()
	m.messages.Delete(requestID)
}

func (m *MemoryOutbox) MarkAttempt(requestID uint64, err error) {
	m.v2Mu.Lock()
	defer m.v2Mu.Unlock()
	if msg, ok := m.messages.Load(requestID); ok {
		msg.Attempts++
		msg.LastAttemptAt = time.Now()
		if err != nil {
			msg.LastError = err.Error()
			msg.Sent = false
			delay := m.retryBackoff * time.Duration(1<<uint(msg.Attempts-1))
			if delay > m.maxRetryDelay {
				delay = m.maxRetryDelay
			}
			msg.NextRetryAt = time.Now().Add(delay)
		}
		m.messages.Store(requestID, msg)
	}
}

func (m *MemoryOutbox) MarkDeadLetter(requestID uint64, reason string) {
	m.v2Mu.Lock()
	defer m.v2Mu.Unlock()
	if msg, ok := m.messages.Load(requestID); ok {
		msg.DeadLetter = true
		msg.LastError = reason
		msg.LastAttemptAt = time.Now()
		m.messages.Store(requestID, msg)
		zLog.Error("Outbox message moved to dead-letter",
			zap.Uint64("request_id", requestID),
			zap.Int("attempts", msg.Attempts),
			zap.String("reason", reason))
	}
}

func (m *MemoryOutbox) ListPending(limit int) []OutboxMessage {
	m.v2Mu.RLock()
	defer m.v2Mu.RUnlock()
	if limit <= 0 {
		limit = 100
	}
	out := make([]OutboxMessage, 0, limit)
	m.messages.Range(func(id uint64, msg OutboxMessage) bool {
		if msg.Sent || msg.DeadLetter {
			return true
		}
		out = append(out, cloneOutboxMessage(msg))
		return len(out) < limit
	})
	return out
}

func (m *MemoryOutbox) ListRetryable(now time.Time, limit int) []OutboxMessage {
	m.v2Mu.RLock()
	defer m.v2Mu.RUnlock()
	if limit <= 0 {
		limit = 100
	}
	out := make([]OutboxMessage, 0, limit)
	m.messages.Range(func(id uint64, msg OutboxMessage) bool {
		if msg.Sent || msg.DeadLetter || msg.Acked {
			return true
		}
		if msg.Attempts >= m.maxRetries {
			return true
		}
		if !msg.NextRetryAt.IsZero() && now.Before(msg.NextRetryAt) {
			return true
		}
		out = append(out, cloneOutboxMessage(msg))
		return len(out) < limit
	})
	return out
}

func (m *MemoryOutbox) ListDeadLetters(limit int) []OutboxMessage {
	m.v2Mu.RLock()
	defer m.v2Mu.RUnlock()
	if limit <= 0 {
		limit = 100
	}
	out := make([]OutboxMessage, 0, limit)
	m.messages.Range(func(id uint64, msg OutboxMessage) bool {
		if !msg.DeadLetter {
			return true
		}
		out = append(out, cloneOutboxMessage(msg))
		return len(out) < limit
	})
	return out
}

func (m *MemoryOutbox) CountPending() int {
	m.v2Mu.RLock()
	defer m.v2Mu.RUnlock()
	count := 0
	m.messages.Range(func(id uint64, msg OutboxMessage) bool {
		if !msg.Sent && !msg.DeadLetter {
			count++
		}
		return true
	})
	return count
}

func (m *MemoryOutbox) CountRetryable() int {
	m.v2Mu.RLock()
	defer m.v2Mu.RUnlock()
	now := time.Now()
	count := 0
	m.messages.Range(func(id uint64, msg OutboxMessage) bool {
		if msg.Sent || msg.DeadLetter || msg.Acked {
			return true
		}
		if msg.Attempts >= m.maxRetries {
			return true
		}
		if !msg.NextRetryAt.IsZero() && now.Before(msg.NextRetryAt) {
			return true
		}
		count++
		return true
	})
	return count
}

func (m *MemoryOutbox) CountDeadLetters() int {
	m.v2Mu.RLock()
	defer m.v2Mu.RUnlock()
	count := 0
	m.messages.Range(func(id uint64, msg OutboxMessage) bool {
		if msg.DeadLetter {
			count++
		}
		return true
	})
	return count
}

func (m *MemoryOutbox) PurgeDeadLetters(olderThan time.Duration) int {
	m.v2Mu.Lock()
	defer m.v2Mu.Unlock()
	now := time.Now()
	removed := 0
	m.messages.Range(func(requestID uint64, msg OutboxMessage) bool {
		if !msg.DeadLetter {
			return true
		}
		ageBase := msg.LastAttemptAt
		if ageBase.IsZero() {
			ageBase = msg.CreatedAt
		}
		if olderThan <= 0 || now.Sub(ageBase) >= olderThan {
			m.messages.Delete(requestID)
			removed++
		}
		return true
	})
	return removed
}

type inboxEntry struct {
	processedAt time.Time
	acked       bool
}

type MemoryInbox struct {
	v2Mu    sync.Mutex
	entries *zMap.TypedMap[uint64, inboxEntry]
	count   atomic.Int64
}

func NewMemoryInbox() *MemoryInbox {
	return &MemoryInbox{
		entries: zMap.NewTypedMap[uint64, inboxEntry](),
	}
}

func (m *MemoryInbox) TryAccept(requestID uint64) bool {
	if requestID == 0 {
		return true
	}
	m.v2Mu.Lock()
	defer m.v2Mu.Unlock()

	// INF-5：用原子 LoadOrStore 做幂等接受。此前 Load-then-Store 非原子——两个并发同
	// requestID 的调用可同时判"不存在"、同时 Store、同时返回 true → 去重失效、消息被处理两次。
	_, loaded := m.entries.LoadOrStore(requestID, inboxEntry{
		processedAt: time.Now(),
		acked:       false,
	})
	if loaded {
		return false // 已存在 = 重复
	}
	m.count.Add(1)
	return true
}

func (m *MemoryInbox) Ack(requestID uint64) bool {
	m.v2Mu.Lock()
	defer m.v2Mu.Unlock()
	if entry, ok := m.entries.Load(requestID); ok {
		entry.acked = true
		m.entries.Store(requestID, entry)
		return true
	}
	return false
}

func (m *MemoryInbox) IsProcessed(requestID uint64) bool {
	m.v2Mu.Lock()
	defer m.v2Mu.Unlock()
	_, exists := m.entries.Load(requestID)
	return exists
}

// Release 删除去重记录，使同 requestID 可被后续 TryAccept 重新接受（INF-2 失败回滚用）。
func (m *MemoryInbox) Release(requestID uint64) {
	if requestID == 0 {
		return
	}
	m.v2Mu.Lock()
	defer m.v2Mu.Unlock()
	if _, ok := m.entries.Load(requestID); ok {
		m.entries.Delete(requestID)
		m.count.Add(-1)
	}
}

func (m *MemoryInbox) Cleanup(olderThan time.Duration) {
	m.v2Mu.Lock()
	defer m.v2Mu.Unlock()
	now := time.Now()
	var toDelete []uint64
	m.entries.Range(func(id uint64, entry inboxEntry) bool {
		if now.Sub(entry.processedAt) > olderThan {
			toDelete = append(toDelete, id)
		}
		return true
	})
	for _, id := range toDelete {
		m.entries.Delete(id)
	}
	m.count.Add(-int64(len(toDelete)))
}

type OperationLog struct {
	entries *zMap.TypedMap[uint64, LogEntry]
}

type LogEntry struct {
	ID        uint64
	Operation string
	Data      interface{}
	Timestamp time.Time
	Completed bool
}

func NewOperationLog() *OperationLog {
	return &OperationLog{
		entries: zMap.NewTypedMap[uint64, LogEntry](),
	}
}

func (ol *OperationLog) Record(id uint64, operation string, data interface{}) {
	ol.entries.Store(id, LogEntry{
		ID:        id,
		Operation: operation,
		Data:      data,
		Timestamp: time.Now(),
		Completed: false,
	})
}

func (ol *OperationLog) Complete(id uint64) {
	if entry, ok := ol.entries.Load(id); ok {
		entry.Completed = true
		ol.entries.Store(id, entry)
	}
}

func (ol *OperationLog) GetIncomplete() []LogEntry {
	var result []LogEntry
	ol.entries.Range(func(id uint64, entry LogEntry) bool {
		if !entry.Completed {
			result = append(result, entry)
		}
		return true
	})
	return result
}

func (ol *OperationLog) Cleanup(maxAge time.Duration) {
	now := time.Now()
	var toDelete []uint64
	ol.entries.Range(func(id uint64, entry LogEntry) bool {
		if entry.Completed && now.Sub(entry.Timestamp) > maxAge {
			toDelete = append(toDelete, id)
		}
		return true
	})
	for _, id := range toDelete {
		ol.entries.Delete(id)
	}
}
