package zNet

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrSendQueueFull          = errors.New("network send queue is full")
	ErrSessionClosed          = errors.New("network session is closed")
	ErrContextSendUnsupported = errors.New("session does not support context-aware delivery")
	ErrInvalidDeliveryClass   = errors.New("invalid delivery class")
)

// DefaultSendTimeout 是旧 Send API 和无 deadline context 的有界兼容超时。
const DefaultSendTimeout = 5 * time.Second

// DeliveryClass 定义队列 admission，不定义应用层 ACK、持久化或业务提交成功。
type DeliveryClass uint8

const (
	// LatestFrame 同一 CoalesceKey 只保留最新值；允许覆盖或过载丢弃。
	LatestFrame DeliveryClass = iota + 1
	// BestEffortEvent 只在队列立即有容量时接纳，否则返回 ErrSendQueueFull。
	BestEffortEvent
	// ReliableCommand 等待容量直到 context 取消或 deadline；它只保证不静默丢弃，不等于业务已提交。
	ReliableCommand
)

// SendOptions 选择投递等级。LatestFrame 的 CoalesceKey 应标识同一条可覆盖状态流。
type SendOptions struct {
	Class       DeliveryClass
	CoalesceKey uint64
}

// ContextSession 是兼容 Session 之上的 context/deadline 感知发送能力。
type ContextSession interface {
	Session
	SendContext(ctx context.Context, protoID ProtoIdType, data []byte) error
	SendWithOptions(ctx context.Context, protoID ProtoIdType, data []byte, options SendOptions) error
}

// SendSessionWithOptions 要求真实 session 支持 context-aware delivery；不会用 goroutine 包装潜在永久阻塞的旧 Send。
func SendSessionWithOptions(ctx context.Context, session Session, protoID ProtoIdType, data []byte, options SendOptions) error {
	contextSession, ok := session.(ContextSession)
	if !ok {
		return ErrContextSendUnsupported
	}
	return contextSession.SendWithOptions(ctx, protoID, data, options)
}

type outboundPacket struct {
	packet   *NetPacket
	deadline time.Time
}

type queuedOutbound struct {
	outbound *outboundPacket
	class    DeliveryClass
	key      uint64
}

// outboundQueue 是服务端 session 共用的有界发送队列。它不关闭内部 channel，Close/Enqueue 并发不会 panic。
type outboundQueue struct {
	mu        sync.Mutex
	capacity  int
	items     *list.List
	latest    map[uint64]*list.Element
	changed   chan struct{}
	ready     chan struct{}
	closed    bool
	cause     error
	metrics   NetworkBackpressureMetricsRecorder
	closeOnce sync.Once
}

func newOutboundQueue(capacity int, metrics NetworkBackpressureMetricsRecorder) *outboundQueue {
	if capacity <= 0 {
		capacity = DefaultChanSize
	}
	queue := &outboundQueue{
		capacity: capacity,
		items:    list.New(),
		latest:   make(map[uint64]*list.Element),
		changed:  make(chan struct{}),
		ready:    make(chan struct{}, 1),
		metrics:  metrics,
	}
	if metrics != nil {
		metrics.AddSendQueueCapacity(capacity)
	}
	return queue
}

func (q *outboundQueue) Enqueue(ctx context.Context, outbound *outboundPacket, options SendOptions) error {
	if outbound == nil || outbound.packet == nil {
		return errors.New("network outbound packet is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		q.recordRejected(options.Class)
		return err
	}
	if options.Class < LatestFrame || options.Class > ReliableCommand {
		return fmt.Errorf("%w: %d", ErrInvalidDeliveryClass, options.Class)
	}
	if options.Class == LatestFrame {
		return q.enqueueLatest(outbound, options.CoalesceKey)
	}

	start := time.Now()
	waited := false
	for {
		q.mu.Lock()
		if q.closed {
			err := q.closeErrorLocked()
			q.mu.Unlock()
			q.recordRejected(options.Class)
			return err
		}
		if q.items.Len() < q.capacity {
			q.items.PushBack(&queuedOutbound{outbound: outbound, class: options.Class})
			q.signalReadyLocked()
			q.mu.Unlock()
			q.addDepth(1)
			if waited && q.metrics != nil {
				q.metrics.RecordSendQueueWait(time.Since(start))
			}
			return nil
		}
		if options.Class == BestEffortEvent {
			q.mu.Unlock()
			q.recordRejected(options.Class)
			return ErrSendQueueFull
		}
		changed := q.changed
		q.mu.Unlock()
		waited = true
		select {
		case <-ctx.Done():
			q.recordRejected(options.Class)
			if q.metrics != nil {
				q.metrics.RecordSendQueueWait(time.Since(start))
			}
			return ctx.Err()
		case <-changed:
		}
	}
}

func (q *outboundQueue) enqueueLatest(outbound *outboundPacket, key uint64) error {
	q.mu.Lock()
	if q.closed {
		err := q.closeErrorLocked()
		q.mu.Unlock()
		q.recordRejected(LatestFrame)
		return err
	}
	if existing := q.latest[key]; existing != nil {
		existing.Value.(*queuedOutbound).outbound = outbound
		q.signalReadyLocked()
		q.mu.Unlock()
		q.recordLatestDropped()
		return nil
	}
	if q.items.Len() >= q.capacity {
		// 最新帧不能挤掉可靠命令或尽力事件；若已有旧 latest，则淘汰最老的 latest 槽。
		for element := q.items.Front(); element != nil; element = element.Next() {
			queued := element.Value.(*queuedOutbound)
			if queued.class != LatestFrame {
				continue
			}
			delete(q.latest, queued.key)
			element.Value = &queuedOutbound{outbound: outbound, class: LatestFrame, key: key}
			q.latest[key] = element
			q.signalReadyLocked()
			q.mu.Unlock()
			q.recordLatestDropped()
			return nil
		}
		// 队列全是不可覆盖消息时，latest 允许丢弃当前帧。
		q.mu.Unlock()
		q.recordLatestDropped()
		return nil
	}
	element := q.items.PushBack(&queuedOutbound{outbound: outbound, class: LatestFrame, key: key})
	q.latest[key] = element
	q.signalReadyLocked()
	q.mu.Unlock()
	q.addDepth(1)
	return nil
}

func (q *outboundQueue) Ready() <-chan struct{} {
	return q.ready
}

func (q *outboundQueue) TryDequeue() (*outboundPacket, bool) {
	q.mu.Lock()
	element := q.items.Front()
	if element == nil {
		q.mu.Unlock()
		return nil, false
	}
	queued := element.Value.(*queuedOutbound)
	q.items.Remove(element)
	if queued.class == LatestFrame {
		delete(q.latest, queued.key)
	}
	q.signalChangedLocked()
	if q.items.Len() > 0 {
		q.signalReadyLocked()
	}
	q.mu.Unlock()
	q.addDepth(-1)
	return queued.outbound, true
}

func (q *outboundQueue) Close(cause error) {
	q.closeOnce.Do(func() {
		if cause == nil {
			cause = ErrSessionClosed
		}
		q.mu.Lock()
		q.closed = true
		q.cause = cause
		q.signalChangedLocked()
		q.signalReadyLocked()
		q.mu.Unlock()
		if q.metrics != nil {
			q.metrics.AddSendQueueCapacity(-q.capacity)
		}
	})
}

func (q *outboundQueue) closeErrorLocked() error {
	if q.cause == nil || errors.Is(q.cause, ErrSessionClosed) {
		return ErrSessionClosed
	}
	return fmt.Errorf("%w: %v", ErrSessionClosed, q.cause)
}

func (q *outboundQueue) signalChangedLocked() {
	close(q.changed)
	q.changed = make(chan struct{})
}

func (q *outboundQueue) signalReadyLocked() {
	select {
	case q.ready <- struct{}{}:
	default:
	}
}

func (q *outboundQueue) recordRejected(class DeliveryClass) {
	if q.metrics == nil {
		return
	}
	switch class {
	case BestEffortEvent:
		q.metrics.IncBestEffortEventRejected()
	case ReliableCommand:
		q.metrics.IncReliableCommandRejected()
	case LatestFrame:
		q.metrics.IncLatestFrameDropped()
	}
}

func (q *outboundQueue) recordLatestDropped() {
	if q.metrics != nil {
		q.metrics.IncLatestFrameDropped()
	}
}

func (q *outboundQueue) addDepth(delta int) {
	if q.metrics != nil {
		q.metrics.AddSendQueueDepth(delta)
	}
}

func normalizeSendContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}
	if timeout <= 0 {
		timeout = DefaultSendTimeout
	}
	return context.WithTimeout(ctx, timeout)
}

func outboundDeadline(ctx context.Context) time.Time {
	deadline, _ := ctx.Deadline()
	return deadline
}

// contextWriteGate 串行化客户端直写，并让等待 writer 所有权也受 delivery admission 约束。
type contextWriteGate chan struct{}

func newContextWriteGate() contextWriteGate {
	gate := make(contextWriteGate, 1)
	gate <- struct{}{}
	return gate
}

func (gate contextWriteGate) acquire(ctx context.Context, class DeliveryClass) (bool, error) {
	if class < LatestFrame || class > ReliableCommand {
		return false, fmt.Errorf("%w: %d", ErrInvalidDeliveryClass, class)
	}
	if class == ReliableCommand {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-gate:
			return true, nil
		}
	}
	select {
	case <-gate:
		return true, nil
	default:
		if class == LatestFrame {
			return false, nil
		}
		return false, ErrSendQueueFull
	}
}

func (gate contextWriteGate) release() {
	gate <- struct{}{}
}
