package zServer

import (
	"sync/atomic"
	"testing"
	"time"
)

type noopHooks struct{}

func (noopHooks) OnBeforeStart() error { return nil }
func (noopHooks) OnAfterStart() error  { return nil }
func (noopHooks) OnBeforeStop()        {}

func waitForCount(c *atomic.Int32, want int32, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if c.Load() >= want {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return c.Load() >= want
}

// TestStateChangeListener_HandleAddRemove 验证 NET-7：AddStateChangeListener 返回唯一句柄，
// 用该句柄 RemoveStateChangeListener 能真正移除监听器。此前用 &listener(参数栈地址)作 key→
// Add 与 Remove 的地址不同→永远删不掉；本用例中"移除后再触发只剩一个监听器被调用"即证明修复。
func TestStateChangeListener_HandleAddRemove(t *testing.T) {
	srv := NewBaseServer(ServerType("test"), "1", "test", "v0", noopHooks{})

	var count atomic.Int32
	h1 := srv.AddStateChangeListener(func(StateChangeEvent) { count.Add(1) })
	h2 := srv.AddStateChangeListener(func(StateChangeEvent) { count.Add(1) })

	if h1 == 0 || h2 == 0 || h1 == h2 {
		t.Fatalf("handles must be unique and nonzero: h1=%d h2=%d", h1, h2)
	}

	// 第一次状态变化：两个监听器都应被调用（Starting→Initializing 合法）。
	if err := srv.SetState(StateInitializing, "t1"); err != nil {
		t.Fatalf("SetState 1 failed: %v", err)
	}
	if !waitForCount(&count, 2, time.Second) {
		t.Fatalf("expected 2 listener calls, got %d", count.Load())
	}

	// 移除 h1 后再次变化：应只剩 h2 被调用（Initializing→Ready 合法）。
	srv.RemoveStateChangeListener(h1)
	if err := srv.SetState(StateReady, "t2"); err != nil {
		t.Fatalf("SetState 2 failed: %v", err)
	}
	if !waitForCount(&count, 3, time.Second) {
		t.Fatalf("expected 3 total calls after remove, got %d", count.Load())
	}

	// 再等一小会儿，确认被移除的 h1 未被调用（否则会到 4）。
	time.Sleep(50 * time.Millisecond)
	if got := count.Load(); got != 3 {
		t.Fatalf("removed listener still fired: total=%d (want 3)", got)
	}
}
