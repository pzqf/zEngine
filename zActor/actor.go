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
	id           int64
	ActorMsgChan chan ActorMessage
	isRunning    bool
	mu           sync.Mutex
	logger       *zap.Logger
}

// NewBaseActor 创建基础Actor实例
func NewBaseActor(id int64, chanSize int) *BaseActor {
	if chanSize <= 0 {
		chanSize = 1024
	}
	return &BaseActor{
		id:           id,
		ActorMsgChan: make(chan ActorMessage, chanSize),
		isRunning:    false,
		logger:       zLog.GetLogger(),
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
	close(a.ActorMsgChan)
	return nil
}

// SendMessage 发送消息给Actor
func (a *BaseActor) SendMessage(msg ActorMessage) {
	if !a.IsRunning() {
		a.logger.Warn("Sending message to stopped actor", zap.Int64("actor_id", a.id))
		return
	}

	select {
	case a.ActorMsgChan <- msg:
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

	for msg := range a.ActorMsgChan {
		a.ProcessMessage(msg)
	}

	a.logger.Info("Actor stopped", zap.Int64("actor_id", a.id))
}
