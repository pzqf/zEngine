package zActor

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testActor 是嵌入 *BaseActor 并重写 ProcessMessage 的最小 Actor，
// 用于验证 SetSelf 分派与并发安全。
type testActor struct {
	*BaseActor
	processed atomic.Int64
}

func newTestActor(id int64, chanSize int) *testActor {
	a := &testActor{BaseActor: NewBaseActor(id, chanSize)}
	a.SetSelf(a) // 关键：使 run() 分派到本类型的 ProcessMessage
	return a
}

func (t *testActor) ProcessMessage(msg ActorMessage) {
	t.processed.Add(1)
}

type testMsg struct{ BaseActorMessage }

func newMsg(id int64) *testMsg { return &testMsg{BaseActorMessage{ActorID: id}} }

// TestActor_SelfDispatch 验证 SetSelf 后消息分派到子类的 ProcessMessage。
func TestActor_SelfDispatch(t *testing.T) {
	a := newTestActor(1, 64)
	if err := a.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	const n = 20
	for i := 0; i < n; i++ {
		a.SendMessage(newMsg(1))
	}
	// 轮询等待处理完成
	deadline := time.Now().Add(2 * time.Second)
	for a.processed.Load() < n && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := a.processed.Load(); got != n {
		t.Fatalf("expected %d processed via self dispatch, got %d", n, got)
	}
	if err := a.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
}

// TestActor_ConcurrentSendAndStop 是并发安全回归测试：
// 旧实现 Stop() 会 close 消息 channel，与并发 SendMessage 形成 send-on-closed panic。
// 修复后 Stop() 只关 stopCh，本测试在 `-race` 下应干净通过、无 panic。
func TestActor_ConcurrentSendAndStop(t *testing.T) {
	for iter := 0; iter < 30; iter++ {
		a := newTestActor(int64(iter), 8)
		if err := a.Start(); err != nil {
			t.Fatalf("start: %v", err)
		}

		var wg sync.WaitGroup
		for g := 0; g < 8; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 300; i++ {
					a.SendMessage(newMsg(a.ID()))
				}
			}()
		}

		time.Sleep(time.Millisecond)
		_ = a.Stop() // 与发送并发——不得 panic
		wg.Wait()
	}
}

// blockingActor 的 ProcessMessage 会阻塞，用于制造满邮箱以验证丢弃计数。
type blockingActor struct {
	*BaseActor
	release chan struct{}
}

func (b *blockingActor) ProcessMessage(msg ActorMessage) {
	<-b.release
}

// TestActor_DropCounter 验证邮箱满时消息被丢弃且计数可观测。
func TestActor_DropCounter(t *testing.T) {
	release := make(chan struct{})
	a := &blockingActor{BaseActor: NewBaseActor(99, 2), release: release}
	a.SetSelf(a)
	if err := a.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	// run() 取走首条并阻塞在 ProcessMessage；容量 2 的邮箱很快填满，其余被丢弃。
	for i := 0; i < 50; i++ {
		a.SendMessage(newMsg(99))
	}
	if a.DroppedMessages() == 0 {
		t.Fatalf("expected dropped messages > 0 with full mailbox")
	}

	close(release) // 解除阻塞，让 run() 能感知 stopCh
	if err := a.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
}

// TestActor_DoubleStop 验证重复 Stop 不 panic（不会二次 close）。
func TestActor_DoubleStop(t *testing.T) {
	a := newTestActor(7, 8)
	if err := a.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := a.Stop(); err != nil {
		t.Fatalf("first stop: %v", err)
	}
	if err := a.Stop(); err == nil {
		t.Fatalf("second stop should return error, got nil")
	}
}

// ——— 监督策略（panic 恢复 + 限次重启）回归测试 ———

type poisonMsg struct {
	BaseActorMessage
	poison bool
}

// supervisedActor：首条 poison 消息触发一次 panic（之后正常），其余消息计数。
type supervisedActor struct {
	*BaseActor
	processed atomic.Int64
	panicked  atomic.Bool
}

func (s *supervisedActor) ProcessMessage(msg ActorMessage) {
	if pm, ok := msg.(*poisonMsg); ok && pm.poison {
		if s.panicked.CompareAndSwap(false, true) {
			panic("supervised boom")
		}
	}
	s.processed.Add(1)
}

// TestActor_SupervisorRestartsAndDrainsBuffered 验证 Restart 策略下 actor panic 后
// 能自动重启并继续处理。关键回归点：**重启不得丢弃在途缓冲消息**——旧实现在 tryRestart
// 里重建 normal mailbox，会把 panic 时邮箱内已排队的消息全部丢掉；修复后复用既有 channel，
// 缓冲消息在重启后应被完整排空。
func TestActor_SupervisorRestartsAndDrainsBuffered(t *testing.T) {
	cfg := SupervisorConfig{Strategy: SupervisorStrategyRestart, MaxRestarts: 3, RestartWindow: time.Minute, RestartBackoff: 10 * time.Millisecond}
	a := &supervisedActor{BaseActor: NewBaseActorWithSupervisor(1, 128, cfg)}
	a.SetSelf(a)
	if err := a.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	const n = 30
	a.SendMessage(&poisonMsg{BaseActorMessage: BaseActorMessage{ActorID: 1}, poison: true}) // 触发一次 panic
	for i := 0; i < n; i++ {
		a.SendMessage(newMsg(1)) // 在途缓冲，重启后须被排空（旧实现会丢）
	}

	deadline := time.Now().Add(3 * time.Second)
	for a.processed.Load() < n && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := a.processed.Load(); got != n {
		t.Fatalf("expected %d processed after restart (buffered msgs must survive), got %d", n, got)
	}
	if !a.IsRunning() {
		t.Fatalf("actor should be running after successful restart")
	}
	if err := a.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
}

type alwaysPanicActor struct {
	*BaseActor
	panics atomic.Int64
}

func (p *alwaysPanicActor) ProcessMessage(msg ActorMessage) {
	p.panics.Add(1)
	panic("always boom")
}

// TestActor_SupervisorGivesUpAfterMaxRestarts 验证超过窗口内 MaxRestarts 后放弃重启、
// actor 转为停止（running=false），不无限重启打爆进程。
func TestActor_SupervisorGivesUpAfterMaxRestarts(t *testing.T) {
	cfg := SupervisorConfig{Strategy: SupervisorStrategyRestart, MaxRestarts: 2, RestartWindow: time.Minute, RestartBackoff: 5 * time.Millisecond}
	a := &alwaysPanicActor{BaseActor: NewBaseActorWithSupervisor(2, 16, cfg)}
	a.SetSelf(a)
	if err := a.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	for i := 0; i < 5; i++ {
		a.SendMessage(newMsg(2)) // 每条都 panic，逐次消耗重启预算
	}

	deadline := time.Now().Add(3 * time.Second)
	for a.IsRunning() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if a.IsRunning() {
		t.Fatalf("actor should have given up (stopped) after exceeding max restarts")
	}
}

// TestActor_SupervisorStopStrategyNoRestart 验证 Stop 策略下 panic 后不重启、直接停止。
func TestActor_SupervisorStopStrategyNoRestart(t *testing.T) {
	cfg := SupervisorConfig{Strategy: SupervisorStrategyStop, MaxRestarts: 3, RestartWindow: time.Minute, RestartBackoff: 5 * time.Millisecond}
	a := &supervisedActor{BaseActor: NewBaseActorWithSupervisor(3, 16, cfg)}
	a.SetSelf(a)
	if err := a.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	a.SendMessage(&poisonMsg{BaseActorMessage: BaseActorMessage{ActorID: 3}, poison: true})

	deadline := time.Now().Add(2 * time.Second)
	for a.IsRunning() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if a.IsRunning() {
		t.Fatalf("actor with Stop strategy should not be running after panic")
	}

	before := a.processed.Load()
	a.SendMessage(newMsg(3))
	time.Sleep(50 * time.Millisecond)
	if a.processed.Load() != before {
		t.Fatalf("stopped actor should not process further messages")
	}
}
