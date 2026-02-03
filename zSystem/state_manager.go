package zSystem

import (
	"sync"
)

// IState 状态接口
type IState interface {
	GetOwnerID() uint64
}

// StateManager 状态管理器
type StateManager struct {
	mu     sync.RWMutex
	states map[uint64]map[string]interface{}
}

// NewStateManager 创建状态管理器
func NewStateManager() *StateManager {
	return &StateManager{
		states: make(map[uint64]map[string]interface{}),
	}
}

// SetState 设置状态
func (sm *StateManager) SetState(ownerID uint64, key string, value interface{}) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.states[ownerID]; !exists {
		sm.states[ownerID] = make(map[string]interface{})
	}

	sm.states[ownerID][key] = value
}

// GetState 获取状态
func (sm *StateManager) GetState(ownerID uint64, key string) (interface{}, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if ownerStates, exists := sm.states[ownerID]; exists {
		if value, ok := ownerStates[key]; ok {
			return value, true
		}
	}
	return nil, false
}

// GetStateWithType 获取指定类型的状态
func (sm *StateManager) GetStateWithType(ownerID uint64, key string) (interface{}, bool) {
	return sm.GetState(ownerID, key)
}

// RemoveState 移除状态
func (sm *StateManager) RemoveState(ownerID uint64, key string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if ownerStates, exists := sm.states[ownerID]; exists {
		delete(ownerStates, key)

		if len(ownerStates) == 0 {
			delete(sm.states, ownerID)
		}
	}
}

// RemoveAllStates 移除所有者的所有状态
func (sm *StateManager) RemoveAllStates(ownerID uint64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	delete(sm.states, ownerID)
}

// GetAllStates 获取所有者的所有状态
func (sm *StateManager) GetAllStates(ownerID uint64) map[string]interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if ownerStates, exists := sm.states[ownerID]; exists {
		result := make(map[string]interface{}, len(ownerStates))
		for k, v := range ownerStates {
			result[k] = v
		}
		return result
	}
	return make(map[string]interface{})
}

// HasState 检查是否存在指定状态
func (sm *StateManager) HasState(ownerID uint64, key string) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if ownerStates, exists := sm.states[ownerID]; exists {
		_, ok := ownerStates[key]
		return ok
	}
	return false
}

// GetOwnerCount 获取状态所有者数量
func (sm *StateManager) GetOwnerCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return len(sm.states)
}

// Clear 清空所有状态
func (sm *StateManager) Clear() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.states = make(map[uint64]map[string]interface{})
}
