package zActor

import (
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/pzqf/zEngine/zLog"
)

// ActorMessage Actor消息接口
type ActorMessage interface {
	GetActorID() int64
}

// BaseActorMessage 基础Actor消息实现
type BaseActorMessage struct {
	ActorID int64
}

// GetActorID 获取ActorID
func (msg *BaseActorMessage) GetActorID() int64 {
	return msg.ActorID
}

// Actor 接口定义
type Actor interface {
	// ID 获取Actor唯一标识
	ID() int64
	// Start 启动Actor
	Start() error
	// Stop 停止Actor
	Stop() error
	// SendMessage 发送消息给Actor
	SendMessage(msg ActorMessage)
	// ProcessMessage 处理消息
	ProcessMessage(msg ActorMessage)
	// IsRunning 检查Actor是否在运行
	IsRunning() bool
}

// BaseActor 基础Actor实现
type BaseActor struct {
	id        int64
	msgChan   chan ActorMessage
	isRunning bool
	mu        sync.Mutex
	logger    *zap.Logger
}

// NewBaseActor 创建基础Actor实例
func NewBaseActor(id int64, chanSize int) *BaseActor {
	if chanSize <= 0 {
		chanSize = 100
	}
	return &BaseActor{
		id:        id,
		msgChan:   make(chan ActorMessage, chanSize),
		isRunning: false,
		logger:    zLog.GetLogger(),
	}
}

// ID 获取Actor唯一标识
func (a *BaseActor) ID() int64 {
	return a.id
}

// Start 启动Actor
func (a *BaseActor) Start() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.isRunning {
		return fmt.Errorf("actor %d is already running", a.id)
	}

	a.isRunning = true
	go a.run()
	return nil
}

// Stop 停止Actor
func (a *BaseActor) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.isRunning {
		return fmt.Errorf("actor %d is not running", a.id)
	}

	a.isRunning = false
	close(a.msgChan)
	return nil
}

// SendMessage 发送消息给Actor
func (a *BaseActor) SendMessage(msg ActorMessage) {
	if !a.IsRunning() {
		a.logger.Warn("Sending message to stopped actor", zap.Int64("actor_id", a.id))
		return
	}

	select {
	case a.msgChan <- msg:
		// 消息发送成功
	default:
		// 消息队列已满，丢弃消息或进行其他处理
		a.logger.Warn("Actor message queue is full", zap.Int64("actor_id", a.id))
	}
}

// ProcessMessage 处理消息，需要被子类重写
func (a *BaseActor) ProcessMessage(msg ActorMessage) {
	// 默认实现，需要被子类重写
	a.logger.Debug("BaseActor received message", zap.Int64("actor_id", a.id), zap.Any("message", msg))
}

// IsRunning 检查Actor是否在运行
func (a *BaseActor) IsRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.isRunning
}

// run 内部运行循环
func (a *BaseActor) run() {
	a.logger.Info("Actor started", zap.Int64("actor_id", a.id))

	for msg := range a.msgChan {
		a.ProcessMessage(msg)
	}

	a.logger.Info("Actor stopped", zap.Int64("actor_id", a.id))
}

// ActorSystem Actor系统
type ActorSystem struct {
	actors    map[int64]Actor
	systemMsg chan ActorMessage
	isRunning bool
	mu        sync.Mutex
	logger    *zap.Logger
}

// 全局Actor系统实例
var globalActorSystem *ActorSystem
var globalActorSystemOnce sync.Once

// GetGlobalActorSystem 获取全局Actor系统实例
func GetGlobalActorSystem() *ActorSystem {
	globalActorSystemOnce.Do(func() {
		globalActorSystem = NewActorSystem()
		if err := globalActorSystem.Start(); err != nil {
			panic(fmt.Sprintf("Failed to start global actor system: %v", err))
		}
	})
	return globalActorSystem
}

// NewActorSystem 创建Actor系统实例
func NewActorSystem() *ActorSystem {
	return &ActorSystem{
		actors:    make(map[int64]Actor),
		systemMsg: make(chan ActorMessage, 100),
		isRunning: false,
		logger:    zLog.GetLogger(),
	}
}

// Start 启动Actor系统
func (as *ActorSystem) Start() error {
	as.mu.Lock()
	defer as.mu.Unlock()

	if as.isRunning {
		return fmt.Errorf("actor system is already running")
	}

	as.isRunning = true
	go as.run()
	return nil
}

// Stop 停止Actor系统
func (as *ActorSystem) Stop() error {
	as.mu.Lock()
	defer as.mu.Unlock()

	if !as.isRunning {
		return fmt.Errorf("actor system is not running")
	}

	// 停止所有Actor
	for _, actor := range as.actors {
		if err := actor.Stop(); err != nil {
			as.logger.Error("Failed to stop actor", zap.Int64("actor_id", actor.ID()), zap.Error(err))
		}
	}

	// 清空Actor映射
	as.actors = make(map[int64]Actor)

	// 停止系统消息处理
	as.isRunning = false
	close(as.systemMsg)

	return nil
}

// RegisterActor 注册Actor到系统
func (as *ActorSystem) RegisterActor(actor Actor) error {
	as.mu.Lock()
	defer as.mu.Unlock()

	if !as.isRunning {
		return fmt.Errorf("actor system is not running")
	}

	if _, exists := as.actors[actor.ID()]; exists {
		return fmt.Errorf("actor with id %d already exists", actor.ID())
	}

	// 启动Actor
	if err := actor.Start(); err != nil {
		return err
	}

	as.actors[actor.ID()] = actor
	return nil
}

// UnregisterActor 从系统中注销Actor
func (as *ActorSystem) UnregisterActor(actorID int64) error {
	as.mu.Lock()
	defer as.mu.Unlock()

	if !as.isRunning {
		return fmt.Errorf("actor system is not running")
	}

	actor, exists := as.actors[actorID]
	if !exists {
		return fmt.Errorf("actor with id %d not found", actorID)
	}

	// 停止并移除Actor
	if err := actor.Stop(); err != nil {
		return err
	}
	delete(as.actors, actorID)

	return nil
}

// GetActor 获取系统中的Actor
func (as *ActorSystem) GetActor(actorID int64) (Actor, bool) {
	as.mu.Lock()
	defer as.mu.Unlock()

	actor, exists := as.actors[actorID]
	return actor, exists
}

// SendMessage 发送消息给指定Actor
func (as *ActorSystem) SendMessage(actorID int64, msg ActorMessage) error {
	as.mu.Lock()
	actor, exists := as.actors[actorID]
	as.mu.Unlock()

	if !exists {
		return fmt.Errorf("actor with id %d not found", actorID)
	}

	actor.SendMessage(msg)
	return nil
}

// BroadcastMessage 广播消息给所有Actor
func (as *ActorSystem) BroadcastMessage(msg ActorMessage) {
	as.mu.Lock()
	actors := make(map[int64]Actor, len(as.actors))
	for id, actor := range as.actors {
		actors[id] = actor
	}
	as.mu.Unlock()

	for _, actor := range actors {
		actor.SendMessage(msg)
	}
}

// run 内部运行循环
func (as *ActorSystem) run() {
	as.logger.Info("Actor system started")

	// 处理系统级消息（如果需要）
	for msg := range as.systemMsg {
		// 系统消息处理逻辑
		as.logger.Debug("Actor system received system message", zap.Any("message", msg))
	}

	as.logger.Info("Actor system stopped")
}

// GetActorCount 获取系统中Actor数量
func (as *ActorSystem) GetActorCount() int {
	as.mu.Lock()
	defer as.mu.Unlock()
	return len(as.actors)
}

// IsRunning 检查Actor系统是否在运行
func (as *ActorSystem) IsRunning() bool {
	as.mu.Lock()
	defer as.mu.Unlock()
	return as.isRunning
}
