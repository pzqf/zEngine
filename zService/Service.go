package zService

import (
	"sync"

	"github.com/pzqf/zEngine/zObject"
)

// ServiceState 服务状态
type ServiceState int

const (
	ServiceStateUnknown  ServiceState = 0
	ServiceStateCreated  ServiceState = 1
	ServiceStateInit     ServiceState = 2
	ServiceStateRunning  ServiceState = 3
	ServiceStateStopping ServiceState = 4
	ServiceStateStopped  ServiceState = 5
)

// Service 服务接口
type Service interface {
	zObject.ManagedObject
	GetState() ServiceState
	SetState(state ServiceState)
}

// BaseService 基础服务实现
type BaseService struct {
	zObject.BaseObject
	state ServiceState
	mu    sync.RWMutex
}

// NewBaseService 创建基础服务实例
func NewBaseService(id interface{}) *BaseService {
	return &BaseService{
		BaseObject: zObject.BaseObject{Id: id},
		state:      ServiceStateCreated,
	}
}

// GetState 获取服务状态
func (bs *BaseService) GetState() ServiceState {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	return bs.state
}

// SetState 设置服务状态
func (bs *BaseService) SetState(state ServiceState) {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	bs.state = state
}

// Init 初始化服务
func (bs *BaseService) Init() error {
	return nil
}

// Close 关闭服务
func (bs *BaseService) Close() error {
	return nil
}

// Serve 运行服务
func (bs *BaseService) Serve() {
	// 空实现，子类可以重写
}

