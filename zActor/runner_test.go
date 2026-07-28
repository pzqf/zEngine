package zActor

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestRunner_DoRunsAndReturns Do 同步执行并等待完成，返回值靠闭包捕获。
func TestRunner_DoRunsAndReturns(t *testing.T) {
	r := NewRunner(1, 0)
	r.Start()
	defer r.Stop()

	var result int
	r.Do(func() { result = 42 })
	if result != 42 {
		t.Fatalf("Do 应同步执行完毕，result 期望 42，got %d", result)
	}
}

// TestRunner_SerialExecution 并发 Do 被串行化：共享计数器无锁自增也不会丢更新。
// （-race 下应无数据竞争——由 CI/race runner 验证；本用例断言最终值正确 + 无死锁。）
func TestRunner_SerialExecution(t *testing.T) {
	r := NewRunner(2, 0)
	r.Start()
	defer r.Stop()

	const n = 200
	counter := 0 // 故意不加锁：Runner 串行保证安全
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Do(func() { counter++ })
		}()
	}
	wg.Wait()
	if counter != n {
		t.Fatalf("串行自增应得 %d，got %d", n, counter)
	}
}

// TestRunner_PostTickNeverBlocks 队列满时 PostTick 丢弃本次、永不阻塞。
func TestRunner_PostTickNeverBlocks(t *testing.T) {
	r := NewRunner(3, 1) // 极小队列
	r.Start()
	defer r.Stop()

	// 先塞一个长命令占住 goroutine，使后续入队排队。
	block := make(chan struct{})
	r.Post(func() { <-block })

	// 猛灌 PostTick——队列会满，多余的被丢弃，但调用绝不阻塞。
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			r.PostTick(func() {})
		}
		close(done)
	}()
	select {
	case <-done:
		// 好：PostTick 未阻塞
	case <-time.After(2 * time.Second):
		t.Fatal("PostTick 在队列满时阻塞了（应丢弃本次）")
	}
	close(block)
}

// TestRunner_PanicIsolation 一条命令 panic 只被隔离，后续命令仍能执行。
func TestRunner_PanicIsolation(t *testing.T) {
	r := NewRunner(4, 0)
	r.Start()
	defer r.Stop()

	r.Do(func() { panic("boom") }) // 应被 safeExec recover，不崩 goroutine

	var ok bool
	r.Do(func() { ok = true }) // 若 goroutine 被 panic 带走，这条永远不返回（死锁→测试超时）
	if !ok {
		t.Fatal("命令 panic 后 Runner 应继续处理后续命令")
	}
}

// TestRunner_StopIdempotent Stop 可重复调用；未 Start 时 Do 就地执行。
func TestRunner_StopIdempotent(t *testing.T) {
	r := NewRunner(5, 0)

	// 未 Start：Do 就地执行，不阻塞。
	var ran bool
	r.Do(func() { ran = true })
	if !ran {
		t.Fatal("未 Start 时 Do 应就地执行")
	}

	r.Start()
	r.Stop()
	r.Stop() // 幂等，不 panic

	if r.IsRunning() {
		t.Fatal("Stop 后 IsRunning 应为 false")
	}
}

// TestRunner_PostReportsStop 停止后 Post 返回 false。
func TestRunner_PostReportsStop(t *testing.T) {
	r := NewRunner(6, 0)
	r.Start()
	r.Stop()
	// 给 goroutine 一点时间感知 stopCh。
	var got atomic.Bool
	deadline := time.After(2 * time.Second)
	for {
		if !r.Post(func() {}) {
			got.Store(true)
			break
		}
		select {
		case <-deadline:
			t.Fatal("Stop 后 Post 应返回 false")
		default:
		}
	}
	if !got.Load() {
		t.Fatal("Stop 后 Post 应返回 false")
	}
}
