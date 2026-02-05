package zInject

import (
	"errors"
	"sync"
)

// Container 依赖注入容器接口
// 提供依赖注册、解析和管理的统一接口
type Container interface {
	// Register 注册工厂函数依赖
	// 每次解析时都会调用工厂函数创建新实例
	Register(name string, factory interface{})
	// RegisterSingleton 注册单例依赖
	// 多次解析返回同一个实例
	RegisterSingleton(name string, instance interface{})
	// Resolve 解析依赖
	// 根据名称获取已注册的依赖实例
	Resolve(name string) (interface{}, error)
	// ResolveWithArgs 带参数解析依赖
	// 支持向工厂函数传递参数
	ResolveWithArgs(name string, args ...interface{}) (interface{}, error)
	// Has 检查依赖是否存在
	Has(name string) bool
	// Remove 移除依赖
	Remove(name string)
	// Clear 清空所有依赖
	Clear()
}

// DependencyType 依赖类型枚举
// 用于区分单例依赖和工厂依赖
type DependencyType int

const (
	// DependencyTypeFactory 工厂类型依赖
	// 每次解析时调用工厂函数创建新实例
	DependencyTypeFactory DependencyType = iota
	// DependencyTypeSingleton 单例类型依赖
	// 多次解析返回同一个预注册的实例
	DependencyTypeSingleton
)

// DependencyInfo 依赖信息结构
// 存储依赖的类型、实例或工厂函数
type DependencyInfo struct {
	Type     DependencyType // 依赖类型
	Instance interface{}    // 单例实例（仅用于单例类型）
	Factory  interface{}    // 工厂函数（仅用于工厂类型）
}

// SimpleContainer 简单依赖注入容器实现
// 提供线程安全的依赖注册和解析功能
type SimpleContainer struct {
	dependencies map[string]*DependencyInfo // 依赖映射表
	mu           sync.RWMutex               // 读写锁，保证线程安全
}

// NewContainer 创建一个新的依赖注入容器
// 返回实现了Container接口的SimpleContainer实例
//
// 返回:
//   - Container: 依赖注入容器实例
func NewContainer() Container {
	return &SimpleContainer{
		dependencies: make(map[string]*DependencyInfo),
	}
}

// Register 注册工厂函数依赖
// 参数:
//   - name: 依赖名称
//   - factory: 工厂函数，支持 func() interface{} 或 func(...interface{}) interface{}
func (c *SimpleContainer) Register(name string, factory interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.dependencies[name] = &DependencyInfo{
		Type:    DependencyTypeFactory,
		Factory: factory,
	}
}

// RegisterSingleton 注册单例依赖
// 参数:
//   - name: 依赖名称
//   - instance: 单例实例
func (c *SimpleContainer) RegisterSingleton(name string, instance interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.dependencies[name] = &DependencyInfo{
		Type:     DependencyTypeSingleton,
		Instance: instance,
	}
}

// Resolve 解析依赖
// 参数:
//   - name: 依赖名称
//
// 返回:
//   - interface{}: 依赖实例
//   - error: 依赖不存在或解析失败时返回错误
func (c *SimpleContainer) Resolve(name string) (interface{}, error) {
	return c.ResolveWithArgs(name)
}

// ResolveWithArgs 带参数解析依赖
// 参数:
//   - name: 依赖名称
//   - args: 传递给工厂函数的参数
//
// 返回:
//   - interface{}: 依赖实例
//   - error: 依赖不存在、工厂函数类型不匹配或解析失败时返回错误
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
		// 根据工厂函数类型调用相应的工厂函数
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
// 参数:
//   - name: 依赖名称
//
// 返回:
//   - bool: 依赖存在时返回true
func (c *SimpleContainer) Has(name string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	_, ok := c.dependencies[name]
	return ok
}

// Remove 移除依赖
// 参数:
//   - name: 依赖名称
func (c *SimpleContainer) Remove(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.dependencies, name)
}

// Clear 清空所有依赖
// 删除容器中所有已注册的依赖
func (c *SimpleContainer) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.dependencies = make(map[string]*DependencyInfo)
}
