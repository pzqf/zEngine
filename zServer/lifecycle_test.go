package zServer

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type lifecycleTestLogger struct{}

func (lifecycleTestLogger) Debug(string, ...interface{}) {}
func (lifecycleTestLogger) Info(string, ...interface{})  {}
func (lifecycleTestLogger) Warn(string, ...interface{})  {}
func (lifecycleTestLogger) Error(string, ...interface{}) {}
func (lifecycleTestLogger) Fatal(string, ...interface{}) {}
func (lifecycleTestLogger) Writer() io.Writer            { return io.Discard }

type lifecycleTestHooks struct {
	server       *BaseServer
	beforeStart  func() error
	afterStart   func() error
	beforeStop   func()
	afterStop    func()
	startReached chan struct{}
}

func (h *lifecycleTestHooks) OnBeforeStart() error {
	if h.beforeStart != nil {
		return h.beforeStart()
	}
	return nil
}

func (h *lifecycleTestHooks) OnAfterStart() error {
	if h.startReached != nil {
		close(h.startReached)
	}
	if h.afterStart != nil {
		return h.afterStart()
	}
	return nil
}

func (h *lifecycleTestHooks) OnBeforeStop() {
	if h.beforeStop != nil {
		h.beforeStop()
	}
}

func (h *lifecycleTestHooks) OnAfterStop() {
	if h.afterStop != nil {
		h.afterStop()
	}
}

func newLifecycleTestServer(hooks *lifecycleTestHooks) *BaseServer {
	server := NewBaseServer("test", "test-1", "test", "0.0.1", hooks)
	hooks.server = server
	server.SetLogger(lifecycleTestLogger{})
	return server
}

func TestStartFailureRollsBackRegisteredResources(t *testing.T) {
	tests := []struct {
		name           string
		failAfterStart bool
	}{
		{name: "before start"},
		{name: "after start", failAfterStart: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startErr := errors.New("start failed")
			var mu sync.Mutex
			var calls []string
			record := func(call string) {
				mu.Lock()
				calls = append(calls, call)
				mu.Unlock()
			}

			hooks := &lifecycleTestHooks{}
			server := newLifecycleTestServer(hooks)
			hooks.beforeStart = func() error {
				if err := server.RegisterCleanup("first", func() error {
					record("first")
					return nil
				}); err != nil {
					return err
				}
				if !tt.failAfterStart {
					return startErr
				}
				return nil
			}
			hooks.afterStart = func() error {
				if err := server.RegisterCleanup("second", func() error {
					record("second")
					return nil
				}); err != nil {
					return err
				}
				return startErr
			}
			hooks.afterStop = func() { record("after-stop") }

			err := server.Start()
			if !errors.Is(err, startErr) {
				t.Fatalf("Start() error = %v, want %v", err, startErr)
			}
			want := []string{"first", "after-stop"}
			if tt.failAfterStart {
				want = []string{"second", "first", "after-stop"}
			}
			mu.Lock()
			got := append([]string(nil), calls...)
			mu.Unlock()
			if !equalStrings(got, want) {
				t.Fatalf("cleanup order = %v, want %v", got, want)
			}
			if server.GetContext().Err() == nil {
				t.Fatal("server context was not canceled after start failure")
			}
			if got := server.GetState(); got != StateStopped {
				t.Fatalf("state = %s, want %s", got, StateStopped)
			}
		})
	}
}

func TestStopRunsAllCleanupsInReverseAndAggregatesErrors(t *testing.T) {
	errSecond := errors.New("second close failed")
	errThird := errors.New("third close failed")
	var callsMu sync.Mutex
	var calls []string
	record := func(call string) {
		callsMu.Lock()
		calls = append(calls, call)
		callsMu.Unlock()
	}

	hooks := &lifecycleTestHooks{
		beforeStop: func() { record("before-stop") },
		afterStop:  func() { record("after-stop") },
	}
	server := newLifecycleTestServer(hooks)
	for _, cleanup := range []struct {
		name string
		err  error
	}{
		{name: "first"},
		{name: "second", err: errSecond},
		{name: "third", err: errThird},
	} {
		cleanup := cleanup
		if err := server.RegisterCleanup(cleanup.name, func() error {
			record(cleanup.name)
			return cleanup.err
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}

	err := server.StopWithError()
	if !errors.Is(err, errSecond) || !errors.Is(err, errThird) {
		t.Fatalf("Stop() error = %v, want both cleanup errors", err)
	}
	want := []string{"before-stop", "third", "second", "first", "after-stop"}
	callsMu.Lock()
	got := append([]string(nil), calls...)
	callsMu.Unlock()
	if !equalStrings(got, want) {
		t.Fatalf("stop order = %v, want %v", got, want)
	}
}

func TestConcurrentStopIsIdempotentAndSharesResult(t *testing.T) {
	cleanupErr := errors.New("close failed")
	cleanupStarted := make(chan struct{})
	releaseCleanup := make(chan struct{})
	var cleanupCount atomic.Int32
	var beforeStopCount atomic.Int32
	var afterStopCount atomic.Int32

	hooks := &lifecycleTestHooks{
		beforeStop: func() { beforeStopCount.Add(1) },
		afterStop:  func() { afterStopCount.Add(1) },
	}
	server := newLifecycleTestServer(hooks)
	if err := server.RegisterCleanup("blocking", func() error {
		cleanupCount.Add(1)
		close(cleanupStarted)
		<-releaseCleanup
		return cleanupErr
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}

	const callers = 32
	errs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		go func() { errs <- server.StopWithError() }()
	}
	select {
	case <-cleanupStarted:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not start")
	}
	close(releaseCleanup)
	for i := 0; i < callers; i++ {
		if err := <-errs; !errors.Is(err, cleanupErr) {
			t.Fatalf("Stop() error = %v, want %v", err, cleanupErr)
		}
	}
	if cleanupCount.Load() != 1 || beforeStopCount.Load() != 1 || afterStopCount.Load() != 1 {
		t.Fatalf("lifecycle counts = cleanup:%d before:%d after:%d, want all 1",
			cleanupCount.Load(), beforeStopCount.Load(), afterStopCount.Load())
	}
}

func TestWaitReturnsOnlyAfterCleanupAndAfterStop(t *testing.T) {
	cleanupStarted := make(chan struct{})
	releaseCleanup := make(chan struct{})
	afterStop := make(chan struct{})
	hooks := &lifecycleTestHooks{afterStop: func() { close(afterStop) }}
	server := newLifecycleTestServer(hooks)
	if err := server.RegisterCleanup("blocking", func() error {
		close(cleanupStarted)
		<-releaseCleanup
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.StopWithError() }()
	<-cleanupStarted

	waitDone := make(chan struct{})
	go func() {
		server.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
		t.Fatal("Wait returned before cleanup completed")
	case <-time.After(30 * time.Millisecond):
	}
	close(releaseCleanup)
	select {
	case <-afterStop:
	case <-time.After(time.Second):
		t.Fatal("after-stop hook did not run")
	}
	select {
	case <-waitDone:
	case <-time.After(time.Second):
		t.Fatal("Wait did not return after stop completed")
	}
}

func TestStartBackgroundIsCanceledAndAwaited(t *testing.T) {
	taskStarted := make(chan struct{})
	taskRelease := make(chan struct{})
	server := newLifecycleTestServer(&lifecycleTestHooks{})
	if err := server.StartBackground("worker", func(ctx context.Context) {
		close(taskStarted)
		<-ctx.Done()
		<-taskRelease
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	<-taskStarted
	stopDone := make(chan error, 1)
	go func() { stopDone <- server.StopWithError() }()
	select {
	case err := <-stopDone:
		t.Fatalf("Stop returned before background task exited: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(taskRelease)
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Stop did not wait for background task")
	}
}

func TestStoppedServerCannotRestart(t *testing.T) {
	server := newLifecycleTestServer(&lifecycleTestHooks{})
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	if err := server.StopWithError(); err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); !errors.Is(err, ErrRestartUnsupported) {
		t.Fatalf("second Start() error = %v, want %v", err, ErrRestartUnsupported)
	}
	if StateStopped.CanTransitionTo(StateStarting) {
		t.Fatal("Stopped -> Starting transition must be rejected")
	}
}

func TestRunReleasesSignalSubscription(t *testing.T) {
	startReached := make(chan struct{})
	hooks := &lifecycleTestHooks{startReached: startReached}
	server := newLifecycleTestServer(hooks)
	signalCtx, cancelSignal := context.WithCancel(context.Background())
	var releaseCount atomic.Int32
	server.signalContext = func() (context.Context, context.CancelFunc) {
		return signalCtx, func() { releaseCount.Add(1) }
	}

	runDone := make(chan error, 1)
	go func() { runDone <- server.Run() }()
	<-startReached
	cancelSignal()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return")
	}
	if got := releaseCount.Load(); got != 1 {
		t.Fatalf("signal release count = %d, want 1", got)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
