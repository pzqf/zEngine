package zService

import (
	"errors"
	"sync"

	"github.com/pzqf/zEngine/zInject"
	"github.com/pzqf/zEngine/zLog"
	"github.com/pzqf/zEngine/zObject"
	"go.uber.org/zap"
)

// ServiceInfo 服务信息结构
// 存储服务实例及其依赖关系
type ServiceInfo struct {
	Service Service        // 服务实例
	Deps    []interface{}  // 依赖的服务ID列表
}

// ServiceManager 服务管理器
// 管理服务的生命周期、依赖关系和依赖注入
type ServiceManager struct {
	zObject.ObjectManager              // 继承对象管理器
	serviceInfos        map[interface{}]*ServiceInfo // 服务信息映射
	container           zInject.Container            // 依赖注入容器
	registry            ServiceRegistry              // 服务注册器
	mu                  sync.RWMutex                 // 读写锁
}

// NewServiceManager 创建一个新的服务管理器
// 返回:
//   - *ServiceManager: 服务管理器实例
func NewServiceManager() *ServiceManager {
	return &ServiceManager{
		ObjectManager: *zObject.NewObjectManager(),
		serviceInfos:  make(map[interface{}]*ServiceInfo),
		container:     zInject.NewContainer(),
		registry:      NewServiceRegistry(),
	}
}

// AddService 添加服务
// 参数:
//   - s: 服务实例
//   - deps: 依赖的服务ID列表
//
// 返回:
//   - error: 添加失败时返回错误
func (sm *ServiceManager) AddService(s Service, deps ...interface{}) error {
	if s.GetId() == nil {
		return errors.New("service must had id")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// 添加服务到对象管理器
	if err := sm.AddObject(s.GetId(), s); err != nil {
		return err
	}

	// 记录服务信息和依赖关系
	sm.serviceInfos[s.GetId()] = &ServiceInfo{
		Service: s,
		Deps:    deps,
	}

	return nil
}

// GetService 获取服务
// 参数:
//   - id: 服务ID
//
// 返回:
//   - Service: 服务实例
//   - error: 获取失败时返回错误
func (sm *ServiceManager) GetService(id interface{}) (Service, error) {
	object, err := sm.GetObject(id)
	if err != nil {
		return nil, err
	}

	return object.(Service), nil
}

// InitServices 初始化所有服务
// 按照拓扑排序顺序初始化服务，确保依赖服务先初始化
//
// 返回:
//   - error: 初始化失败时返回错误
func (sm *ServiceManager) InitServices() error {
	// 拓扑排序，确保依赖服务先初始化
	services, err := sm.topologicalSort()
	if err != nil {
		return err
	}

	// 按顺序初始化服务
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
// 按照拓扑排序顺序启动服务，每个服务在独立的goroutine中运行
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
			}(s)
		}
	}
}

// CloseServices 关闭所有服务
// 按照逆拓扑排序顺序关闭服务，确保依赖服务后关闭
//
// 返回:
//   - error: 关闭失败时返回错误
func (sm *ServiceManager) CloseServices() error {
	// 拓扑排序，确保依赖服务后关闭
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
// 根据服务依赖关系进行拓扑排序，检测循环依赖
//
// 返回:
//   - []Service: 排序后的服务列表
//   - error: 存在循环依赖时返回错误
func (sm *ServiceManager) topologicalSort() ([]Service, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	visited := make(map[interface{}]bool) // 已访问标记
	temp := make(map[interface{}]bool)    // 临时访问标记（用于检测循环）
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

			// 添加服务到结果列表
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

	// 遍历所有服务进行排序
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
// 参数:
//   - id: 服务ID
//
// 返回:
//   - ServiceState: 服务状态
//   - error: 获取失败时返回错误
func (sm *ServiceManager) GetServiceState(id interface{}) (ServiceState, error) {
	service, err := sm.GetService(id)
	if err != nil {
		return ServiceStateUnknown, err
	}

	return service.GetState(), nil
}

// ListServices 列出所有服务
// 返回:
//   - []Service: 所有服务列表
func (sm *ServiceManager) ListServices() []Service {
	var services []Service
	sm.ObjectsRange(func(key, value interface{}) bool {
		services = append(services, value.(Service))
		return true
	})

	return services
}

// RegisterDependency 注册依赖
// 参数:
//   - name: 依赖名称
//   - factory: 工厂函数
func (sm *ServiceManager) RegisterDependency(name string, factory interface{}) {
	sm.container.Register(name, factory)
}

// RegisterSingleton 注册单例依赖
// 参数:
//   - name: 依赖名称
//   - instance: 单例实例
func (sm *ServiceManager) RegisterSingleton(name string, instance interface{}) {
	sm.container.RegisterSingleton(name, instance)
}

// ResolveDependency 解析依赖
// 参数:
//   - name: 依赖名称
//
// 返回:
//   - interface{}: 依赖实例
//   - error: 解析失败时返回错误
func (sm *ServiceManager) ResolveDependency(name string) (interface{}, error) {
	return sm.container.Resolve(name)
}

// ResolveDependencyWithArgs 带参数解析依赖
// 参数:
//   - name: 依赖名称
//   - args: 传递给工厂函数的参数
//
// 返回:
//   - interface{}: 依赖实例
//   - error: 解析失败时返回错误
func (sm *ServiceManager) ResolveDependencyWithArgs(name string, args ...interface{}) (interface{}, error) {
	return sm.container.ResolveWithArgs(name, args...)
}

// HasDependency 检查依赖是否存在
// 参数:
//   - name: 依赖名称
//
// 返回:
//   - bool: 依赖存在时返回true
func (sm *ServiceManager) HasDependency(name string) bool {
	return sm.container.Has(name)
}

// GetContainer 获取依赖注入容器
// 返回:
//   - zInject.Container: 依赖注入容器实例
func (sm *ServiceManager) GetContainer() zInject.Container {
	return sm.container
}

// RegisterService 注册服务到服务注册器
// 参数:
//   - factory: 服务工厂
//   - deps: 依赖列表
//
// 返回:
//   - error: 注册失败时返回错误
func (sm *ServiceManager) RegisterService(factory ServiceFactory, deps ...interface{}) error {
	return sm.registry.RegisterServiceWithDeps(factory, deps...)
}

// AutoRegisterServices 自动注册所有已注册的服务
// 返回:
//   - error: 注册失败时返回错误
func (sm *ServiceManager) AutoRegisterServices() error {
	return AutoRegisterServices(sm.registry, sm)
}

// GetRegistry 获取服务注册器
// 返回:
//   - ServiceRegistry: 服务注册器实例
func (sm *ServiceManager) GetRegistry() ServiceRegistry {
	return sm.registry
}
