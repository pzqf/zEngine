package zActor

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/pzqf/zEngine/zLog"
)

type SupervisorStrategy int

const (
	SupervisorStrategyRestart   SupervisorStrategy = iota
	SupervisorStrategyStop
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

type PriorityActorMessage struct {
	BaseActorMessage
	Priority MessagePriority
}

func (msg *PriorityActorMessage) GetPriority() MessagePriority {
	return msg.Priority
}

type MessagePriority int

const (
	PriorityHigh   MessagePriority = 0
	PriorityNormal MessagePriority = 1
	PriorityLow    MessagePriority = 2
)

type Actor interface {
	ID() int64
	Start() error
	Stop() error
	SendMessage(msg ActorMessage)
	ProcessMessage(msg ActorMessage)
	IsRunning() bool
}

type LifecycleHooks interface {
	OnStart() error
	OnStop()
	OnRestart() error
}

type BaseActor struct {
	id            int64
	ActorMsgChan  chan ActorMessage
	highPriority  chan ActorMessage
	running       atomic.Bool
	mu            sync.Mutex
	logger        *zap.Logger
	supervisorCfg SupervisorConfig
	restartTimes  []time.Time
	hooks         LifecycleHooks
	stopCh        chan struct{}
	self          Actor
}

func NewBaseActor(id int64, chanSize int) *BaseActor {
	if chanSize <= 0 {
		chanSize = 1024
	}
	return &BaseActor{
		id:            id,
		ActorMsgChan:  make(chan ActorMessage, chanSize),
		highPriority:  make(chan ActorMessage, chanSize),
		logger:        zLog.GetLogger(),
		supervisorCfg: DefaultSupervisorConfig(),
		stopCh:        make(chan struct{}),
	}
}

func NewBaseActorWithSupervisor(id int64, chanSize int, cfg SupervisorConfig) *BaseActor {
	if chanSize <= 0 {
		chanSize = 1024
	}
	return &BaseActor{
		id:            id,
		ActorMsgChan:  make(chan ActorMessage, chanSize),
		highPriority:  make(chan ActorMessage, chanSize),
		logger:        zLog.GetLogger(),
		supervisorCfg: cfg,
		stopCh:        make(chan struct{}),
	}
}

func (a *BaseActor) SetLifecycleHooks(hooks LifecycleHooks) {
	a.hooks = hooks
}

func (a *BaseActor) ID() int64 {
	return a.id
}

func (a *BaseActor) SetSelf(actor Actor) {
	a.self = actor
}

func (a *BaseActor) Start() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.running.Load() {
		return fmt.Errorf("actor %d is already running", a.id)
	}

	a.running.Store(true)
	a.stopCh = make(chan struct{})

	if a.hooks != nil {
		if err := a.hooks.OnStart(); err != nil {
			a.running.Store(false)
			return fmt.Errorf("actor %d OnStart failed: %w", a.id, err)
		}
	}

	go a.run()
	return nil
}

func (a *BaseActor) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.running.Load() {
		return fmt.Errorf("actor %d is not running", a.id)
	}

	a.running.Store(false)
	close(a.ActorMsgChan)
	close(a.highPriority)
	close(a.stopCh)

	if a.hooks != nil {
		a.hooks.OnStop()
	}

	return nil
}

func (a *BaseActor) SendMessage(msg ActorMessage) {
	if !a.IsRunning() {
		a.logger.Warn("Sending message to stopped actor", zap.Int64("actor_id", a.id))
		return
	}

	priority := PriorityNormal
	if pm, ok := msg.(*PriorityActorMessage); ok {
		priority = pm.Priority
	}

	switch priority {
	case PriorityHigh:
		select {
		case a.highPriority <- msg:
		default:
			a.logger.Warn("Actor high priority queue is full", zap.Int64("actor_id", a.id))
		}
	default:
		select {
		case a.ActorMsgChan <- msg:
		default:
			a.logger.Warn("Actor message queue is full", zap.Int64("actor_id", a.id))
		}
	}
}

func (a *BaseActor) SendPriority(msg ActorMessage, priority MessagePriority) {
	if !a.IsRunning() {
		a.logger.Warn("Sending priority message to stopped actor", zap.Int64("actor_id", a.id))
		return
	}

	pMsg := &PriorityActorMessage{
		BaseActorMessage: BaseActorMessage{ActorID: a.id},
		Priority:         priority,
	}

	switch priority {
	case PriorityHigh:
		select {
		case a.highPriority <- pMsg:
		default:
			a.logger.Warn("Actor high priority queue is full", zap.Int64("actor_id", a.id))
		}
	default:
		select {
		case a.ActorMsgChan <- pMsg:
		default:
			a.logger.Warn("Actor message queue is full", zap.Int64("actor_id", a.id))
		}
	}
}

func (a *BaseActor) ProcessMessage(msg ActorMessage) {
	a.logger.Debug("BaseActor received message", zap.Int64("actor_id", a.id), zap.Any("message", msg))
}

func (a *BaseActor) IsRunning() bool {
	return a.running.Load()
}

func (a *BaseActor) run() {
	a.logger.Info("Actor started", zap.Int64("actor_id", a.id))

	defer func() {
		if r := recover(); r != nil {
			a.logger.Error("Actor panic recovered",
				zap.Int64("actor_id", a.id),
				zap.Any("panic", r))
			a.handlePanic()
		}
	}()

	processFn := a.ProcessMessage
	if a.self != nil {
		processFn = a.self.ProcessMessage
	}

	for {
		select {
		case <-a.stopCh:
			a.logger.Info("Actor stopped via stop channel", zap.Int64("actor_id", a.id))
			return
		case msg, ok := <-a.highPriority:
			if !ok {
				return
			}
			processFn(msg)
		default:
			select {
			case msg, ok := <-a.highPriority:
				if !ok {
					return
				}
				processFn(msg)
			case msg, ok := <-a.ActorMsgChan:
				if !ok {
					return
				}
				processFn(msg)
			}
		}
	}
}

func (a *BaseActor) handlePanic() {
	a.running.Store(false)

	switch a.supervisorCfg.Strategy {
	case SupervisorStrategyRestart:
		a.tryRestart()
	case SupervisorStrategyStop:
		a.logger.Info("Actor stopped by supervisor strategy", zap.Int64("actor_id", a.id))
	case SupervisorStrategyEscalate:
		a.logger.Error("Actor panic escalated", zap.Int64("actor_id", a.id))
	}
}

func (a *BaseActor) tryRestart() {
	now := time.Now()
	windowStart := now.Add(-a.supervisorCfg.RestartWindow)
	var recentRestarts []time.Time
	for _, t := range a.restartTimes {
		if t.After(windowStart) {
			recentRestarts = append(recentRestarts, t)
		}
	}
	a.restartTimes = recentRestarts

	if len(recentRestarts) >= a.supervisorCfg.MaxRestarts {
		a.logger.Error("Actor exceeded max restarts",
			zap.Int64("actor_id", a.id),
			zap.Int("max_restarts", a.supervisorCfg.MaxRestarts))
		return
	}

	time.Sleep(a.supervisorCfg.RestartBackoff)

	a.mu.Lock()
	a.ActorMsgChan = make(chan ActorMessage, cap(a.ActorMsgChan))
	a.highPriority = make(chan ActorMessage, cap(a.highPriority))
	a.stopCh = make(chan struct{})
	a.running.Store(true)
	a.restartTimes = append(a.restartTimes, time.Now())
	a.mu.Unlock()

	if a.hooks != nil {
		if err := a.hooks.OnRestart(); err != nil {
			a.logger.Error("Actor OnRestart failed",
				zap.Int64("actor_id", a.id),
				zap.Error(err))
			a.running.Store(false)
			return
		}
	}

	go a.run()
	a.logger.Info("Actor restarted",
		zap.Int64("actor_id", a.id),
		zap.Int("restart_count", len(a.restartTimes)))
}
