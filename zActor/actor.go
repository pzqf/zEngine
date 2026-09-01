package zActor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/pzqf/zEngine/zLog"
)

var (
	ErrActorNotRunning                    = errors.New("actor not running")
	ErrActorStarting                      = errors.New("actor starting")
	ErrActorRunning                       = errors.New("actor running")
	ErrActorRestarting                    = errors.New("actor restarting")
	ErrActorStopping                      = errors.New("actor stopping")
	ErrActorStopped                       = errors.New("actor stopped")
	ErrActorMailboxFull                   = errors.New("actor mailbox full")
	ErrActorNilMessage                    = errors.New("actor message is nil")
	ErrActorInvalidPriority               = errors.New("actor message priority is invalid")
	ErrActorUnsupportedSupervisorStrategy = errors.New("actor supervisor strategy is unsupported")
	ErrActorLifecycleHookPanic            = errors.New("actor lifecycle hook panic")
)

type SupervisorStrategy int

const (
	SupervisorStrategyRestart SupervisorStrategy = iota
	SupervisorStrategyStop
	// SupervisorStrategyEscalate is retained for source compatibility only. BaseActor has no parent
	// supervision tree, so Start rejects this strategy instead of pretending that logging is escalation.
	// Deprecated: use Restart or Stop until a real parent supervisor has a production caller.
	SupervisorStrategyEscalate
)

type SupervisorConfig struct {
	Strategy       SupervisorStrategy
	MaxRestarts    int
	RestartWindow  time.Duration
	RestartBackoff time.Duration
}

func DefaultSupervisorConfig() SupervisorConfig {
	return SupervisorConfig{
		Strategy:       SupervisorStrategyRestart,
		MaxRestarts:    3,
		RestartWindow:  60 * time.Second,
		RestartBackoff: 1 * time.Second,
	}
}

type ActorMessage interface {
	GetActorID() int64
}

type BaseActorMessage struct {
	ActorID int64
}

func (msg *BaseActorMessage) GetActorID() int64 {
	return msg.ActorID
}

// PriorityActorMessage is the legacy standalone priority message. New callers should keep their own
// message type and pass it to SendPriority so ProcessMessage receives the original payload.
// Deprecated: use SendPriority.
type PriorityActorMessage struct {
	BaseActorMessage
	Priority MessagePriority
}

func (msg *PriorityActorMessage) GetPriority() MessagePriority {
	return msg.Priority
}

type MessagePriority int

const (
	PriorityHigh MessagePriority = iota
	PriorityNormal
	PriorityLow
)

type ActorState uint32

const (
	ActorStateNew ActorState = iota
	ActorStateStarting
	ActorStateRunning
	ActorStateRestarting
	ActorStateStopping
	ActorStateStopped
)

func (s ActorState) String() string {
	switch s {
	case ActorStateNew:
		return "new"
	case ActorStateStarting:
		return "starting"
	case ActorStateRunning:
		return "running"
	case ActorStateRestarting:
		return "restarting"
	case ActorStateStopping:
		return "stopping"
	case ActorStateStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

type Actor interface {
	ID() int64
	Start() error
	Stop() error
	SendMessage(msg ActorMessage) error
	ProcessMessage(msg ActorMessage)
	IsRunning() bool
}

type LifecycleHooks interface {
	OnStart() error
	OnStop()
	OnRestart() error
}

type ActorStats struct {
	State ActorState

	NormalDepth int
	HighDepth   int
	LowDepth    int
	LatestDepth int
	Capacity    int

	Accepted       uint64
	RejectedFull   uint64
	RejectedState  uint64
	LatestReplaced uint64
	Panics         uint64
	Restarts       uint64
	Abandoned      uint64
	HookFailures   uint64
}

const (
	maxHighPriorityBurst = 8
	maxNonLowBurst       = 16
)

type actorRun struct {
	stopCh     chan struct{}
	doneCh     chan struct{}
	stopOnce   sync.Once
	finishOnce sync.Once
	hooks      LifecycleHooks
}

func newActorRun(hooks LifecycleHooks) *actorRun {
	return &actorRun{stopCh: make(chan struct{}), doneCh: make(chan struct{}), hooks: hooks}
}

func (r *actorRun) requestStop() {
	r.stopOnce.Do(func() { close(r.stopCh) })
}

type BaseActor struct {
	id int64

	// ActorMsgChan is the legacy normal-priority mailbox. Direct sends bypass lifecycle admission and
	// metrics; production callers must use SendMessage. It remains temporarily for source compatibility.
	// Deprecated: use SendMessage.
	ActorMsgChan chan ActorMessage
	highPriority chan ActorMessage
	lowPriority  chan ActorMessage
	latest       chan ActorMessage

	state atomic.Uint32
	mu    sync.Mutex

	logger        *zap.Logger
	supervisorCfg SupervisorConfig
	restartTimes  []time.Time
	hooks         LifecycleHooks
	self          Actor
	current       *actorRun

	accepted       atomic.Uint64
	rejectedFull   atomic.Uint64
	rejectedState  atomic.Uint64
	latestReplaced atomic.Uint64
	panics         atomic.Uint64
	restarts       atomic.Uint64
	abandoned      atomic.Uint64
	hookFailures   atomic.Uint64
}

func NewBaseActor(id int64, chanSize int) *BaseActor {
	return newBaseActor(id, chanSize, DefaultSupervisorConfig())
}

func NewBaseActorWithSupervisor(id int64, chanSize int, cfg SupervisorConfig) *BaseActor {
	return newBaseActor(id, chanSize, cfg)
}

func newBaseActor(id int64, chanSize int, cfg SupervisorConfig) *BaseActor {
	if chanSize <= 0 {
		chanSize = 1024
	}
	return &BaseActor{
		id:            id,
		ActorMsgChan:  make(chan ActorMessage, chanSize),
		highPriority:  make(chan ActorMessage, chanSize),
		lowPriority:   make(chan ActorMessage, chanSize),
		latest:        make(chan ActorMessage, 1),
		logger:        zLog.GetLogger(),
		supervisorCfg: cfg,
	}
}

// SetLifecycleHooks configures construction-time hooks. Replacing hooks while an actor is active is
// supported for race safety, but the current lifecycle transition uses the snapshot it started with.
func (a *BaseActor) SetLifecycleHooks(hooks LifecycleHooks) {
	a.mu.Lock()
	a.hooks = hooks
	a.mu.Unlock()
}

func (a *BaseActor) ID() int64 {
	return a.id
}

// SetSelf configures the concrete message dispatcher used by subsequent runs.
func (a *BaseActor) SetSelf(actor Actor) {
	a.mu.Lock()
	a.self = actor
	a.mu.Unlock()
}

func (a *BaseActor) Start() error {
	a.mu.Lock()
	state := ActorState(a.state.Load())
	switch state {
	case ActorStateNew, ActorStateStopped:
		// A completed generation can be started again without overlapping its old consumer.
	case ActorStateStarting:
		a.mu.Unlock()
		return ErrActorStarting
	case ActorStateStopping:
		a.mu.Unlock()
		return ErrActorStopping
	case ActorStateRunning:
		a.mu.Unlock()
		return ErrActorRunning
	case ActorStateRestarting:
		a.mu.Unlock()
		return ErrActorRestarting
	default:
		a.mu.Unlock()
		return ErrActorNotRunning
	}
	if !supportedSupervisorStrategy(a.supervisorCfg.Strategy) {
		a.mu.Unlock()
		return fmt.Errorf("actor %d: %w: %d", a.id, ErrActorUnsupportedSupervisorStrategy, a.supervisorCfg.Strategy)
	}

	hooks := a.hooks
	run := newActorRun(hooks)
	a.current = run
	a.restartTimes = nil
	a.state.Store(uint32(ActorStateStarting))
	self := a.self
	a.mu.Unlock()

	if err := a.callOnStart(hooks); err != nil {
		a.failStart(run)
		return fmt.Errorf("actor %d OnStart failed: %w", a.id, err)
	}

	a.mu.Lock()
	if a.current != run || ActorState(a.state.Load()) != ActorStateStarting {
		err := actorStateAdmissionError(ActorState(a.state.Load()))
		a.mu.Unlock()
		if err == nil {
			err = ErrActorStarting
		}
		return err
	}
	a.state.Store(uint32(ActorStateRunning))
	a.mu.Unlock()

	go a.run(run, a.processFunc(self))
	return nil
}

// Stop closes admission and requests an orderly drain. It preserves the legacy non-waiting behavior;
// callers that own actor teardown should use StopContext.
func (a *BaseActor) Stop() error {
	_, err := a.beginStop()
	return err
}

// StopContext closes admission, drains messages accepted by a normally running actor, and waits for
// OnStop plus the consumer goroutine to finish. A context timeout only stops waiting, not actor shutdown.
func (a *BaseActor) StopContext(ctx context.Context) error {
	ctx = normalizeActorContext(ctx)
	run, err := a.beginStop()
	if err != nil {
		return err
	}
	select {
	case <-run.doneCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// WaitStopped waits for the current generation without initiating a stop.
func (a *BaseActor) WaitStopped(ctx context.Context) error {
	ctx = normalizeActorContext(ctx)
	a.mu.Lock()
	state := ActorState(a.state.Load())
	if state == ActorStateStopped {
		a.mu.Unlock()
		return nil
	}
	if state == ActorStateNew || a.current == nil {
		a.mu.Unlock()
		return ErrActorNotRunning
	}
	done := a.current.doneCh
	a.mu.Unlock()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *BaseActor) beginStop() (*actorRun, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	switch ActorState(a.state.Load()) {
	case ActorStateRunning, ActorStateRestarting:
		a.state.Store(uint32(ActorStateStopping))
		run := a.current
		run.requestStop()
		return run, nil
	case ActorStateStarting:
		return nil, ErrActorStarting
	case ActorStateStopping:
		return nil, ErrActorStopping
	case ActorStateStopped:
		return nil, ErrActorStopped
	default:
		return nil, ErrActorNotRunning
	}
}

// SendMessage performs non-blocking normal admission, unless msg is the legacy PriorityActorMessage.
// A nil error means ownership transferred to the actor; full and lifecycle rejection are explicit.
func (a *BaseActor) SendMessage(msg ActorMessage) error {
	if msg == nil {
		return ErrActorNilMessage
	}
	priority := PriorityNormal
	if prioritized, ok := msg.(interface{ GetPriority() MessagePriority }); ok {
		priority = prioritized.GetPriority()
	}
	return a.send(msg, priority)
}

// SendPriority admits the original message into the requested priority lane.
func (a *BaseActor) SendPriority(msg ActorMessage, priority MessagePriority) error {
	if msg == nil {
		return ErrActorNilMessage
	}
	return a.send(msg, priority)
}

func (a *BaseActor) send(msg ActorMessage, priority MessagePriority) error {
	mailbox, err := a.mailbox(priority)
	if err != nil {
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if err := actorStateAdmissionError(ActorState(a.state.Load())); err != nil {
		a.rejectedState.Add(1)
		return err
	}
	select {
	case mailbox <- msg:
		a.accepted.Add(1)
		return nil
	default:
		a.rejectedFull.Add(1)
		return ErrActorMailboxFull
	}
}

// SendLatest uses a dedicated one-message lane. A successful replacement is expected loss and is
// reported separately from mailbox rejection.
func (a *BaseActor) SendLatest(msg ActorMessage) (replaced bool, err error) {
	if msg == nil {
		return false, ErrActorNilMessage
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := actorStateAdmissionError(ActorState(a.state.Load())); err != nil {
		a.rejectedState.Add(1)
		return false, err
	}
	select {
	case <-a.latest:
		replaced = true
		a.latestReplaced.Add(1)
	default:
	}
	a.latest <- msg
	a.accepted.Add(1)
	return replaced, nil
}

func (a *BaseActor) mailbox(priority MessagePriority) (chan ActorMessage, error) {
	switch priority {
	case PriorityHigh:
		return a.highPriority, nil
	case PriorityNormal:
		return a.ActorMsgChan, nil
	case PriorityLow:
		return a.lowPriority, nil
	default:
		return nil, ErrActorInvalidPriority
	}
}

func (a *BaseActor) ProcessMessage(msg ActorMessage) {
	a.logger.Debug("BaseActor received message", zap.Int64("actor_id", a.id), zap.Any("message", msg))
}

func (a *BaseActor) IsRunning() bool {
	state := ActorState(a.state.Load())
	return state == ActorStateRunning || state == ActorStateRestarting
}

// DroppedMessages is retained for compatibility and includes explicit full rejections plus messages
// abandoned after an unrecoverable actor failure.
func (a *BaseActor) DroppedMessages() uint64 {
	return a.rejectedFull.Load() + a.abandoned.Load()
}

func (a *BaseActor) Stats() ActorStats {
	return ActorStats{
		State:          ActorState(a.state.Load()),
		NormalDepth:    len(a.ActorMsgChan),
		HighDepth:      len(a.highPriority),
		LowDepth:       len(a.lowPriority),
		LatestDepth:    len(a.latest),
		Capacity:       cap(a.ActorMsgChan),
		Accepted:       a.accepted.Load(),
		RejectedFull:   a.rejectedFull.Load(),
		RejectedState:  a.rejectedState.Load(),
		LatestReplaced: a.latestReplaced.Load(),
		Panics:         a.panics.Load(),
		Restarts:       a.restarts.Load(),
		Abandoned:      a.abandoned.Load(),
		HookFailures:   a.hookFailures.Load(),
	}
}

type actorScheduler struct {
	highBurst   int
	nonLowBurst int
}

type actorLane uint8

const (
	actorLaneHigh actorLane = iota
	actorLaneNormal
	actorLaneLow
	actorLaneLatest
)

func (a *BaseActor) run(run *actorRun, processFn func(ActorMessage)) {
	a.logger.Info("Actor started", zap.Int64("actor_id", a.id))
	defer func() {
		if recovered := recover(); recovered != nil {
			a.panics.Add(1)
			a.logger.Error("Actor panic recovered", zap.Int64("actor_id", a.id), zap.Any("panic", recovered))
			a.handlePanic(run, processFn)
		}
	}()

	scheduler := actorScheduler{}
	for {
		select {
		case <-run.stopCh:
			a.drainAndFinish(run, processFn, &scheduler)
			return
		default:
		}
		if msg, lane, ok := a.tryReceiveFair(&scheduler); ok {
			scheduler.record(lane)
			processFn(msg)
			continue
		}

		select {
		case <-run.stopCh:
			a.drainAndFinish(run, processFn, &scheduler)
			return
		case msg := <-a.highPriority:
			scheduler.record(actorLaneHigh)
			processFn(msg)
		case msg := <-a.ActorMsgChan:
			scheduler.record(actorLaneNormal)
			processFn(msg)
		case msg := <-a.lowPriority:
			scheduler.record(actorLaneLow)
			processFn(msg)
		case msg := <-a.latest:
			scheduler.record(actorLaneLatest)
			processFn(msg)
		}
	}
}

func (a *BaseActor) drainAndFinish(run *actorRun, processFn func(ActorMessage), scheduler *actorScheduler) {
	for {
		msg, lane, ok := a.tryReceiveFair(scheduler)
		if !ok {
			a.finishStop(run)
			return
		}
		scheduler.record(lane)
		processFn(msg)
	}
}

func (a *BaseActor) tryReceiveFair(scheduler *actorScheduler) (ActorMessage, actorLane, bool) {
	if scheduler.nonLowBurst >= maxNonLowBurst {
		if msg, ok := tryReceive(a.lowPriority); ok {
			return msg, actorLaneLow, true
		}
		if msg, ok := tryReceive(a.latest); ok {
			return msg, actorLaneLatest, true
		}
	}
	if scheduler.highBurst >= maxHighPriorityBurst {
		if msg, ok := tryReceive(a.ActorMsgChan); ok {
			return msg, actorLaneNormal, true
		}
		if msg, ok := tryReceive(a.lowPriority); ok {
			return msg, actorLaneLow, true
		}
		if msg, ok := tryReceive(a.latest); ok {
			return msg, actorLaneLatest, true
		}
	}
	if msg, ok := tryReceive(a.highPriority); ok {
		return msg, actorLaneHigh, true
	}
	if msg, ok := tryReceive(a.ActorMsgChan); ok {
		return msg, actorLaneNormal, true
	}
	if msg, ok := tryReceive(a.lowPriority); ok {
		return msg, actorLaneLow, true
	}
	if msg, ok := tryReceive(a.latest); ok {
		return msg, actorLaneLatest, true
	}
	return nil, actorLaneNormal, false
}

func tryReceive(ch <-chan ActorMessage) (ActorMessage, bool) {
	select {
	case msg := <-ch:
		return msg, true
	default:
		return nil, false
	}
}

func (s *actorScheduler) record(lane actorLane) {
	switch lane {
	case actorLaneHigh:
		s.highBurst++
		s.nonLowBurst++
	case actorLaneNormal:
		s.highBurst = 0
		s.nonLowBurst++
	case actorLaneLow, actorLaneLatest:
		s.highBurst = 0
		s.nonLowBurst = 0
	}
}

func (a *BaseActor) handlePanic(run *actorRun, processFn func(ActorMessage)) {
	a.mu.Lock()
	if a.current != run {
		a.mu.Unlock()
		return
	}
	state := ActorState(a.state.Load())
	if state == ActorStateStopping {
		a.mu.Unlock()
		a.drainAfterPanicStop(run, processFn)
		return
	}

	switch a.supervisorCfg.Strategy {
	case SupervisorStrategyStop:
		a.state.Store(uint32(ActorStateStopping))
		run.requestStop()
		a.mu.Unlock()
		a.abandonMailboxes()
		a.finishStop(run)
	case SupervisorStrategyRestart:
		now := time.Now()
		windowStart := now.Add(-a.supervisorCfg.RestartWindow)
		recent := a.restartTimes[:0]
		for _, restartedAt := range a.restartTimes {
			if restartedAt.After(windowStart) {
				recent = append(recent, restartedAt)
			}
		}
		a.restartTimes = recent
		if len(recent) >= a.supervisorCfg.MaxRestarts {
			a.state.Store(uint32(ActorStateStopping))
			run.requestStop()
			a.mu.Unlock()
			a.logger.Error("Actor exceeded max restarts, giving up", zap.Int64("actor_id", a.id),
				zap.Int("max_restarts", a.supervisorCfg.MaxRestarts))
			a.abandonMailboxes()
			a.finishStop(run)
			return
		}
		a.state.Store(uint32(ActorStateRestarting))
		backoff := a.supervisorCfg.RestartBackoff
		a.mu.Unlock()

		timer := time.NewTimer(backoff)
		defer timer.Stop()
		select {
		case <-run.stopCh:
			a.drainAfterPanicStop(run, processFn)
			return
		case <-timer.C:
		}

		if err := a.callOnRestart(run.hooks); err != nil {
			a.logger.Error("Actor OnRestart failed", zap.Int64("actor_id", a.id), zap.Error(err))
			a.transitionRestartFailure(run)
			return
		}

		a.mu.Lock()
		if a.current != run {
			a.mu.Unlock()
			return
		}
		if ActorState(a.state.Load()) == ActorStateStopping {
			a.mu.Unlock()
			a.drainAfterPanicStop(run, processFn)
			return
		}
		if ActorState(a.state.Load()) != ActorStateRestarting {
			a.mu.Unlock()
			a.abandonMailboxes()
			a.finishStop(run)
			return
		}
		a.restartTimes = append(a.restartTimes, time.Now())
		restartCount := len(a.restartTimes)
		a.restarts.Add(1)
		a.state.Store(uint32(ActorStateRunning))
		a.mu.Unlock()

		go a.run(run, processFn)
		a.logger.Info("Actor restarted", zap.Int64("actor_id", a.id), zap.Int("restart_count", restartCount))
	default:
		a.state.Store(uint32(ActorStateStopping))
		run.requestStop()
		a.mu.Unlock()
		a.abandonMailboxes()
		a.finishStop(run)
	}
}

func (a *BaseActor) transitionRestartFailure(run *actorRun) {
	a.mu.Lock()
	if a.current == run && ActorState(a.state.Load()) == ActorStateRestarting {
		a.state.Store(uint32(ActorStateStopping))
		run.requestStop()
	}
	a.mu.Unlock()
	a.abandonMailboxes()
	a.finishStop(run)
}

func (a *BaseActor) drainAfterPanicStop(run *actorRun, processFn func(ActorMessage)) {
	scheduler := actorScheduler{}
	defer func() {
		if recovered := recover(); recovered != nil {
			a.panics.Add(1)
			a.logger.Error("Actor panic while draining after stop", zap.Int64("actor_id", a.id),
				zap.Any("panic", recovered))
			a.abandonMailboxes()
			a.finishStop(run)
		}
	}()
	a.drainAndFinish(run, processFn, &scheduler)
}

func (a *BaseActor) abandonMailboxes() {
	var count uint64
	for {
		drained := false
		for _, mailbox := range []chan ActorMessage{a.highPriority, a.ActorMsgChan, a.lowPriority, a.latest} {
			select {
			case <-mailbox:
				count++
				drained = true
			default:
			}
		}
		if !drained {
			break
		}
	}
	a.abandoned.Add(count)
}

func (a *BaseActor) finishStop(run *actorRun) {
	run.finishOnce.Do(func() {
		a.callOnStop(run.hooks)

		a.mu.Lock()
		if a.current == run {
			a.state.Store(uint32(ActorStateStopped))
		}
		close(run.doneCh)
		a.mu.Unlock()
		a.logger.Info("Actor stopped", zap.Int64("actor_id", a.id))
	})
}

func (a *BaseActor) failStart(run *actorRun) {
	a.mu.Lock()
	if a.current == run && ActorState(a.state.Load()) == ActorStateStarting {
		a.state.Store(uint32(ActorStateStopping))
		run.requestStop()
	}
	a.mu.Unlock()
	a.callOnStop(run.hooks)
	a.mu.Lock()
	if a.current == run {
		a.state.Store(uint32(ActorStateStopped))
	}
	close(run.doneCh)
	a.mu.Unlock()
}

func (a *BaseActor) processFunc(self Actor) func(ActorMessage) {
	if self != nil {
		return self.ProcessMessage
	}
	a.logger.Warn("Actor has no self set; messages will use BaseActor.ProcessMessage default (missing SetSelf?)",
		zap.Int64("actor_id", a.id))
	return a.ProcessMessage
}

func (a *BaseActor) callOnStart(hooks LifecycleHooks) (err error) {
	if hooks == nil {
		return nil
	}
	defer a.recoverHookPanic("OnStart", &err)
	err = hooks.OnStart()
	if err != nil {
		a.hookFailures.Add(1)
	}
	return err
}

func (a *BaseActor) callOnRestart(hooks LifecycleHooks) (err error) {
	if hooks == nil {
		return nil
	}
	defer a.recoverHookPanic("OnRestart", &err)
	err = hooks.OnRestart()
	if err != nil {
		a.hookFailures.Add(1)
	}
	return err
}

func (a *BaseActor) callOnStop(hooks LifecycleHooks) {
	if hooks == nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			a.hookFailures.Add(1)
			a.logger.Error("Actor lifecycle hook panic", zap.Int64("actor_id", a.id),
				zap.String("hook", "OnStop"), zap.Any("panic", recovered))
		}
	}()
	hooks.OnStop()
}

func (a *BaseActor) recoverHookPanic(hook string, err *error) {
	if recovered := recover(); recovered != nil {
		a.hookFailures.Add(1)
		*err = fmt.Errorf("%w: %s: %v", ErrActorLifecycleHookPanic, hook, recovered)
	}
}

func supportedSupervisorStrategy(strategy SupervisorStrategy) bool {
	return strategy == SupervisorStrategyRestart || strategy == SupervisorStrategyStop
}

func actorStateAdmissionError(state ActorState) error {
	switch state {
	case ActorStateRunning, ActorStateRestarting:
		return nil
	case ActorStateStarting:
		return ErrActorStarting
	case ActorStateStopping:
		return ErrActorStopping
	case ActorStateStopped:
		return ErrActorStopped
	default:
		return ErrActorNotRunning
	}
}

func normalizeActorContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
