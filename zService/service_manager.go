package zService

import (
	"errors"
	"sync"

	"github.com/pzqf/zEngine/zInject"
	"github.com/pzqf/zEngine/zLog"
	"github.com/pzqf/zEngine/zObject"
	"go.uber.org/zap"
)

// ServiceInfo 服务信息
type ServiceInfo struct {
	Service Service
	Deps    []interface{}
}

// ServiceManager 服务管理器
type ServiceManager struct {
	zObject.ObjectManager
	serviceInfos map[interface{}]*ServiceInfo
	container    zInject.Container
	registry     ServiceRegistry
	mu           sync.RWMutex
}

// NewServiceManager 创建一个新的服务管理器
func NewServiceManager() *ServiceManager {
	return &ServiceManager{
		ObjectManager: *zObject.NewObjectManager(),
		serviceInfos:  make(map[interface{}]*ServiceInfo),
		container:     zInject.NewContainer(),
		registry:      NewServiceRegistry(),
	}
}

// AddService 添加服务
func (sm *ServiceManager) AddService(s Service, deps ...interface{}) error {
	if s.GetId() == nil {
		return errors.New("service must had id")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// 添加服务
	if err := sm.AddObject(s.GetId(), s); err != nil {
		return err
	}

	// 记录服务信息和依赖
	sm.serviceInfos[s.GetId()] = &ServiceInfo{
		Service: s,
		Deps:    deps,
	}

	return nil
}

// GetService 获取服务
func (sm *ServiceManager) GetService(id interface{}) (Service, error) {
	object, err := sm.GetObject(id)
	if err != nil {
		return nil, err
	}

	return object.(Service), nil
}

// InitServices 初始化所有服务
func (sm *ServiceManager) InitServices() error {
	// 拓扑排序，确保依赖服务先初始化
	services, err := sm.topologicalSort()
	if err != nil {
		return err
	}

	// 初始化服务
	for _, s := range services {
		if s.GetState() == ServiceStateCreated {
			s.SetState(ServiceStateInit)
			if err := s.Init(); err != nil {
				s.SetState(ServiceStateCreated)
				zLog.Error("Failed to init service", zap.Any("serviceId", s.GetId()), zap.Error(err))
				return err
			}
			zLog.Info("Service initialized", zap.Any("serviceId", s.GetId()))
		}
	}

	return nil
}

// ServeServices 启动所有服务
func (sm *ServiceManager) ServeServices() {
	// 拓扑排序，确保依赖服务先启动
	services, err := sm.topologicalSort()
	if err != nil {
		zLog.Error("Failed to sort services", zap.Error(err))
		return
	}

	// 启动服务
	for _, s := range services {
		if s.GetState() == ServiceStateInit {
			s.SetState(ServiceStateRunning)
			go func(service Service) {
				defer func() {
					if r := recover(); r != nil {
						zLog.Error("Service panicked", zap.Any("serviceId", service.GetId()), zap.Any("panic", r))
						service.SetState(ServiceStateStopped)
					}
				}()
				zLog.Info("Starting service", zap.Any("serviceId", service.GetId()))
				service.Serve()
				// 移除自动设置停止状态的逻辑，让服务自己管理状态
				// 或者让CloseServices()方法来设置停止状态
			}(s)
		}
	}
}

// CloseServices 关闭所有服务
func (sm *ServiceManager) CloseServices() error {
	// 逆拓扑排序，确保依赖服务后关闭
	services, err := sm.topologicalSort()
	if err != nil {
		return err
	}

	// 逆序关闭服务
	for i := len(services) - 1; i >= 0; i-- {
		s := services[i]
		if s.GetState() == ServiceStateRunning {
			s.SetState(ServiceStateStopping)
			if err := s.Close(); err != nil {
				zLog.Error("Failed to close service", zap.Any("serviceId", s.GetId()), zap.Error(err))
				return err
			}
			s.SetState(ServiceStateStopped)
			zLog.Info("Service closed", zap.Any("serviceId", s.GetId()))
		}
	}

	return nil
}

// topologicalSort 拓扑排序服务
func (sm *ServiceManager) topologicalSort() ([]Service, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	visited := make(map[interface{}]bool)
	temp := make(map[interface{}]bool)
	result := make([]Service, 0)

	// 深度优先搜索
	var dfs func(interface{}) error
	dfs = func(id interface{}) error {
		if temp[id] {
			return errors.New("circular dependency detected")
		}

		if visited[id] {
			return nil
		}

		temp[id] = true

		// 先处理依赖
		if info, ok := sm.serviceInfos[id]; ok {
			for _, depId := range info.Deps {
				if err := dfs(depId); err != nil {
					return err
				}
			}

			// 添加服务
			service, err := sm.GetService(id)
			if err != nil {
				return err
			}
			result = append(result, service)
		}

		temp[id] = false
		visited[id] = true
		return nil
	}

	// 遍历所有服务
	for id := range sm.serviceInfos {
		if !visited[id] {
			if err := dfs(id); err != nil {
				return nil, err
			}
		}
	}

	return result, nil
}

// GetServiceState 获取服务状态
func (sm *ServiceManager) GetServiceState(id interface{}) (ServiceState, error) {
	service, err := sm.GetService(id)
	if err != nil {
		return ServiceStateUnknown, err
	}

	return service.GetState(), nil
}

// ListServices 列出所有服务
func (sm *ServiceManager) ListServices() []Service {
	var services []Service
	sm.ObjectsRange(func(key, value interface{}) bool {
		services = append(services, value.(Service))
		return true
	})

	return services
}

// RegisterDependency 注册依赖
func (sm *ServiceManager) RegisterDependency(name string, factory interface{}) {
	sm.container.Register(name, factory)
}

// RegisterSingleton 注册单例依赖
func (sm *ServiceManager) RegisterSingleton(name string, instance interface{}) {
	sm.container.RegisterSingleton(name, instance)
}

// ResolveDependency 解析依赖
func (sm *ServiceManager) ResolveDependency(name string) (interface{}, error) {
	return sm.container.Resolve(name)
}

// ResolveDependencyWithArgs 带参数解析依赖
func (sm *ServiceManager) ResolveDependencyWithArgs(name string, args ...interface{}) (interface{}, error) {
	return sm.container.ResolveWithArgs(name, args...)
}

// HasDependency 检查依赖是否存在
func (sm *ServiceManager) HasDependency(name string) bool {
	return sm.container.Has(name)
}

// GetContainer 获取依赖注入容器
func (sm *ServiceManager) GetContainer() zInject.Container {
	return sm.container
}

// RegisterService 注册服务到服务注册器
func (sm *ServiceManager) RegisterService(factory ServiceFactory, deps ...interface{}) error {
	return sm.registry.RegisterServiceWithDeps(factory, deps...)
}

// AutoRegisterServices 自动注册所有已注册的服务
func (sm *ServiceManager) AutoRegisterServices() error {
	return AutoRegisterServices(sm.registry, sm)
}

// GetRegistry 获取服务注册器
func (sm *ServiceManager) GetRegistry() ServiceRegistry {
	return sm.registry
}
