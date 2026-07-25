package zServer

import (
	"fmt"
	"time"
)

// ServerState 服务器生命周期状态
type ServerState string

const (
	StateStarting     ServerState = "starting"     // 服务器正在启动
	StateInitializing ServerState = "initializing" // 服务器正在初始化
	StateReady        ServerState = "ready"        // 服务器已就绪
	StateHealthy      ServerState = "healthy"      // 服务器健康运行
	StateDraining     ServerState = "draining"     // 服务器正在排空流量
	StateMaintenance  ServerState = "maintenance"  // 服务器维护中
	StateStopped      ServerState = "stopped"      // 服务器已停止
)

// String 返回状态的字符串表示
func (s ServerState) String() string {
	return string(s)
}

// IsActive 判断状态是否为活跃状态
func (s ServerState) IsActive() bool {
	switch s {
	case StateReady, StateHealthy:
		return true
	default:
		return false
	}
}

// IsTerminal 判断状态是否为终止状态
func (s ServerState) IsTerminal() bool {
	return s == StateStopped
}

// CanTransitionTo 判断是否可以从当前状态转换到目标状态
func (s ServerState) CanTransitionTo(targetState ServerState) bool {
	switch s {
	case StateStarting:
		return targetState == StateInitializing || targetState == StateStopped
	case StateInitializing:
		return targetState == StateReady || targetState == StateMaintenance || targetState == StateStopped
	case StateReady:
		return targetState == StateHealthy || targetState == StateMaintenance || targetState == StateDraining || targetState == StateStopped
	case StateHealthy:
		return targetState == StateMaintenance || targetState == StateDraining || targetState == StateStopped
	case StateDraining:
		return targetState == StateStopped
	case StateMaintenance:
		return targetState == StateReady || targetState == StateHealthy || targetState == StateStopped
	case StateStopped:
		return targetState == StateStarting // 允许从停止状态重新启动
	default:
		return false
	}
}

// StateChangeEvent 状态变化事件
type StateChangeEvent struct {
	OldState  ServerState
	NewState  ServerState
	Timestamp time.Time
	Reason    string
}

// StateChangeListener 状态变化监听器
type StateChangeListener func(event StateChangeEvent)

// GetState 获取当前状态
func (s *BaseServer) GetState() ServerState {
	if state, ok := s.state.Load().(ServerState); ok {
		return state
	}
	return StateStarting
}

// SetState 设置状态
func (s *BaseServer) SetState(state ServerState, reason string) error {
	oldState := s.GetState()
	if oldState == state {
		// 状态未改变，直接返回
		return nil
	}
	if !oldState.CanTransitionTo(state) {
		return fmt.Errorf("invalid state transition from %s to %s", oldState, state)
	}

	s.state.Store(state)

	// 触发状态变化事件
	event := StateChangeEvent{
		OldState:  oldState,
		NewState:  state,
		Timestamp: time.Now(),
		Reason:    reason,
	}

	// 通知所有监听器
	s.stateListeners.Range(func(_ uint64, listener StateChangeListener) bool {
		go listener(event) // 异步通知，避免阻塞
		return true
	})

	if s.logger != nil {
		s.logger.Info("Server state changed, old_state: %s, new_state: %s, reason:%s", oldState, state, reason)
	}
	return nil
}

// AddStateChangeListener 添加状态变化监听器，返回用于移除的句柄（NET-7）。
// 此前用 &listener（参数栈地址）作 key：Add 与 Remove 的地址不同→永远删不掉，且栈地址
// 复用可能相互覆盖。改为自增句柄，稳定且可精确移除。
func (s *BaseServer) AddStateChangeListener(listener StateChangeListener) uint64 {
	if listener == nil {
		return 0
	}
	handle := s.listenerSeq.Add(1)
	s.stateListeners.Store(handle, listener)
	return handle
}

// RemoveStateChangeListener 按 AddStateChangeListener 返回的句柄移除监听器。
func (s *BaseServer) RemoveStateChangeListener(handle uint64) {
	if handle != 0 {
		s.stateListeners.Delete(handle)
	}
}

// IsStateActive 判断当前状态是否为活跃状态
func (s *BaseServer) IsStateActive() bool {
	return s.GetState().IsActive()
}

// IsStateTerminal 判断当前状态是否为终止状态
func (s *BaseServer) IsStateTerminal() bool {
	return s.GetState().IsTerminal()
}

// CanTransitionTo 判断是否可以转换到目标状态
func (s *BaseServer) CanTransitionTo(targetState ServerState) bool {
	return s.GetState().CanTransitionTo(targetState)
}

// Initialize 初始化服务器
func (s *BaseServer) Initialize() error {
	return s.SetState(StateInitializing, "server initializing")
}

// Ready 标记服务器就绪
func (s *BaseServer) Ready() error {
	return s.SetState(StateReady, "server ready")
}

// Healthy 标记服务器健康
func (s *BaseServer) Healthy() error {
	return s.SetState(StateHealthy, "server healthy")
}

// Drain 标记服务器进入流量排空状态
func (s *BaseServer) Drain() error {
	return s.SetState(StateDraining, "server draining")
}

// EnterMaintenance 进入维护模式
func (s *BaseServer) EnterMaintenance() error {
	return s.SetState(StateMaintenance, "server entering maintenance")
}

// ExitMaintenance 退出维护模式
func (s *BaseServer) ExitMaintenance() error {
	return s.SetState(StateReady, "server exiting maintenance")
}
