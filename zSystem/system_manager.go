package zSystem

import (
	"sync"
)

// SystemManager 系统管理器
type SystemManager struct {
	mu      sync.RWMutex
	systems map[string]ISystem
}

// GlobalSystemManager 全局系统管理器实例
var GlobalSystemManager *SystemManager

// init 初始化全局系统管理器
func init() {
	GlobalSystemManager = NewSystemManager()
}

// NewSystemManager 创建系统管理器
func NewSystemManager() *SystemManager {
	return &SystemManager{
		systems: make(map[string]ISystem),
	}
}

// RegisterSystem 注册系统
func (sm *SystemManager) RegisterSystem(system ISystem) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	name := system.GetSystemName()
	if _, exists := sm.systems[name]; exists {
		return nil
	}

	sm.systems[name] = system
	return nil
}

// UnregisterSystem 注销系统
func (sm *SystemManager) UnregisterSystem(name string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	delete(sm.systems, name)
}

// GetSystem 获取系统
func (sm *SystemManager) GetSystem(name string) (ISystem, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	system, ok := sm.systems[name]
	return system, ok
}

// InitializeAll 初始化所有系统
func (sm *SystemManager) InitializeAll() error {
	sm.mu.RLock()
	systems := make([]ISystem, 0, len(sm.systems))
	for _, system := range sm.systems {
		systems = append(systems, system)
	}
	sm.mu.RUnlock()

	for _, system := range systems {
		if err := system.Initialize(); err != nil {
			return err
		}
	}

	return nil
}

// UpdateAll 更新所有系统
func (sm *SystemManager) UpdateAll(deltaTime float64) {
	sm.mu.RLock()
	systems := make([]ISystem, 0, len(sm.systems))
	for _, system := range sm.systems {
		systems = append(systems, system)
	}
	sm.mu.RUnlock()

	for _, system := range systems {
		system.Update(deltaTime)
	}
}

// ShutdownAll 关闭所有系统
func (sm *SystemManager) ShutdownAll() error {
	sm.mu.RLock()
	systems := make([]ISystem, 0, len(sm.systems))
	for _, system := range sm.systems {
		systems = append(systems, system)
	}
	sm.mu.RUnlock()

	for _, system := range systems {
		if err := system.Shutdown(); err != nil {
			return err
		}
	}

	return nil
}

// GetSystemCount 获取系统数量
func (sm *SystemManager) GetSystemCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return len(sm.systems)
}

// GetSystemNames 获取所有系统名称
func (sm *SystemManager) GetSystemNames() []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	names := make([]string, 0, len(sm.systems))
	for name := range sm.systems {
		names = append(names, name)
	}
	return names
}

// HasSystem 检查是否存在指定系统
func (sm *SystemManager) HasSystem(name string) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	_, ok := sm.systems[name]
	return ok
}

// Clear 清空所有系统
func (sm *SystemManager) Clear() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.systems = make(map[string]ISystem)
}
