package zInject

import (
	"errors"
	"sync"
)

// Container 依赖注入容器接口
type Container interface {
	// Register 注册依赖
	Register(name string, factory interface{})
	// RegisterSingleton 注册单例依赖
	RegisterSingleton(name string, instance interface{})
	// Resolve 解析依赖
	Resolve(name string) (interface{}, error)
	// ResolveWithArgs 带参数解析依赖
	ResolveWithArgs(name string, args ...interface{}) (interface{}, error)
	// Has 检查依赖是否存在
	Has(name string) bool
	// Remove 移除依赖
	Remove(name string)
	// Clear 清空所有依赖
	Clear()
}

// DependencyType 依赖类型
type DependencyType int

const (
	// DependencyTypeFactory 工厂类型依赖
	DependencyTypeFactory DependencyType = iota
	// DependencyTypeSingleton 单例类型依赖
	DependencyTypeSingleton
)

// DependencyInfo 依赖信息
type DependencyInfo struct {
	Type     DependencyType
	Instance interface{} // 单例实例
	Factory  interface{} // 工厂函数
}

// SimpleContainer 简单依赖注入容器实现
type SimpleContainer struct {
	dependencies map[string]*DependencyInfo
	mu           sync.RWMutex
}

// NewContainer 创建一个新的依赖注入容器
func NewContainer() Container {
	return &SimpleContainer{
		dependencies: make(map[string]*DependencyInfo),
	}
}

// Register 注册依赖
func (c *SimpleContainer) Register(name string, factory interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.dependencies[name] = &DependencyInfo{
		Type:    DependencyTypeFactory,
		Factory: factory,
	}
}

// RegisterSingleton 注册单例依赖
func (c *SimpleContainer) RegisterSingleton(name string, instance interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.dependencies[name] = &DependencyInfo{
		Type:     DependencyTypeSingleton,
		Instance: instance,
	}
}

// Resolve 解析依赖
func (c *SimpleContainer) Resolve(name string) (interface{}, error) {
	return c.ResolveWithArgs(name)
}

// ResolveWithArgs 带参数解析依赖
func (c *SimpleContainer) ResolveWithArgs(name string, args ...interface{}) (interface{}, error) {
	c.mu.RLock()
	depInfo, ok := c.dependencies[name]
	c.mu.RUnlock()

	if !ok {
		return nil, errors.New("dependency not found: " + name)
	}

	switch depInfo.Type {
	case DependencyTypeSingleton:
		return depInfo.Instance, nil
	case DependencyTypeFactory:
		// 尝试调用不同类型的工厂函数
		switch f := depInfo.Factory.(type) {
		case func() interface{}:
			return f(), nil
		case func(...interface{}) interface{}:
			return f(args...), nil
		default:
			return nil, errors.New("invalid factory type for dependency: " + name)
		}
	default:
		return nil, errors.New("unknown dependency type for: " + name)
	}
}

// Has 检查依赖是否存在
func (c *SimpleContainer) Has(name string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	_, ok := c.dependencies[name]
	return ok
}

// Remove 移除依赖
func (c *SimpleContainer) Remove(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.dependencies, name)
}

// Clear 清空所有依赖
func (c *SimpleContainer) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.dependencies = make(map[string]*DependencyInfo)
}

