package zInject

import (
	"testing"
)

// TestContainer_Basic 测试容器的基本功能
func TestContainer_Basic(t *testing.T) {
	container := NewContainer()

	// 测试注册和解析单例
	container.RegisterSingleton("testSingleton", "test value")
	value, err := container.Resolve("testSingleton")
	if err != nil {
		t.Errorf("Failed to resolve singleton: %v", err)
	}
	if value != "test value" {
		t.Errorf("Expected 'test value', got %v", value)
	}

	// 测试注册和解析工厂
	container.Register("testFactory", func() interface{} {
		return "factory value"
	})
	value, err = container.Resolve("testFactory")
	if err != nil {
		t.Errorf("Failed to resolve factory: %v", err)
	}
	if value != "factory value" {
		t.Errorf("Expected 'factory value', got %v", value)
	}

	// 测试带参数的解析
	container.Register("testFactoryWithArgs", func(args ...interface{}) interface{} {
		if len(args) > 0 {
			return args[0]
		}
		return "default"
	})
	value, err = container.ResolveWithArgs("testFactoryWithArgs", "arg value")
	if err != nil {
		t.Errorf("Failed to resolve factory with args: %v", err)
	}
	if value != "arg value" {
		t.Errorf("Expected 'arg value', got %v", value)
	}

	// 测试依赖是否存在
	if !container.Has("testSingleton") {
		t.Error("Expected testSingleton to exist")
	}
	if container.Has("nonExistent") {
		t.Error("Expected nonExistent to not exist")
	}

	// 测试移除依赖
	container.Remove("testSingleton")
	if container.Has("testSingleton") {
		t.Error("Expected testSingleton to be removed")
	}

	// 测试清空容器
	container.Clear()
	if container.Has("testFactory") {
		t.Error("Expected testFactory to be cleared")
	}
}

// TestContainer_ErrorHandling 测试容器的错误处理
func TestContainer_ErrorHandling(t *testing.T) {
	container := NewContainer()

	// 测试解析不存在的依赖
	_, err := container.Resolve("nonExistent")
	if err == nil {
		t.Error("Expected error when resolving non-existent dependency")
	}

	// 测试无效的工厂类型
	container.Register("invalidFactory", "not a function")
	_, err = container.Resolve("invalidFactory")
	if err == nil {
		t.Error("Expected error when resolving invalid factory")
	}
}
