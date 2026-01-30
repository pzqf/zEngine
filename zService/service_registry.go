package zService

import (
	"reflect"
	"sync"

	"github.com/pzqf/zEngine/zLog"
	"go.uber.org/zap"
)

// ServiceRegistry 服务注册器接口
type ServiceRegistry interface {
	// RegisterService 注册服务
	RegisterService(factory func() Service) error
	// RegisterServiceWithDeps 注册带依赖的服务
	RegisterServiceWithDeps(factory func() Service, deps ...interface{}) error
	// GetAllServices 获取所有已注册的服务工厂
	GetAllServices() map[string]ServiceFactory
	// GetServiceInfos 获取所有服务注册信息
	GetServiceInfos() map[string]*ServiceRegistryInfo
	// Clear 清空所有注册的服务
	Clear()
}

// ServiceFactory 服务工厂函数类型
type ServiceFactory func() Service

// ServiceRegistryInfo 服务注册信息
type ServiceRegistryInfo struct {
	Factory ServiceFactory
	Deps    []interface{}
}

// SimpleServiceRegistry 简单服务注册器实现
type SimpleServiceRegistry struct {
	services map[string]*ServiceRegistryInfo
	mu       sync.RWMutex
}

// NewServiceRegistry 创建一个新的服务注册器
func NewServiceRegistry() ServiceRegistry {
	return &SimpleServiceRegistry{
		services: make(map[string]*ServiceRegistryInfo),
	}
}

// RegisterService 注册服务
func (r *SimpleServiceRegistry) RegisterService(factory func() Service) error {
	return r.RegisterServiceWithDeps(factory)
}

// RegisterServiceWithDeps 注册带依赖的服务
func (r *SimpleServiceRegistry) RegisterServiceWithDeps(factory func() Service, deps ...interface{}) error {
	// 创建服务实例以获取服务ID
	service := factory()
	if service == nil {
		return nil
	}

	serviceId := service.GetId()
	if serviceId == nil {
		return nil
	}

	serviceName := reflect.TypeOf(service).String()

	r.mu.Lock()
	defer r.mu.Unlock()

	r.services[serviceName] = &ServiceRegistryInfo{
		Factory: factory,
		Deps:    deps,
	}

	zLog.Info("Service registered", zap.String("service", serviceName), zap.Any("id", serviceId))
	return nil
}

// GetAllServices 获取所有已注册的服务工厂
func (r *SimpleServiceRegistry) GetAllServices() map[string]ServiceFactory {
	r.mu.RLock()
	defer r.mu.RUnlock()

	factories := make(map[string]ServiceFactory)
	for name, info := range r.services {
		factories[name] = info.Factory
	}

	return factories
}

// GetServiceInfos 获取所有服务注册信息
func (r *SimpleServiceRegistry) GetServiceInfos() map[string]*ServiceRegistryInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 创建一个副本以避免并发问题
	serviceInfos := make(map[string]*ServiceRegistryInfo)
	for name, info := range r.services {
		serviceInfos[name] = info
	}

	return serviceInfos
}

// Clear 清空所有注册的服务
func (r *SimpleServiceRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.services = make(map[string]*ServiceRegistryInfo)
}

// AutoRegisterServices 自动注册服务到服务管理器
func AutoRegisterServices(registry ServiceRegistry, manager *ServiceManager) error {
	// 获取所有服务注册信息
	serviceInfos := registry.GetServiceInfos()

	for name, info := range serviceInfos {
		service := info.Factory()
		if service == nil {
			zLog.Warn("Failed to create service", zap.String("service", name))
			continue
		}

		if err := manager.AddService(service, info.Deps...); err != nil {
			zLog.Error("Failed to add service", zap.String("service", name), zap.Error(err))
			continue
		}

		zLog.Info("Service auto-registered", zap.String("service", name), zap.Any("id", service.GetId()))
	}

	return nil
}
