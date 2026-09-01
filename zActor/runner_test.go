package zActor

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunnerDoContextRunsAndReturns(t *testing.T) {
	r := NewRunner(1, 0)
	r.Start()
	defer r.Stop()

	var result int
	err := r.DoContext(context.Background(), func(context.Context) { result = 42 })
	if err != nil {
		t.Fatalf("DoContext returned error: %v", err)
	}
	if result != 42 {
		t.Fatalf("DoContext must wait for completion: got %d, want 42", result)
	}
}

func TestRunnerDoContextSerializesConcurrentCalls(t *testing.T) {
	r := NewRunner(2, 0)
	r.Start()
	defer r.Stop()

	const n = 200
	counter := 0 // Intentionally unlocked: the Runner is the single writer.
	errCh := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- r.DoContext(context.Background(), func(context.Context) { counter++ })
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent DoContext returned error: %v", err)
		}
	}
	if counter != n {
		t.Fatalf("serialized counter: got %d, want %d", counter, n)
	}
}

func TestRunnerDoesNotExecuteBeforeStart(t *testing.T) {
	r := NewRunner(3, 1)
	var ran atomic.Bool

	err := r.DoContext(context.Background(), func(context.Context) { ran.Store(true) })
	if !errors.Is(err, ErrRunnerNotStarted) {
		t.Fatalf("DoContext before Start: got %v, want ErrRunnerNotStarted", err)
	}
	if ran.Load() {
		t.Fatal("DoContext executed the command in the caller before Start")
	}

	// Deprecated compatibility calls must not retain the old inline fallback.
	r.Do(func() { ran.Store(true) })
	if ran.Load() {
		t.Fatal("legacy Do executed the command in the caller before Start")
	}
	if r.Post(func() {}) {
		t.Fatal("legacy Post accepted a command before Start")
	}
}

func TestRunnerLifecycleErrorPrecedesCanceledContext(t *testing.T) {
	r := NewRunner(31, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := r.DoContext(ctx, func(context.Context) {}); !errors.Is(err, ErrRunnerNotStarted) {
		t.Fatalf("canceled DoContext before Start: got %v, want ErrRunnerNotStarted", err)
	}
	if err := r.PostContext(ctx, func(context.Context) {}); !errors.Is(err, ErrRunnerNotStarted) {
		t.Fatalf("canceled PostContext before Start: got %v, want ErrRunnerNotStarted", err)
	}

	r.Start()
	if err := r.StopContext(context.Background()); err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	if err := r.DoContext(ctx, func(context.Context) {}); !errors.Is(err, ErrRunnerStopped) {
		t.Fatalf("canceled DoContext after Stop: got %v, want ErrRunnerStopped", err)
	}
}

func TestRunnerDoContextReportsStoppingAndStopped(t *testing.T) {
	r := NewRunner(4, 2)
	r.Start()

	entered := make(chan struct{})
	release := make(chan struct{})
	doDone := make(chan error, 1)
	go func() {
		doDone <- r.DoContext(context.Background(), func(context.Context) {
			close(entered)
			<-release
		})
	}()
	<-entered

	stopDone := make(chan error, 1)
	go func() { stopDone <- r.StopContext(context.Background()) }()
	waitForRunnerError(t, r, ErrRunnerStopping)

	var ran atomic.Bool
	err := r.DoContext(context.Background(), func(context.Context) { ran.Store(true) })
	if !errors.Is(err, ErrRunnerStopping) {
		t.Fatalf("DoContext while stopping: got %v, want ErrRunnerStopping", err)
	}
	if ran.Load() {
		t.Fatal("DoContext executed the command while stopping")
	}

	close(release)
	if err := <-doDone; err != nil {
		t.Fatalf("accepted command returned error: %v", err)
	}
	if err := <-stopDone; err != nil {
		t.Fatalf("StopContext returned error: %v", err)
	}

	err = r.DoContext(context.Background(), func(context.Context) { ran.Store(true) })
	if !errors.Is(err, ErrRunnerStopped) {
		t.Fatalf("DoContext after Stop: got %v, want ErrRunnerStopped", err)
	}
	if r.IsRunning() {
		t.Fatal("IsRunning returned true after StopContext completed")
	}
}

func TestRunnerDoContextQueueWaitIsCancelable(t *testing.T) {
	r := NewRunner(5, 1)
	r.Start()
	defer r.Stop()

	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- r.DoContext(context.Background(), func(context.Context) {
			close(entered)
			<-release
		})
	}()
	<-entered

	if err := r.PostContext(context.Background(), func(context.Context) {}); err != nil {
		t.Fatalf("fill queue: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	var ran atomic.Bool
	err := r.DoContext(ctx, func(context.Context) { ran.Store(true) })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("DoContext on full queue: got %v, want deadline exceeded", err)
	}
	if ran.Load() {
		t.Fatal("command rejected by context was executed")
	}

	postCtx, postCancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer postCancel()
	var postRan atomic.Bool
	err = r.PostContext(postCtx, func(context.Context) { postRan.Store(true) })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("PostContext on full queue: got %v, want deadline exceeded", err)
	}
	if postRan.Load() {
		t.Fatal("Post command rejected by context was executed")
	}

	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first command returned error: %v", err)
	}
}

func TestRunnerAcceptedDoCanCancelBeforeExecution(t *testing.T) {
	r := NewRunner(32, 2)
	r.Start()
	defer r.Stop()

	entered := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_ = r.DoContext(context.Background(), func(context.Context) {
			close(entered)
			<-release
		})
	}()
	<-entered

	ctx, cancel := context.WithCancel(context.Background())
	var ran atomic.Bool
	done := make(chan error, 1)
	go func() {
		done <- r.DoContext(ctx, func(context.Context) { ran.Store(true) })
	}()
	waitForRunnerQueueDepth(t, r, 1)
	cancel()
	close(release)

	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("accepted canceled command: got %v, want context.Canceled", err)
	}
	if ran.Load() {
		t.Fatal("command canceled before execution was run")
	}
}

func TestRunnerStopDrainsAcceptedCommands(t *testing.T) {
	r := NewRunner(6, 4)
	r.Start()

	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- r.DoContext(context.Background(), func(context.Context) {
			close(entered)
			<-release
		})
	}()
	<-entered

	var executed atomic.Int32
	for i := 0; i < 3; i++ {
		if err := r.PostContext(context.Background(), func(context.Context) { executed.Add(1) }); err != nil {
			t.Fatalf("PostContext %d: %v", i, err)
		}
	}

	stopDone := make(chan error, 1)
	go func() { stopDone <- r.StopContext(context.Background()) }()
	waitForRunnerError(t, r, ErrRunnerStopping)
	close(release)

	if err := <-firstDone; err != nil {
		t.Fatalf("first command returned error: %v", err)
	}
	if err := <-stopDone; err != nil {
		t.Fatalf("StopContext returned error: %v", err)
	}
	if got := executed.Load(); got != 3 {
		t.Fatalf("accepted commands executed: got %d, want 3", got)
	}
}

func TestRunnerStopContextWaitIsCancelable(t *testing.T) {
	r := NewRunner(7, 1)
	r.Start()

	entered := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_ = r.DoContext(context.Background(), func(context.Context) {
			close(entered)
			<-release
		})
	}()
	<-entered

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := r.StopContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("StopContext timeout: got %v, want deadline exceeded", err)
	}
	waitForRunnerError(t, r, ErrRunnerStopping)

	close(release)
	if err := r.StopContext(context.Background()); err != nil {
		t.Fatalf("wait for eventual stop: %v", err)
	}
}

func TestRunnerDoContextRejectsReentrantCall(t *testing.T) {
	r := NewRunner(8, 2)
	r.Start()
	defer r.Stop()

	var nestedErr error
	var nestedRan atomic.Bool
	err := r.DoContext(context.Background(), func(execCtx context.Context) {
		nestedErr = r.DoContext(execCtx, func(context.Context) { nestedRan.Store(true) })
	})
	if err != nil {
		t.Fatalf("outer DoContext: %v", err)
	}
	if !errors.Is(nestedErr, ErrRunnerReentrantCall) {
		t.Fatalf("nested DoContext: got %v, want ErrRunnerReentrantCall", nestedErr)
	}
	if nestedRan.Load() {
		t.Fatal("reentrant command was executed")
	}
}

func TestRunnerDoContextRejectsCrossRunnerCycle(t *testing.T) {
	a := NewRunner(9, 2)
	b := NewRunner(10, 2)
	a.Start()
	b.Start()
	defer a.Stop()
	defer b.Stop()

	var cycleErr error
	err := a.DoContext(context.Background(), func(aCtx context.Context) {
		if err := b.DoContext(aCtx, func(bCtx context.Context) {
			cycleErr = a.DoContext(bCtx, func(context.Context) {})
		}); err != nil {
			t.Errorf("call into runner b: %v", err)
		}
	})
	if err != nil {
		t.Fatalf("outer DoContext: %v", err)
	}
	if !errors.Is(cycleErr, ErrRunnerReentrantCall) {
		t.Fatalf("cycle call: got %v, want ErrRunnerReentrantCall", cycleErr)
	}
}

func TestRunnerPanicIsolationReturnsError(t *testing.T) {
	r := NewRunner(11, 0)
	r.Start()
	defer r.Stop()

	err := r.DoContext(context.Background(), func(context.Context) { panic("boom") })
	if !errors.Is(err, ErrRunnerCommandPanic) {
		t.Fatalf("panic result: got %v, want ErrRunnerCommandPanic", err)
	}

	var ok bool
	if err := r.DoContext(context.Background(), func(context.Context) { ok = true }); err != nil {
		t.Fatalf("command after panic: %v", err)
	}
	if !ok {
		t.Fatal("Runner did not continue after a command panic")
	}
}

func TestRunnerTryPostNeverBlocksAndReportsFull(t *testing.T) {
	r := NewRunner(12, 1)
	r.Start()
	defer r.Stop()

	entered := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_ = r.DoContext(context.Background(), func(context.Context) {
			close(entered)
			<-release
		})
	}()
	<-entered

	if err := r.TryPost(func(context.Context) {}); err != nil {
		t.Fatalf("first TryPost: %v", err)
	}
	start := time.Now()
	err := r.TryPost(func(context.Context) {})
	if !errors.Is(err, ErrRunnerQueueFull) {
		t.Fatalf("second TryPost: got %v, want ErrRunnerQueueFull", err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("TryPost blocked for %v", elapsed)
	}
	close(release)
}

func TestRunnerStopAndAdmissionRace(t *testing.T) {
	r := NewRunner(13, 32)
	r.Start()

	var accepted atomic.Int64
	var executed atomic.Int64
	start := make(chan struct{})
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 200; i++ {
				err := r.PostContext(context.Background(), func(context.Context) { executed.Add(1) })
				switch {
				case err == nil:
					accepted.Add(1)
				case errors.Is(err, ErrRunnerStopping), errors.Is(err, ErrRunnerStopped):
					return
				default:
					t.Errorf("unexpected PostContext error: %v", err)
					return
				}
			}
		}()
	}
	close(start)
	time.Sleep(time.Millisecond)
	if err := r.StopContext(context.Background()); err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	wg.Wait()

	if got, want := executed.Load(), accepted.Load(); got != want {
		t.Fatalf("executed accepted commands: got %d, want %d", got, want)
	}
	if r.Post(func() {}) {
		t.Fatal("legacy Post accepted a command after Stop")
	}
}

func TestRunnerStopIsIdempotentBeforeAndAfterStart(t *testing.T) {
	beforeStart := NewRunner(14, 1)
	if err := beforeStart.StopContext(context.Background()); err != nil {
		t.Fatalf("StopContext before Start: %v", err)
	}
	if err := beforeStart.StopContext(context.Background()); err != nil {
		t.Fatalf("second StopContext before Start: %v", err)
	}
	beforeStart.Start()
	if beforeStart.IsRunning() {
		t.Fatal("a stopped Runner restarted")
	}

	r := NewRunner(15, 1)
	r.Start()
	if err := r.StopContext(context.Background()); err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	if err := r.StopContext(context.Background()); err != nil {
		t.Fatalf("second StopContext: %v", err)
	}
	r.Stop()
}

func waitForRunnerError(t *testing.T, r *Runner, target error) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		err := r.TryPost(func(context.Context) {})
		if errors.Is(err, target) {
			return
		}
		if err == nil || errors.Is(err, ErrRunnerQueueFull) {
			time.Sleep(time.Millisecond)
			continue
		}
		t.Fatalf("waiting for %v: got %v", target, err)
	}
	t.Fatalf("Runner did not reach state returning %v", target)
}

func waitForRunnerQueueDepth(t *testing.T, r *Runner, depth int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(r.cmdCh) >= depth {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("Runner queue depth did not reach %d", depth)
}
