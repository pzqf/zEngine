package zSystem

import (
	"sync"
)

// ISystem 系统接口
type ISystem interface {
	// Initialize 初始化系统
	Initialize() error

	// Update 更新系统
	Update(deltaTime float64)

	// Shutdown 关闭系统
	Shutdown() error

	// GetSystemName 获取系统名称
	GetSystemName() string
}

// BaseSystem 基础系统实现
type BaseSystem struct {
	mu          sync.RWMutex
	name        string
	initialized bool
}

// NewBaseSystem 创建基础系统
func NewBaseSystem(name string) *BaseSystem {
	return &BaseSystem{
		name: name,
	}
}

// Initialize 初始化系统
func (bs *BaseSystem) Initialize() error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	bs.initialized = true
	return nil
}

// Update 更新系统
func (bs *BaseSystem) Update(deltaTime float64) {

}

// Shutdown 关闭系统
func (bs *BaseSystem) Shutdown() error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	bs.initialized = false
	return nil
}

// GetSystemName 获取系统名称
func (bs *BaseSystem) GetSystemName() string {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	return bs.name
}

// IsInitialized 检查系统是否已初始化
func (bs *BaseSystem) IsInitialized() bool {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	return bs.initialized
}
