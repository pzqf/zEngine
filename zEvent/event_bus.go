package zEvent

import (
	"sync"
	"sync/atomic"

	"github.com/pzqf/zEngine/zLog"
	"github.com/pzqf/zUtil/zConcurrency"
	"go.uber.org/zap"
)

type EventHandler func(event *Event)

// SubscriptionID 是一次订阅的唯一句柄，由 Subscribe 返回，用于 Unsubscribe。
// 用 ID 而非直接比较 EventHandler，是因为 Go 函数值不可比较，无法按 handler 退订。
type SubscriptionID uint64

type subscription struct {
	id SubscriptionID
	fn EventHandler
}

type EventBus struct {
	handlers   map[EventType][]subscription
	nextID     atomic.Uint64
	mu         sync.RWMutex
	running    atomic.Bool
	logger     *zap.Logger
	workerPool *zConcurrency.WorkerPool
}

var globalEventBus *EventBus
var once sync.Once

func GetGlobalEventBus() *EventBus {
	once.Do(func() {
		globalEventBus = NewEventBus()
	})
	return globalEventBus
}

func NewEventBus() *EventBus {
	bus := &EventBus{
		handlers: make(map[EventType][]subscription),
		logger:   zLog.GetLogger(),
	}
	bus.running.Store(true)

	pool := zConcurrency.NewWorkerPool(4, 1024)
	pool.Start()
	bus.workerPool = pool

	bus.logger.Info("EventBus initialized")
	return bus
}

func NewEventBusWithPool(workers int, queueSize int) *EventBus {
	bus := &EventBus{
		handlers: make(map[EventType][]subscription),
		logger:   zLog.GetLogger(),
	}
	bus.running.Store(true)

	pool := zConcurrency.NewWorkerPool(workers, queueSize)
	pool.Start()
	bus.workerPool = pool

	bus.logger.Info("EventBus initialized with custom pool",
		zap.Int("workers", workers),
		zap.Int("queueSize", queueSize))
	return bus
}

// Subscribe 订阅事件，返回可用于 Unsubscribe 的订阅句柄。
// 返回值可忽略（若无需退订）。
func (eb *EventBus) Subscribe(eventType EventType, handler EventHandler) SubscriptionID {
	if !eb.running.Load() {
		return 0
	}

	id := SubscriptionID(eb.nextID.Add(1))

	eb.mu.Lock()
	defer eb.mu.Unlock()

	eb.handlers[eventType] = append(eb.handlers[eventType], subscription{id: id, fn: handler})
	eb.logger.Debug("Subscribed to event",
		zap.Int("eventType", int(eventType)),
		zap.Uint64("subscriptionID", uint64(id)),
		zap.Int("handlerCount", len(eb.handlers[eventType])))
	return id
}

// Unsubscribe 按订阅句柄退订，返回是否命中并移除。
func (eb *EventBus) Unsubscribe(id SubscriptionID) bool {
	if !eb.running.Load() || id == 0 {
		return false
	}

	eb.mu.Lock()
	defer eb.mu.Unlock()

	for eventType, subs := range eb.handlers {
		for i, s := range subs {
			if s.id == id {
				eb.handlers[eventType] = append(subs[:i], subs[i+1:]...)
				if len(eb.handlers[eventType]) == 0 {
					delete(eb.handlers, eventType)
				}
				eb.logger.Debug("Unsubscribed from event",
					zap.Int("eventType", int(eventType)),
					zap.Uint64("subscriptionID", uint64(id)))
				return true
			}
		}
	}
	return false
}

// snapshotHandlers 在读锁下拷贝某事件类型的 handler 列表，
// 避免 Publish 迭代期间被并发 Unsubscribe 修改底层数组。
func (eb *EventBus) snapshotHandlers(eventType EventType) []EventHandler {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	subs := eb.handlers[eventType]
	if len(subs) == 0 {
		return nil
	}
	fns := make([]EventHandler, len(subs))
	for i, s := range subs {
		fns[i] = s.fn
	}
	return fns
}

func (eb *EventBus) Publish(event *Event) {
	if !eb.running.Load() {
		return
	}

	fns := eb.snapshotHandlers(event.Type)
	if len(fns) == 0 {
		return
	}

	for _, handler := range fns {
		h := handler
		e := event
		err := eb.workerPool.Submit(func() error {
			defer func() {
				if r := recover(); r != nil {
					eb.logger.Error("Panic in event handler",
						zap.Int("eventType", int(e.Type)),
						zap.Any("recover", r))
				}
			}()
			h(e)
			return nil
		})
		if err != nil {
			eb.logger.Error("Failed to submit event handler to worker pool",
				zap.Int("eventType", int(event.Type)),
				zap.Error(err))
		}
	}
}

func (eb *EventBus) PublishSync(event *Event) {
	if !eb.running.Load() {
		return
	}

	fns := eb.snapshotHandlers(event.Type)
	if len(fns) == 0 {
		return
	}

	for _, handler := range fns {
		func() {
			defer func() {
				if r := recover(); r != nil {
					eb.logger.Error("Panic in event handler",
						zap.Int("eventType", int(event.Type)),
						zap.Any("recover", r))
				}
			}()
			handler(event)
		}()
	}
}

func (eb *EventBus) Close() {
	if !eb.running.CompareAndSwap(true, false) {
		return
	}

	if eb.workerPool != nil {
		eb.workerPool.Stop()
	}

	eb.mu.Lock()
	defer eb.mu.Unlock()

	for eventType := range eb.handlers {
		delete(eb.handlers, eventType)
	}

	eb.logger.Info("EventBus closed")
}

func (eb *EventBus) GetHandlerCount(eventType EventType) int {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	if handlers, exists := eb.handlers[eventType]; exists {
		return len(handlers)
	}
	return 0
}

func (eb *EventBus) HasSubscribers(eventType EventType) bool {
	return eb.GetHandlerCount(eventType) > 0
}
