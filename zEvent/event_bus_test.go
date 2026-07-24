package zEvent

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const evtA EventType = 1

// TestEventBus_SubscribeUnsubscribe 验证 Subscribe 返回句柄、Unsubscribe 按句柄退订生效。
func TestEventBus_SubscribeUnsubscribe(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()

	var count atomic.Int64
	id := bus.Subscribe(evtA, func(e *Event) { count.Add(1) })
	if id == 0 {
		t.Fatal("Subscribe should return a non-zero SubscriptionID")
	}
	if bus.GetHandlerCount(evtA) != 1 {
		t.Fatalf("expected 1 handler, got %d", bus.GetHandlerCount(evtA))
	}

	bus.PublishSync(NewEvent(evtA, nil, nil))
	if count.Load() != 1 {
		t.Fatalf("expected handler called once, got %d", count.Load())
	}

	if !bus.Unsubscribe(id) {
		t.Fatal("Unsubscribe(id) should return true for existing subscription")
	}
	if bus.GetHandlerCount(evtA) != 0 {
		t.Fatalf("expected 0 handlers after unsubscribe, got %d", bus.GetHandlerCount(evtA))
	}

	bus.PublishSync(NewEvent(evtA, nil, nil))
	if count.Load() != 1 {
		t.Fatalf("handler should not be called after unsubscribe, count=%d", count.Load())
	}
}

// TestEventBus_UnsubscribeUnknown 验证退订不存在的句柄返回 false。
func TestEventBus_UnsubscribeUnknown(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()
	if bus.Unsubscribe(0) {
		t.Fatal("Unsubscribe(0) should return false")
	}
	if bus.Unsubscribe(SubscriptionID(9999)) {
		t.Fatal("Unsubscribe(unknown) should return false")
	}
}

// TestEventBus_UnsubscribeOneOfMany 验证多订阅时只精确移除目标，其余保留。
func TestEventBus_UnsubscribeOneOfMany(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()

	var a, b, c atomic.Int64
	bus.Subscribe(evtA, func(e *Event) { a.Add(1) })
	idB := bus.Subscribe(evtA, func(e *Event) { b.Add(1) })
	bus.Subscribe(evtA, func(e *Event) { c.Add(1) })

	if !bus.Unsubscribe(idB) {
		t.Fatal("unsubscribe idB failed")
	}
	bus.PublishSync(NewEvent(evtA, nil, nil))

	if a.Load() != 1 || c.Load() != 1 {
		t.Fatalf("remaining handlers should fire: a=%d c=%d", a.Load(), c.Load())
	}
	if b.Load() != 0 {
		t.Fatalf("unsubscribed handler should not fire: b=%d", b.Load())
	}
}

// TestEventBus_AsyncPublish 验证异步 Publish 能投递到 handler。
func TestEventBus_AsyncPublish(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()

	done := make(chan struct{}, 1)
	bus.Subscribe(evtA, func(e *Event) { done <- struct{}{} })
	bus.Publish(NewEvent(evtA, nil, "payload"))

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("async Publish did not deliver within timeout")
	}
}

// TestEventBus_SyncPanicIsolation 验证 PublishSync 中某 handler panic 不影响其它 handler。
func TestEventBus_SyncPanicIsolation(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()

	var after atomic.Int64
	bus.Subscribe(evtA, func(e *Event) { panic("boom") })
	bus.Subscribe(evtA, func(e *Event) { after.Add(1) })

	bus.PublishSync(NewEvent(evtA, nil, nil)) // 不得因第一个 handler panic 而中断
	if after.Load() != 1 {
		t.Fatalf("second handler should run despite first panicking, got %d", after.Load())
	}
}

// TestEventBus_ConcurrentSubUnsubPublish 并发订阅/退订/发布，验证无 panic/竞态崩溃。
func TestEventBus_ConcurrentSubUnsubPublish(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				id := bus.Subscribe(evtA, func(e *Event) {})
				bus.Publish(NewEvent(evtA, nil, nil))
				bus.Unsubscribe(id)
			}
		}()
	}
	wg.Wait()
}
