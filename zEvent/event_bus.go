package zEvent

import (
	"sync"
	"sync/atomic"

	"github.com/pzqf/zEngine/zLog"
	"github.com/pzqf/zUtil/zConcurrency"
	"go.uber.org/zap"
)

type EventHandler func(event *Event)

type EventBus struct {
	handlers   map[EventType][]EventHandler
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
		handlers: make(map[EventType][]EventHandler),
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
		handlers: make(map[EventType][]EventHandler),
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

func (eb *EventBus) Subscribe(eventType EventType, handler EventHandler) {
	if !eb.running.Load() {
		return
	}

	eb.mu.Lock()
	defer eb.mu.Unlock()

	if _, exists := eb.handlers[eventType]; !exists {
		eb.handlers[eventType] = make([]EventHandler, 0)
	}

	eb.handlers[eventType] = append(eb.handlers[eventType], handler)
	eb.logger.Debug("Subscribed to event",
		zap.Int("eventType", int(eventType)),
		zap.Int("handlerCount", len(eb.handlers[eventType])))
}

func (eb *EventBus) Unsubscribe(eventType EventType, handler EventHandler) {
	if !eb.running.Load() {
		return
	}

	eb.logger.Warn("Unsubscribe method is not fully implemented due to Go language limitations")
}

func (eb *EventBus) Publish(event *Event) {
	if !eb.running.Load() {
		return
	}

	eb.mu.RLock()
	handlers, exists := eb.handlers[event.Type]
	eb.mu.RUnlock()

	if !exists || len(handlers) == 0 {
		return
	}

	for _, handler := range handlers {
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

	eb.mu.RLock()
	handlers, exists := eb.handlers[event.Type]
	eb.mu.RUnlock()

	if !exists || len(handlers) == 0 {
		return
	}

	for _, handler := range handlers {
		defer func() {
			if r := recover(); r != nil {
				eb.logger.Error("Panic in event handler",
					zap.Int("eventType", int(event.Type)),
					zap.Any("recover", r))
			}
		}()
		handler(event)
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
