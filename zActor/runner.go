package zActor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"

	"github.com/pzqf/zEngine/zLog"
)

var (
	ErrRunnerNotStarted    = errors.New("runner not started")
	ErrRunnerStopping      = errors.New("runner stopping")
	ErrRunnerStopped       = errors.New("runner stopped")
	ErrRunnerQueueFull     = errors.New("runner queue full")
	ErrRunnerReentrantCall = errors.New("runner reentrant call")
	ErrRunnerCommandPanic  = errors.New("runner command panic")
	ErrRunnerNilCommand    = errors.New("runner command is nil")
)

type runnerState uint32

const (
	runnerStateNew runnerState = iota
	runnerStateRunning
	runnerStateStopping
	runnerStateStopped
)

type runnerCommand struct {
	ctx               context.Context
	fn                func(context.Context)
	done              chan error
	cancelBeforeStart bool
}

func (c runnerCommand) complete(err error) {
	if c.done != nil {
		c.done <- err
	}
}

type runnerExecutionKey struct{}

type runnerExecution struct {
	runner *Runner
	parent *runnerExecution
}

// Runner 是一个单写者闭包串行器。所有已接纳命令只在它独占的 goroutine 上执行。
type Runner struct {
	id int64

	mu      sync.Mutex
	state   atomic.Uint32
	cmdCh   chan runnerCommand
	spaceCh chan struct{}
	stopCh  chan struct{}
	doneCh  chan struct{}

	logger *zap.Logger
}

// NewRunner 创建 Runner。queueSize<=0 时使用默认容量 1024；Start 前不接纳命令。
func NewRunner(id int64, queueSize int) *Runner {
	if queueSize <= 0 {
		queueSize = 1024
	}
	return &Runner{
		id:      id,
		cmdCh:   make(chan runnerCommand, queueSize),
		spaceCh: make(chan struct{}, 1),
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
		logger:  zLog.GetLogger(),
	}
}

// Start 启动独占 goroutine。重复调用无副作用；已停止的 Runner 不支持重启。
func (r *Runner) Start() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if runnerState(r.state.Load()) != runnerStateNew {
		return
	}
	r.state.Store(uint32(runnerStateRunning))
	go r.run()
}

// IsRunning 仅在 Runner 仍接纳新命令时返回 true。
func (r *Runner) IsRunning() bool {
	return runnerState(r.state.Load()) == runnerStateRunning
}

// DoContext 把 fn 投递到 Runner goroutine，并等待执行完成、显式取消或 panic 结果。
//
// execCtx 记录当前 Runner 调用链。同步调用其它 Runner 时必须继续传递它；同 Runner 重入或
// A->B->A 环形调用会返回 ErrRunnerReentrantCall，而不是自投递死锁。ctx 取消只会撤销尚未
// 接纳或尚未开始的命令；一旦 fn 开始，DoContext 会等它真正结束后再返回。
func (r *Runner) DoContext(ctx context.Context, fn func(context.Context)) error {
	ctx = normalizeRunnerContext(ctx)
	if fn == nil {
		return ErrRunnerNilCommand
	}
	if runnerInExecutionChain(ctx, r) {
		return ErrRunnerReentrantCall
	}

	done := make(chan error, 1)
	cmd := runnerCommand{
		ctx:               ctx,
		fn:                fn,
		done:              done,
		cancelBeforeStart: true,
	}
	if err := r.admit(ctx, cmd, true); err != nil {
		return err
	}
	return <-done
}

// PostContext 异步接纳命令；队列满时等待容量、ctx 取消或 Runner 停止。
// 返回 nil 后命令属于 Runner，停止流程会把它执行完，不会静默丢弃。
func (r *Runner) PostContext(ctx context.Context, fn func(context.Context)) error {
	ctx = normalizeRunnerContext(ctx)
	if fn == nil {
		return ErrRunnerNilCommand
	}
	if runnerInExecutionChain(ctx, r) {
		return ErrRunnerReentrantCall
	}
	return r.admit(ctx, runnerCommand{ctx: ctx, fn: fn}, true)
}

// TryPost 非阻塞接纳命令。队列满返回 ErrRunnerQueueFull。
func (r *Runner) TryPost(fn func(context.Context)) error {
	if fn == nil {
		return ErrRunnerNilCommand
	}
	return r.admit(context.Background(), runnerCommand{
		ctx: context.Background(),
		fn:  fn,
	}, false)
}

// StopContext 原子关闭 admission，并等待所有已接纳命令执行完成和 goroutine 退出。
// ctx 只限制等待时间；超时后 Runner 仍会继续排空并最终进入 stopped。
func (r *Runner) StopContext(ctx context.Context) error {
	ctx = normalizeRunnerContext(ctx)
	if runnerInExecutionChain(ctx, r) {
		return ErrRunnerReentrantCall
	}

	r.mu.Lock()
	switch runnerState(r.state.Load()) {
	case runnerStateNew:
		r.state.Store(uint32(runnerStateStopped))
		close(r.stopCh)
		close(r.doneCh)
		r.mu.Unlock()
		return nil
	case runnerStateRunning:
		r.state.Store(uint32(runnerStateStopping))
		close(r.stopCh)
	case runnerStateStopping:
		// Another caller already owns the transition; wait on the same completion.
	case runnerStateStopped:
		r.mu.Unlock()
		return nil
	}
	done := r.doneCh
	r.mu.Unlock()

	select {
	case <-done:
		return nil
	default:
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Runner) admit(ctx context.Context, cmd runnerCommand, wait bool) error {
	for {
		r.mu.Lock()
		if err := runnerStateError(runnerState(r.state.Load())); err != nil {
			r.mu.Unlock()
			return err
		}
		if err := ctx.Err(); err != nil {
			r.mu.Unlock()
			return err
		}
		select {
		case r.cmdCh <- cmd:
			r.mu.Unlock()
			return nil
		default:
			r.mu.Unlock()
		}

		if !wait {
			return ErrRunnerQueueFull
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-r.stopCh:
			return runnerStateError(runnerState(r.state.Load()))
		case <-r.spaceCh:
		}
	}
}

func (r *Runner) run() {
	for {
		select {
		case <-r.stopCh:
			r.drainAndStop()
			return
		default:
		}

		select {
		case cmd := <-r.cmdCh:
			r.signalQueueSpace()
			r.execute(cmd)
		case <-r.stopCh:
			r.drainAndStop()
			return
		}
	}
}

func (r *Runner) drainAndStop() {
	for {
		select {
		case cmd := <-r.cmdCh:
			r.signalQueueSpace()
			r.execute(cmd)
		default:
			r.mu.Lock()
			r.state.Store(uint32(runnerStateStopped))
			close(r.doneCh)
			r.mu.Unlock()
			return
		}
	}
}

func (r *Runner) signalQueueSpace() {
	select {
	case r.spaceCh <- struct{}{}:
	default:
	}
}

func (r *Runner) execute(cmd runnerCommand) {
	if cmd.cancelBeforeStart {
		if err := cmd.ctx.Err(); err != nil {
			cmd.complete(err)
			return
		}
	}
	execCtx := withRunnerExecution(cmd.ctx, r)
	cmd.complete(r.safeExec(execCtx, cmd.fn))
}

func (r *Runner) safeExec(ctx context.Context, fn func(context.Context)) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("%w: %v", ErrRunnerCommandPanic, rec)
			if r.logger != nil {
				r.logger.Error("runner command panic recovered",
					zap.Int64("runner_id", r.id), zap.Any("panic", rec))
			}
		}
	}()
	fn(ctx)
	return nil
}

func runnerStateError(state runnerState) error {
	switch state {
	case runnerStateNew:
		return ErrRunnerNotStarted
	case runnerStateRunning:
		return nil
	case runnerStateStopping:
		return ErrRunnerStopping
	case runnerStateStopped:
		return ErrRunnerStopped
	default:
		return ErrRunnerStopped
	}
}

func normalizeRunnerContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func withRunnerExecution(ctx context.Context, r *Runner) context.Context {
	parent, _ := ctx.Value(runnerExecutionKey{}).(*runnerExecution)
	return context.WithValue(ctx, runnerExecutionKey{}, &runnerExecution{runner: r, parent: parent})
}

func runnerInExecutionChain(ctx context.Context, r *Runner) bool {
	execution, _ := ctx.Value(runnerExecutionKey{}).(*runnerExecution)
	for current := execution; current != nil; current = current.parent {
		if current.runner == r {
			return true
		}
	}
	return false
}
