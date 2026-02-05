package zService

import (
	"sync"

	"github.com/pzqf/zEngine/zObject"
)

// ServiceState 服务状态枚举
// 用于标识服务在生命周期中的不同阶段
type ServiceState int

const (
	ServiceStateUnknown  ServiceState = 0 // 未知状态
	ServiceStateCreated  ServiceState = 1 // 已创建
	ServiceStateInit     ServiceState = 2 // 已初始化
	ServiceStateRunning  ServiceState = 3 // 运行中
	ServiceStateStopping ServiceState = 4 // 停止中
	ServiceStateStopped  ServiceState = 5 // 已停止
)

// Service 服务接口
// 定义了服务的基本行为和生命周期方法
type Service interface {
	zObject.ManagedObject        // 继承托管对象接口
	GetState() ServiceState      // 获取服务状态
	SetState(state ServiceState) // 设置服务状态
}

// BaseService 基础服务实现
// 提供Service接口的默认实现，可嵌入到具体服务中
type BaseService struct {
	zObject.BaseObject     // 继承基础对象
	state            ServiceState // 服务状态
	mu               sync.RWMutex // 状态读写锁
}

// NewBaseService 创建基础服务实例
// 参数:
//   - serviceId: 服务唯一标识符
//
// 返回:
//   - *BaseService: 基础服务实例
func NewBaseService(serviceId string) *BaseService {
	return &BaseService{
		BaseObject: zObject.BaseObject{Id: serviceId},
		state:      ServiceStateCreated,
	}
}

// ServiceId 获取服务ID
// 返回:
//   - string: 服务ID字符串
func (bs *BaseService) ServiceId() string {
	if id := bs.GetId(); id != nil {
		return id.(string)
	}
	return ""
}

// GetState 获取服务状态
// 返回:
//   - ServiceState: 当前服务状态
func (bs *BaseService) GetState() ServiceState {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	return bs.state
}

// SetState 设置服务状态
// 参数:
//   - state: 新的服务状态
func (bs *BaseService) SetState(state ServiceState) {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	bs.state = state
}

// Init 初始化服务
// 返回:
//   - error: 初始化失败时返回错误
func (bs *BaseService) Init() error {
	return nil
}

// Close 关闭服务
// 返回:
//   - error: 关闭失败时返回错误
func (bs *BaseService) Close() error {
	return nil
}

// Serve 运行服务
// 空实现，子类可以重写以实现具体服务逻辑
func (bs *BaseService) Serve() {
	// 空实现，子类可以重写
}
