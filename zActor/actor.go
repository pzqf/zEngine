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
	dropped       atomic.Uint64
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
	a.restartTimes = nil // 显式启动重置重启预算

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
	// 只关闭 stopCh 作为退出信号；不关闭消息 channel。
	// 关闭消息 channel 会与并发的 SendMessage 形成 send-on-closed panic（TOCTOU）。
	// 未消费的缓冲消息随 actor 停止一并丢弃并由 GC 回收。
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
			a.dropped.Add(1)
			a.logger.Warn("Actor high priority queue is full, message dropped",
				zap.Int64("actor_id", a.id), zap.Uint64("dropped_total", a.dropped.Load()))
		}
	default:
		select {
		case a.ActorMsgChan <- msg:
		default:
			a.dropped.Add(1)
			a.logger.Warn("Actor message queue is full, message dropped",
				zap.Int64("actor_id", a.id), zap.Uint64("dropped_total", a.dropped.Load()))
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
			a.dropped.Add(1)
			a.logger.Warn("Actor high priority queue is full, message dropped",
				zap.Int64("actor_id", a.id), zap.Uint64("dropped_total", a.dropped.Load()))
		}
	default:
		select {
		case a.ActorMsgChan <- pMsg:
		default:
			a.dropped.Add(1)
			a.logger.Warn("Actor message queue is full, message dropped",
				zap.Int64("actor_id", a.id), zap.Uint64("dropped_total", a.dropped.Load()))
		}
	}
}

func (a *BaseActor) ProcessMessage(msg ActorMessage) {
	a.logger.Debug("BaseActor received message", zap.Int64("actor_id", a.id), zap.Any("message", msg))
}

func (a *BaseActor) IsRunning() bool {
	return a.running.Load()
}

// DroppedMessages 返回因邮箱满而被丢弃的消息累计数，用于监控/告警。
func (a *BaseActor) DroppedMessages() uint64 {
	return a.dropped.Load()
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
	} else {
		// 未调用 SetSelf 时，嵌入 BaseActor 的子类重写的 ProcessMessage 不会被分派，
		// 只会走 BaseActor 的空实现——显式告警，避免静默退化难以排查。
		a.logger.Warn("Actor has no self set; messages will use BaseActor.ProcessMessage default (missing SetSelf?)",
			zap.Int64("actor_id", a.id))
	}

	for {
		// 高优先级优先：先非阻塞排空 highPriority；同时始终监听 stopCh。
		select {
		case <-a.stopCh:
			a.logger.Info("Actor stopped via stop channel", zap.Int64("actor_id", a.id))
			return
		case msg := <-a.highPriority:
			processFn(msg)
			continue
		default:
		}

		// 无高优先消息时阻塞等待，stopCh 始终在选择集内，保证停止能被及时感知。
		select {
		case <-a.stopCh:
			a.logger.Info("Actor stopped via stop channel", zap.Int64("actor_id", a.id))
			return
		case msg := <-a.highPriority:
			processFn(msg)
		case msg := <-a.ActorMsgChan:
			processFn(msg)
		}
	}
}

func (a *BaseActor) handlePanic() {
	switch a.supervisorCfg.Strategy {
	case SupervisorStrategyRestart:
		// 保持 running=true 跨越重启窗口：panic 的 run() 已退出，但 actor 语义上仍存活、
		// 正在重启。这样退避期间的 SendMessage 会缓冲（不丢弃），且并发 Stop() 不会因
		// "not running" 被拒—— Stop 能关闭 stopCh，tryRestart 据此放弃重启，停止意图得以生效。
		a.tryRestart()
	case SupervisorStrategyStop:
		a.running.Store(false)
		a.logger.Info("Actor stopped by supervisor strategy", zap.Int64("actor_id", a.id))
	case SupervisorStrategyEscalate:
		a.running.Store(false)
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
		a.running.Store(false)
		a.logger.Error("Actor exceeded max restarts, giving up",
			zap.Int64("actor_id", a.id),
			zap.Int("max_restarts", a.supervisorCfg.MaxRestarts))
		return
	}

	time.Sleep(a.supervisorCfg.RestartBackoff)

	a.mu.Lock()
	// 若退避期间 Stop() 已关闭 stopCh，尊重停止意图、放弃重启。
	select {
	case <-a.stopCh:
		a.running.Store(false)
		a.mu.Unlock()
		a.logger.Info("Actor restart aborted: stopped during backoff", zap.Int64("actor_id", a.id))
		return
	default:
	}
	// 不重建 ActorMsgChan/highPriority/stopCh：panic（来自 ProcessMessage）并未关闭它们，
	// 重建会（1）丢弃邮箱里所有在途消息且不计入 dropped，（2）与无锁读取这些字段的并发
	// SendMessage/run() 形成数据竞争。复用既有 channel 即可消除两者。
	a.restartTimes = append(a.restartTimes, time.Now())
	restartCount := len(a.restartTimes)
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
		zap.Int("restart_count", restartCount))
}
