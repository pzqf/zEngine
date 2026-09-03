package zActor

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type gateActor struct {
	*BaseActor
	entered   chan struct{}
	release   chan struct{}
	enterOnce sync.Once
	processed atomic.Int64
}

func newGateActor(id int64, size int) *gateActor {
	a := &gateActor{
		BaseActor: NewBaseActor(id, size),
		entered:   make(chan struct{}),
		release:   make(chan struct{}),
	}
	a.SetSelf(a)
	return a
}

func (a *gateActor) ProcessMessage(ActorMessage) {
	a.enterOnce.Do(func() {
		close(a.entered)
		<-a.release
	})
	a.processed.Add(1)
}

func TestActorSendAdmissionReportsStateFullAndNil(t *testing.T) {
	a := newGateActor(101, 1)
	if err := a.SendMessage(newMsg(a.ID())); !errors.Is(err, ErrActorNotRunning) {
		t.Fatalf("send before Start: got %v, want ErrActorNotRunning", err)
	}
	if err := a.SendMessage(nil); !errors.Is(err, ErrActorNilMessage) {
		t.Fatalf("send nil: got %v, want ErrActorNilMessage", err)
	}
	if err := a.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := a.SendMessage(newMsg(a.ID())); err != nil {
		t.Fatalf("send first: %v", err)
	}
	<-a.entered
	if err := a.SendMessage(newMsg(a.ID())); err != nil {
		t.Fatalf("fill mailbox: %v", err)
	}
	if err := a.SendMessage(newMsg(a.ID())); !errors.Is(err, ErrActorMailboxFull) {
		t.Fatalf("send to full mailbox: got %v, want ErrActorMailboxFull", err)
	}

	stopDone := make(chan error, 1)
	go func() { stopDone <- a.StopContext(context.Background()) }()
	waitForActorState(t, a.BaseActor, ActorStateStopping)
	if err := a.SendMessage(newMsg(a.ID())); !errors.Is(err, ErrActorStopping) {
		t.Fatalf("send while stopping: got %v, want ErrActorStopping", err)
	}
	close(a.release)
	if err := <-stopDone; err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	if got := a.processed.Load(); got != 2 {
		t.Fatalf("processed accepted messages: got %d, want 2", got)
	}
	if err := a.SendMessage(newMsg(a.ID())); !errors.Is(err, ErrActorStopped) {
		t.Fatalf("send after stop: got %v, want ErrActorStopped", err)
	}
	stats := a.Stats()
	if stats.Accepted != 2 || stats.RejectedFull != 1 || stats.RejectedState < 2 {
		t.Fatalf("unexpected actor stats: %+v", stats)
	}
}

func TestActorStopAndAdmissionRaceDrainsEveryAcceptedMessage(t *testing.T) {
	a := newTestActor(102, 32)
	if err := a.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	var accepted atomic.Int64
	start := make(chan struct{})
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 200; i++ {
				err := a.SendMessage(newMsg(a.ID()))
				switch {
				case err == nil:
					accepted.Add(1)
				case errors.Is(err, ErrActorMailboxFull):
				case errors.Is(err, ErrActorStopping), errors.Is(err, ErrActorStopped):
					return
				default:
					t.Errorf("unexpected SendMessage error: %v", err)
					return
				}
			}
		}()
	}
	close(start)
	time.Sleep(time.Millisecond)
	if err := a.StopContext(context.Background()); err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	wg.Wait()
	if got, want := a.processed.Load(), accepted.Load(); got != want {
		t.Fatalf("processed accepted messages: got %d, want %d", got, want)
	}
}

type recordActor struct {
	*BaseActor
	mu       sync.Mutex
	received []ActorMessage
	entered  chan struct{}
	release  chan struct{}
	first    sync.Once
}

type priorityTaggedMessage struct {
	BaseActorMessage
}

func (*priorityTaggedMessage) GetPriority() MessagePriority {
	return PriorityHigh
}

func newRecordActor(id int64, size int, gated bool) *recordActor {
	a := &recordActor{BaseActor: NewBaseActor(id, size)}
	if gated {
		a.entered = make(chan struct{})
		a.release = make(chan struct{})
	}
	a.SetSelf(a)
	return a
}

func (a *recordActor) ProcessMessage(msg ActorMessage) {
	if a.entered != nil {
		a.first.Do(func() {
			close(a.entered)
			<-a.release
		})
	}
	a.mu.Lock()
	a.received = append(a.received, msg)
	a.mu.Unlock()
}

func (a *recordActor) snapshot() []ActorMessage {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]ActorMessage(nil), a.received...)
}

func TestActorSendPriorityPreservesPayloadAndBoundsStarvation(t *testing.T) {
	a := newRecordActor(103, 64, true)
	if err := a.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	blocker := newMsg(a.ID())
	if err := a.SendMessage(blocker); err != nil {
		t.Fatalf("send blocker: %v", err)
	}
	<-a.entered

	high := make([]ActorMessage, maxHighPriorityBurst+4)
	for i := range high {
		high[i] = newMsg(a.ID())
		if err := a.SendPriority(high[i], PriorityHigh); err != nil {
			t.Fatalf("SendPriority %d: %v", i, err)
		}
	}
	normal := newMsg(a.ID())
	if err := a.SendMessage(normal); err != nil {
		t.Fatalf("send normal: %v", err)
	}

	stopDone := make(chan error, 1)
	go func() { stopDone <- a.StopContext(context.Background()) }()
	close(a.release)
	if err := <-stopDone; err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	received := a.snapshot()
	if len(received) != len(high)+2 {
		t.Fatalf("received count: got %d, want %d", len(received), len(high)+2)
	}
	if received[1] != high[0] {
		t.Fatal("SendPriority did not deliver the original message payload")
	}
	normalIndex := -1
	for i, msg := range received {
		if msg == normal {
			normalIndex = i
			break
		}
	}
	if normalIndex < 0 || normalIndex > maxHighPriorityBurst+1 {
		t.Fatalf("normal message starved behind high priority burst: index=%d", normalIndex)
	}
}

func TestActorSendMessageAlwaysUsesNormalLane(t *testing.T) {
	a := newRecordActor(112, 4, true)
	if err := a.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := a.SendMessage(newMsg(a.ID())); err != nil {
		t.Fatalf("send blocker: %v", err)
	}
	<-a.entered

	msg := &priorityTaggedMessage{BaseActorMessage: BaseActorMessage{ActorID: a.ID()}}
	if err := a.SendMessage(msg); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	stats := a.Stats()
	if stats.NormalDepth != 1 || stats.HighDepth != 0 {
		t.Fatalf("SendMessage lane depths: normal=%d high=%d, want normal=1 high=0", stats.NormalDepth, stats.HighDepth)
	}

	stopDone := make(chan error, 1)
	go func() { stopDone <- a.StopContext(context.Background()) }()
	close(a.release)
	if err := <-stopDone; err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	received := a.snapshot()
	if len(received) != 2 || received[1] != msg {
		t.Fatalf("received messages: got %#v, want blocker then original message", received)
	}
}

func TestActorLatestWinsUsesSeparateSlot(t *testing.T) {
	a := newRecordActor(104, 4, true)
	if err := a.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := a.SendMessage(newMsg(a.ID())); err != nil {
		t.Fatalf("send blocker: %v", err)
	}
	<-a.entered

	first := newMsg(a.ID())
	second := newMsg(a.ID())
	if replaced, err := a.SendLatest(first); err != nil || replaced {
		t.Fatalf("first SendLatest: replaced=%v err=%v", replaced, err)
	}
	if replaced, err := a.SendLatest(second); err != nil || !replaced {
		t.Fatalf("second SendLatest: replaced=%v err=%v", replaced, err)
	}

	stopDone := make(chan error, 1)
	go func() { stopDone <- a.StopContext(context.Background()) }()
	close(a.release)
	if err := <-stopDone; err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	received := a.snapshot()
	if len(received) != 2 || received[1] != second {
		t.Fatalf("latest-wins delivery: got %#v, want blocker then second", received)
	}
	if got := a.Stats().LatestReplaced; got != 1 {
		t.Fatalf("latest replacement metric: got %d, want 1", got)
	}
}

type callbackHooks struct {
	onStart   func() error
	onStop    func()
	onRestart func() error
}

func (h *callbackHooks) OnStart() error {
	if h.onStart != nil {
		return h.onStart()
	}
	return nil
}

func (h *callbackHooks) OnStop() {
	if h.onStop != nil {
		h.onStop()
	}
}

func (h *callbackHooks) OnRestart() error {
	if h.onRestart != nil {
		return h.onRestart()
	}
	return nil
}

func TestActorLifecycleHooksRunOutsideMutexAndReentryDoesNotDeadlock(t *testing.T) {
	a := newTestActor(105, 8)
	var startStartErr, startStopErr, startSendErr error
	var stopStartErr, stopStopErr, stopSendErr error
	hooks := &callbackHooks{}
	hooks.onStart = func() error {
		startStartErr = a.Start()
		startStopErr = a.Stop()
		startSendErr = a.SendMessage(newMsg(a.ID()))
		return nil
	}
	hooks.onStop = func() {
		stopStartErr = a.Start()
		stopStopErr = a.Stop()
		stopSendErr = a.SendMessage(newMsg(a.ID()))
	}
	a.SetLifecycleHooks(hooks)

	startDone := make(chan error, 1)
	go func() { startDone <- a.Start() }()
	select {
	case err := <-startDone:
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("OnStart reentry deadlocked")
	}
	if !errors.Is(startStartErr, ErrActorStarting) || !errors.Is(startStopErr, ErrActorStarting) ||
		!errors.Is(startSendErr, ErrActorStarting) {
		t.Fatalf("OnStart reentry errors: Start=%v Stop=%v Send=%v", startStartErr, startStopErr, startSendErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := a.StopContext(ctx); err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	if !errors.Is(stopStartErr, ErrActorStopping) || !errors.Is(stopStopErr, ErrActorStopping) ||
		!errors.Is(stopSendErr, ErrActorStopping) {
		t.Fatalf("OnStop reentry errors: Start=%v Stop=%v Send=%v", stopStartErr, stopStopErr, stopSendErr)
	}
}

func TestActorOnRestartHookReentryReturnsStableResultsAndDrainsOnStop(t *testing.T) {
	cfg := DefaultSupervisorConfig()
	cfg.RestartBackoff = time.Millisecond
	a := &supervisedActor{BaseActor: NewBaseActorWithSupervisor(108, 8, cfg)}
	a.SetSelf(a)

	var restartStartErr, restartSendErr, restartStopErr error
	hooks := &callbackHooks{}
	hooks.onRestart = func() error {
		restartSendErr = a.SendMessage(newMsg(a.ID()))
		restartStartErr = a.Start()
		restartStopErr = a.Stop()
		return nil
	}
	a.SetLifecycleHooks(hooks)

	if err := a.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := a.SendMessage(&poisonMsg{BaseActorMessage: BaseActorMessage{ActorID: a.ID()}, poison: true}); err != nil {
		t.Fatalf("send poison: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := a.WaitStopped(ctx); err != nil {
		t.Fatalf("WaitStopped: %v", err)
	}
	if restartSendErr != nil || !errors.Is(restartStartErr, ErrActorRestarting) || restartStopErr != nil {
		t.Fatalf("OnRestart reentry errors: Send=%v Start=%v Stop=%v", restartSendErr, restartStartErr, restartStopErr)
	}
	if got := a.processed.Load(); got != 1 {
		t.Fatalf("messages accepted by OnRestart before Stop: got %d processed, want 1", got)
	}
	if got := a.Stats().Abandoned; got != 0 {
		t.Fatalf("normal Stop during OnRestart abandoned %d accepted messages", got)
	}
}

func TestActorStopDuringRestartBackoffDrainsAcceptedMessages(t *testing.T) {
	cfg := DefaultSupervisorConfig()
	cfg.RestartBackoff = time.Second
	a := &supervisedActor{BaseActor: NewBaseActorWithSupervisor(109, 8, cfg)}
	a.SetSelf(a)
	if err := a.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := a.SendMessage(&poisonMsg{BaseActorMessage: BaseActorMessage{ActorID: a.ID()}, poison: true}); err != nil {
		t.Fatalf("send poison: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := a.SendMessage(newMsg(a.ID())); err != nil {
			t.Fatalf("send queued %d: %v", i, err)
		}
	}
	waitForActorState(t, a.BaseActor, ActorStateRestarting)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := a.StopContext(ctx); err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	if got := a.processed.Load(); got != 3 {
		t.Fatalf("processed accepted messages: got %d, want 3", got)
	}
	if got := a.Stats().Abandoned; got != 0 {
		t.Fatalf("normal Stop during restart abandoned %d accepted messages", got)
	}
}

func TestActorLifecycleHooksAreGenerationScopedAndFailuresAreCounted(t *testing.T) {
	a := newTestActor(110, 8)
	var firstStops, secondStarts, secondStops atomic.Int64
	first := &callbackHooks{onStop: func() { firstStops.Add(1) }}
	second := &callbackHooks{
		onStart: func() error {
			secondStarts.Add(1)
			return nil
		},
		onStop: func() { secondStops.Add(1) },
	}
	a.SetLifecycleHooks(first)
	if err := a.Start(); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	a.SetLifecycleHooks(second)
	if err := a.StopContext(context.Background()); err != nil {
		t.Fatalf("first StopContext: %v", err)
	}
	if firstStops.Load() != 1 || secondStops.Load() != 0 {
		t.Fatalf("first generation hooks: first stops=%d second stops=%d", firstStops.Load(), secondStops.Load())
	}
	if err := a.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if err := a.StopContext(context.Background()); err != nil {
		t.Fatalf("second StopContext: %v", err)
	}
	if secondStarts.Load() != 1 || secondStops.Load() != 1 {
		t.Fatalf("second generation hooks: starts=%d stops=%d", secondStarts.Load(), secondStops.Load())
	}

	failing := newTestActor(111, 8)
	failing.SetLifecycleHooks(&callbackHooks{onStart: func() error { return errors.New("start failed") }})
	if err := failing.Start(); err == nil {
		t.Fatal("Start with failing hook returned nil")
	}
	if got := failing.Stats().HookFailures; got != 1 {
		t.Fatalf("hook failure count: got %d, want 1", got)
	}
}

func TestActorUnknownSupervisorStrategyIsExplicitlyUnsupported(t *testing.T) {
	cfg := DefaultSupervisorConfig()
	cfg.Strategy = SupervisorStrategy(2)
	a := NewBaseActorWithSupervisor(106, 8, cfg)
	if err := a.Start(); !errors.Is(err, ErrActorUnsupportedSupervisorStrategy) {
		t.Fatalf("Start with unknown strategy: got %v, want ErrActorUnsupportedSupervisorStrategy", err)
	}
}

func TestActorSupervisorStopCountsAbandonedMessages(t *testing.T) {
	cfg := DefaultSupervisorConfig()
	cfg.Strategy = SupervisorStrategyStop
	a := &panicGateActor{
		BaseActor: NewBaseActorWithSupervisor(107, 8, cfg),
		entered:   make(chan struct{}),
		release:   make(chan struct{}),
	}
	a.SetSelf(a)
	if err := a.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := a.SendMessage(newMsg(a.ID())); err != nil {
		t.Fatalf("send poison: %v", err)
	}
	<-a.entered
	for i := 0; i < 3; i++ {
		if err := a.SendMessage(newMsg(a.ID())); err != nil {
			t.Fatalf("send queued %d: %v", i, err)
		}
	}
	close(a.release)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := a.WaitStopped(ctx); err != nil {
		t.Fatalf("WaitStopped: %v", err)
	}
	stats := a.Stats()
	if stats.Panics != 1 || stats.Abandoned != 3 {
		t.Fatalf("supervisor stop stats: %+v", stats)
	}
}

type panicGateActor struct {
	*BaseActor
	entered chan struct{}
	release chan struct{}
}

func (a *panicGateActor) ProcessMessage(ActorMessage) {
	close(a.entered)
	<-a.release
	panic("panic gate")
}

func waitForActorState(t *testing.T, a *BaseActor, target ActorState) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if a.Stats().State == target {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("actor did not reach state %v", target)
}
