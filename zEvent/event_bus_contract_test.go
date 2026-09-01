package zEvent

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/pzqf/zUtil/zConcurrency"
)

type eventExecutorFunc func(zConcurrency.Task) error

func (f eventExecutorFunc) Submit(task zConcurrency.Task) error {
	return f(task)
}

func TestEventBusDefaultRequiresExplicitAsyncExecutor(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()

	var calls int
	if _, err := bus.SubscribeChecked(evtA, func(*Event) { calls++ }); err != nil {
		t.Fatalf("SubscribeChecked: %v", err)
	}
	if err := bus.PublishChecked(NewEvent(evtA, nil, nil)); !errors.Is(err, ErrEventBusNoExecutor) {
		t.Fatalf("PublishChecked error = %v, want ErrEventBusNoExecutor", err)
	}
	if calls != 0 {
		t.Fatalf("async handler ran without an executor: calls=%d", calls)
	}
	if err := bus.PublishSyncChecked(NewEvent(evtA, nil, nil)); err != nil {
		t.Fatalf("PublishSyncChecked: %v", err)
	}
	if calls != 1 {
		t.Fatalf("sync handler calls=%d, want 1", calls)
	}
}

func TestEventBusBorrowedExecutorRemainsOwnedByCaller(t *testing.T) {
	pool := zConcurrency.NewWorkerPool(1, 4)
	pool.Start()
	defer pool.Stop()

	bus := NewEventBusWithExecutor(pool)
	delivered := make(chan struct{}, 1)
	if _, err := bus.SubscribeChecked(evtA, func(*Event) { delivered <- struct{}{} }); err != nil {
		t.Fatalf("SubscribeChecked: %v", err)
	}
	if err := bus.PublishChecked(NewEvent(evtA, nil, nil)); err != nil {
		t.Fatalf("PublishChecked: %v", err)
	}
	select {
	case <-delivered:
	case <-time.After(time.Second):
		t.Fatal("borrowed executor did not deliver event")
	}

	bus.Close()
	acceptedAfterBusClose := make(chan struct{}, 1)
	if err := pool.Submit(func() error {
		acceptedAfterBusClose <- struct{}{}
		return nil
	}); err != nil {
		t.Fatalf("EventBus closed a borrowed executor: %v", err)
	}
	select {
	case <-acceptedAfterBusClose:
	case <-time.After(time.Second):
		t.Fatal("borrowed executor stopped after EventBus.Close")
	}
}

func TestEventBusMapsExecutorSaturationToStableError(t *testing.T) {
	bus := NewEventBusWithExecutor(eventExecutorFunc(func(zConcurrency.Task) error {
		return zConcurrency.ErrWorkerPoolFull
	}))
	defer bus.Close()
	if _, err := bus.SubscribeChecked(evtA, func(*Event) {}); err != nil {
		t.Fatalf("SubscribeChecked: %v", err)
	}
	if err := bus.PublishChecked(NewEvent(evtA, nil, nil)); !errors.Is(err, ErrEventBusFull) {
		t.Fatalf("PublishChecked error = %v, want ErrEventBusFull", err)
	}
}

func TestEventBusRealWorkerPoolSaturation(t *testing.T) {
	pool := zConcurrency.NewWorkerPool(1, 1)
	pool.Start()
	defer pool.Stop()
	bus := NewEventBusWithExecutor(pool)
	defer bus.Close()

	handlerStarted := make(chan struct{})
	unblockHandler := make(chan struct{})
	var startOnce sync.Once
	if _, err := bus.SubscribeChecked(evtA, func(*Event) {
		startOnce.Do(func() { close(handlerStarted) })
		<-unblockHandler
	}); err != nil {
		t.Fatalf("SubscribeChecked: %v", err)
	}
	if err := bus.PublishChecked(NewEvent(evtA, nil, 1)); err != nil {
		t.Fatalf("publish running handler: %v", err)
	}
	<-handlerStarted
	if err := bus.PublishChecked(NewEvent(evtA, nil, 2)); err != nil {
		t.Fatalf("publish queued handler: %v", err)
	}
	if err := bus.PublishChecked(NewEvent(evtA, nil, 3)); !errors.Is(err, ErrEventBusFull) {
		t.Fatalf("saturated PublishChecked error = %v, want ErrEventBusFull", err)
	}
	close(unblockHandler)
}

func TestEventBusCloseWaitsForInFlightAdmission(t *testing.T) {
	submitStarted := make(chan struct{})
	unblockSubmit := make(chan struct{})
	var startOnce sync.Once
	bus := NewEventBusWithExecutor(eventExecutorFunc(func(zConcurrency.Task) error {
		startOnce.Do(func() { close(submitStarted) })
		<-unblockSubmit
		return nil
	}))
	if _, err := bus.SubscribeChecked(evtA, func(*Event) {}); err != nil {
		t.Fatalf("SubscribeChecked: %v", err)
	}

	published := make(chan error, 1)
	go func() { published <- bus.PublishChecked(NewEvent(evtA, nil, nil)) }()
	<-submitStarted

	closed := make(chan struct{})
	go func() {
		bus.Close()
		close(closed)
	}()
	select {
	case <-closed:
		t.Fatal("Close returned while an admitted executor submission was in flight")
	case <-time.After(20 * time.Millisecond):
	}
	close(unblockSubmit)
	if err := <-published; err != nil {
		t.Fatalf("in-flight PublishChecked: %v", err)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close did not finish after submission completed")
	}

	if _, err := bus.SubscribeChecked(evtA, func(*Event) {}); !errors.Is(err, ErrEventBusClosed) {
		t.Fatalf("SubscribeChecked after Close error = %v", err)
	}
	if _, err := bus.UnsubscribeChecked(1); !errors.Is(err, ErrEventBusClosed) {
		t.Fatalf("UnsubscribeChecked after Close error = %v", err)
	}
	if err := bus.PublishChecked(NewEvent(evtA, nil, nil)); !errors.Is(err, ErrEventBusClosed) {
		t.Fatalf("PublishChecked after Close error = %v", err)
	}
}

func TestEventBusExecutorPanicDoesNotPoisonClose(t *testing.T) {
	bus := NewEventBusWithExecutor(eventExecutorFunc(func(zConcurrency.Task) error {
		panic("executor panic")
	}))
	if _, err := bus.SubscribeChecked(evtA, func(*Event) {}); err != nil {
		t.Fatalf("SubscribeChecked: %v", err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Error("PublishChecked did not propagate executor panic")
			}
		}()
		_ = bus.PublishChecked(NewEvent(evtA, nil, nil))
	}()

	closed := make(chan struct{})
	go func() {
		bus.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("executor panic left EventBus.Close waiting forever")
	}
}

func TestEventBusHandlersRunInOrderOutsideSubscriptionLock(t *testing.T) {
	bus := NewEventBusWithExecutor(eventExecutorFunc(func(task zConcurrency.Task) error {
		return task()
	}))
	defer bus.Close()

	var order []int
	firstID, err := bus.SubscribeChecked(evtA, func(*Event) {
		order = append(order, 1)
		if _, subscribeErr := bus.SubscribeChecked(EventType(2), func(*Event) {}); subscribeErr != nil {
			t.Errorf("SubscribeChecked from handler: %v", subscribeErr)
		}
		panic("isolated")
	})
	if err != nil {
		t.Fatalf("subscribe first: %v", err)
	}
	if _, err := bus.SubscribeChecked(evtA, func(*Event) {
		order = append(order, 2)
		if removed, unsubscribeErr := bus.UnsubscribeChecked(firstID); unsubscribeErr != nil || !removed {
			t.Errorf("UnsubscribeChecked from handler = %v, %v", removed, unsubscribeErr)
		}
	}); err != nil {
		t.Fatalf("subscribe second: %v", err)
	}

	if err := bus.PublishChecked(NewEvent(evtA, nil, nil)); err != nil {
		t.Fatalf("PublishChecked: %v", err)
	}
	if !reflect.DeepEqual(order, []int{1, 2}) {
		t.Fatalf("handler order = %v, want [1 2]", order)
	}
	if got := bus.HandlerPanicCount(); got != 1 {
		t.Fatalf("HandlerPanicCount = %d, want 1", got)
	}
}

func TestGlobalEventBusCanBeClosedAndRebuilt(t *testing.T) {
	CloseGlobalEventBus()
	first := GetGlobalEventBus()
	CloseGlobalEventBus()
	second := GetGlobalEventBus()
	defer CloseGlobalEventBus()

	if first == second {
		t.Fatal("GetGlobalEventBus returned the permanently closed instance")
	}
	if err := first.PublishSyncChecked(NewEvent(evtA, nil, nil)); !errors.Is(err, ErrEventBusClosed) {
		t.Fatalf("first bus error = %v, want ErrEventBusClosed", err)
	}
	if _, err := second.SubscribeChecked(evtA, func(*Event) {}); err != nil {
		t.Fatalf("rebuilt bus SubscribeChecked: %v", err)
	}
}

func TestEventBusCheckedAPIsRejectInvalidInputs(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()
	if _, err := bus.SubscribeChecked(evtA, nil); !errors.Is(err, ErrEventBusInvalidHandler) {
		t.Fatalf("nil handler error = %v", err)
	}
	if _, err := bus.UnsubscribeChecked(0); !errors.Is(err, ErrEventBusInvalidSubscription) {
		t.Fatalf("zero subscription error = %v", err)
	}
	if err := bus.PublishChecked(nil); !errors.Is(err, ErrEventBusInvalidEvent) {
		t.Fatalf("nil event error = %v", err)
	}
}

func TestEventBusZeroValueIsUsable(t *testing.T) {
	var bus EventBus
	if _, err := bus.SubscribeChecked(evtA, func(*Event) {}); err != nil {
		t.Fatalf("zero-value SubscribeChecked: %v", err)
	}
	if err := bus.PublishSyncChecked(NewEvent(evtA, nil, nil)); err != nil {
		t.Fatalf("zero-value PublishSyncChecked: %v", err)
	}
	bus.Close()
}
