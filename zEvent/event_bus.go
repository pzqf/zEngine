package zEvent

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/pzqf/zEngine/zLog"
	"github.com/pzqf/zUtil/zConcurrency"
	"go.uber.org/zap"
)

var (
	ErrEventBusClosed              = errors.New("event bus is closed")
	ErrEventBusInvalidEvent        = errors.New("event bus event is nil")
	ErrEventBusInvalidHandler      = errors.New("event bus handler is nil")
	ErrEventBusInvalidSubscription = errors.New("event bus subscription is invalid")
	ErrEventBusNoExecutor          = errors.New("event bus has no async executor")
	ErrEventBusFull                = errors.New("event bus executor queue is full")
	ErrEventBusExecutorUnavailable = errors.New("event bus executor is unavailable")
	ErrEventBusExecutorRejected    = errors.New("event bus executor rejected task")
)

type EventHandler func(event *Event)

// AsyncExecutor accepts already-snapshotted event work. The injector owns its lifecycle.
type AsyncExecutor interface {
	Submit(task zConcurrency.Task) error
}

type stoppableExecutor interface {
	Stop()
}

// SubscriptionID is a stable handle returned by SubscribeChecked and Subscribe.
type SubscriptionID uint64

type subscription struct {
	id SubscriptionID
	fn EventHandler
}

type eventBusState uint8

const (
	eventBusOpen eventBusState = iota
	eventBusClosing
	eventBusClosed
)

type EventBus struct {
	mu                  sync.Mutex
	stateChanged        *sync.Cond
	state               eventBusState
	handlers            map[EventType][]subscription
	nextID              uint64
	inFlightSubmissions int
	logger              *zap.Logger
	executor            AsyncExecutor
	ownedExecutor       stoppableExecutor
	handlerPanics       atomic.Uint64
}

var (
	globalEventBusMu sync.Mutex
	globalEventBus   *EventBus
)

// GetGlobalEventBus lazily creates a synchronous-only process bus. A process that needs
// asynchronous handlers must install a root-owned executor with RebuildGlobalEventBus.
func GetGlobalEventBus() *EventBus {
	globalEventBusMu.Lock()
	defer globalEventBusMu.Unlock()
	if globalEventBus == nil {
		globalEventBus = NewEventBus()
	}
	return globalEventBus
}

// RebuildGlobalEventBus installs a fresh process bus and closes the previous bus. The caller
// retains ownership of executor and must stop it after CloseGlobalEventBus during shutdown.
func RebuildGlobalEventBus(executor AsyncExecutor) *EventBus {
	replacement := NewEventBusWithExecutor(executor)
	globalEventBusMu.Lock()
	previous := globalEventBus
	globalEventBus = replacement
	globalEventBusMu.Unlock()
	if previous != nil {
		previous.Close()
	}
	return replacement
}

// CloseGlobalEventBus detaches and closes the process bus. A later GetGlobalEventBus call
// creates a fresh usable bus instead of returning a permanently closed singleton.
func CloseGlobalEventBus() {
	globalEventBusMu.Lock()
	bus := globalEventBus
	globalEventBus = nil
	globalEventBusMu.Unlock()
	if bus != nil {
		bus.Close()
	}
}

// NewEventBus creates only a subscription table and starts no goroutines.
func NewEventBus() *EventBus {
	return newEventBus(nil, nil)
}

// NewEventBusWithExecutor creates a bus that borrows an explicitly managed async executor.
func NewEventBusWithExecutor(executor AsyncExecutor) *EventBus {
	return newEventBus(executor, nil)
}

// NewEventBusWithPool preserves the old convenience API. The returned bus owns this pool and
// drains/stops it from Close. New production code should inject a process-owned executor.
// Deprecated: use NewEventBusWithExecutor with a root-owned executor.
func NewEventBusWithPool(workers int, queueSize int) *EventBus {
	pool := zConcurrency.NewWorkerPool(workers, queueSize)
	pool.Start()
	return newEventBus(pool, pool)
}

func newEventBus(executor AsyncExecutor, owned stoppableExecutor) *EventBus {
	bus := &EventBus{
		state:         eventBusOpen,
		handlers:      make(map[EventType][]subscription),
		logger:        zLog.GetLogger(),
		executor:      executor,
		ownedExecutor: owned,
	}
	bus.stateChanged = sync.NewCond(&bus.mu)
	return bus
}

func (eb *EventBus) initializeLocked() {
	if eb.stateChanged == nil {
		eb.stateChanged = sync.NewCond(&eb.mu)
	}
	if eb.handlers == nil {
		eb.handlers = make(map[EventType][]subscription)
	}
	if eb.logger == nil {
		eb.logger = zLog.GetLogger()
	}
}

func (eb *EventBus) eventLogger() *zap.Logger {
	eb.mu.Lock()
	eb.initializeLocked()
	logger := eb.logger
	eb.mu.Unlock()
	return logger
}

// SubscribeChecked atomically rejects nil handlers and subscriptions after closing begins.
func (eb *EventBus) SubscribeChecked(eventType EventType, handler EventHandler) (SubscriptionID, error) {
	if handler == nil {
		return 0, ErrEventBusInvalidHandler
	}
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.initializeLocked()
	if eb.state != eventBusOpen {
		return 0, ErrEventBusClosed
	}
	eb.nextID++
	id := SubscriptionID(eb.nextID)
	eb.handlers[eventType] = append(eb.handlers[eventType], subscription{id: id, fn: handler})
	return id, nil
}

// Subscribe is the compatibility wrapper for callers that cannot yet handle admission errors.
// Deprecated: use SubscribeChecked.
func (eb *EventBus) Subscribe(eventType EventType, handler EventHandler) SubscriptionID {
	id, err := eb.SubscribeChecked(eventType, handler)
	if err != nil {
		eb.eventLogger().Debug("Event subscription rejected", zap.Int("eventType", int(eventType)), zap.Error(err))
	}
	return id
}

// UnsubscribeChecked removes exactly one subscription while the bus is open.
func (eb *EventBus) UnsubscribeChecked(id SubscriptionID) (bool, error) {
	if id == 0 {
		return false, ErrEventBusInvalidSubscription
	}
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.initializeLocked()
	if eb.state != eventBusOpen {
		return false, ErrEventBusClosed
	}
	for eventType, subs := range eb.handlers {
		for i, current := range subs {
			if current.id != id {
				continue
			}
			eb.handlers[eventType] = append(subs[:i], subs[i+1:]...)
			if len(eb.handlers[eventType]) == 0 {
				delete(eb.handlers, eventType)
			}
			return true, nil
		}
	}
	return false, nil
}

// Unsubscribe is the compatibility wrapper for callers that cannot yet handle admission errors.
// Deprecated: use UnsubscribeChecked.
func (eb *EventBus) Unsubscribe(id SubscriptionID) bool {
	removed, _ := eb.UnsubscribeChecked(id)
	return removed
}

// PublishChecked submits one task per event. Handlers in its snapshot run sequentially in
// subscription order; executor capacity and scheduling remain properties of the injected scope.
func (eb *EventBus) PublishChecked(event *Event) error {
	if event == nil {
		return ErrEventBusInvalidEvent
	}
	handlers, executor, err := eb.beginAsyncPublish(event.Type)
	if err != nil || len(handlers) == 0 {
		return err
	}
	defer eb.finishAsyncSubmission()
	err = executor.Submit(func() error {
		eb.runHandlers(event, handlers)
		return nil
	})
	return mapExecutorError(err)
}

// Publish is the compatibility wrapper that preserves the original fire-and-forget signature.
// Deprecated: use PublishChecked when rejection matters.
func (eb *EventBus) Publish(event *Event) {
	if err := eb.PublishChecked(event); err != nil {
		eb.eventLogger().Debug("Event publish rejected", zap.Error(err))
	}
}

// PublishSyncChecked snapshots admission atomically and invokes handlers outside the bus lock.
func (eb *EventBus) PublishSyncChecked(event *Event) error {
	if event == nil {
		return ErrEventBusInvalidEvent
	}
	eb.mu.Lock()
	eb.initializeLocked()
	if eb.state != eventBusOpen {
		eb.mu.Unlock()
		return ErrEventBusClosed
	}
	handlers := eb.snapshotHandlersLocked(event.Type)
	eb.mu.Unlock()
	eb.runHandlers(event, handlers)
	return nil
}

// PublishSync is the compatibility wrapper for the original synchronous API.
// Deprecated: use PublishSyncChecked when rejection matters.
func (eb *EventBus) PublishSync(event *Event) {
	if err := eb.PublishSyncChecked(event); err != nil {
		eb.eventLogger().Debug("Synchronous event publish rejected", zap.Error(err))
	}
}

func (eb *EventBus) beginAsyncPublish(eventType EventType) ([]EventHandler, AsyncExecutor, error) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.initializeLocked()
	if eb.state != eventBusOpen {
		return nil, nil, ErrEventBusClosed
	}
	handlers := eb.snapshotHandlersLocked(eventType)
	if len(handlers) == 0 {
		return nil, nil, nil
	}
	if eb.executor == nil {
		return nil, nil, ErrEventBusNoExecutor
	}
	eb.inFlightSubmissions++
	return handlers, eb.executor, nil
}

func (eb *EventBus) finishAsyncSubmission() {
	eb.mu.Lock()
	eb.inFlightSubmissions--
	if eb.inFlightSubmissions == 0 {
		eb.stateChanged.Broadcast()
	}
	eb.mu.Unlock()
}

func (eb *EventBus) snapshotHandlersLocked(eventType EventType) []EventHandler {
	subs := eb.handlers[eventType]
	if len(subs) == 0 {
		return nil
	}
	handlers := make([]EventHandler, len(subs))
	for i, current := range subs {
		handlers[i] = current.fn
	}
	return handlers
}

func (eb *EventBus) runHandlers(event *Event, handlers []EventHandler) {
	for _, handler := range handlers {
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					eb.handlerPanics.Add(1)
					eb.eventLogger().Error("Panic in event handler",
						zap.Int("eventType", int(event.Type)),
						zap.Any("recover", recovered))
				}
			}()
			handler(event)
		}()
	}
}

func mapExecutorError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, zConcurrency.ErrWorkerPoolFull):
		return errors.Join(ErrEventBusFull, err)
	case errors.Is(err, zConcurrency.ErrWorkerPoolClosed), errors.Is(err, zConcurrency.ErrWorkerPoolNotRunning):
		return errors.Join(ErrEventBusExecutorUnavailable, err)
	default:
		return errors.Join(ErrEventBusExecutorRejected, err)
	}
}

// Close atomically rejects new operations, waits only for in-flight executor submissions, then
// clears subscriptions. Borrowed executors remain running; the compatibility pool is drained.
func (eb *EventBus) Close() {
	eb.mu.Lock()
	eb.initializeLocked()
	switch eb.state {
	case eventBusClosing:
		for eb.state != eventBusClosed {
			eb.stateChanged.Wait()
		}
		eb.mu.Unlock()
		return
	case eventBusClosed:
		eb.mu.Unlock()
		return
	}
	eb.state = eventBusClosing
	for eb.inFlightSubmissions > 0 {
		eb.stateChanged.Wait()
	}
	clear(eb.handlers)
	owned := eb.ownedExecutor
	eb.mu.Unlock()

	if owned != nil {
		owned.Stop()
	}

	eb.mu.Lock()
	eb.state = eventBusClosed
	eb.stateChanged.Broadcast()
	eb.mu.Unlock()
}

func (eb *EventBus) GetHandlerCount(eventType EventType) int {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.initializeLocked()
	return len(eb.handlers[eventType])
}

func (eb *EventBus) HasSubscribers(eventType EventType) bool {
	return eb.GetHandlerCount(eventType) > 0
}

// HandlerPanicCount returns the number of isolated handler panics observed by this bus.
func (eb *EventBus) HandlerPanicCount() uint64 {
	return eb.handlerPanics.Load()
}
