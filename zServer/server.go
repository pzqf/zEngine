package zServer

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/pzqf/zEngine/zLog"
	"github.com/pzqf/zEngine/zObject"
	"github.com/pzqf/zUtil/zMap"
)

// Logger 日志接口（与 zNet 保持一致）
type Logger = zLog.Logger

// ServerType 服务器类型
// 使用 string 类型，允许用户自定义服务器类型
type ServerType string

// BaseServer 基础服务器结构
type BaseServer struct {
	zObject.BaseObject
	ServerType     ServerType
	ServerName     string
	ServerVersion  string
	isRunning      atomic.Bool
	state          atomic.Value // ServerState
	components     *zMap.TypedMap[string, interface{}]
	stateListeners *zMap.TypedMap[uintptr, StateChangeListener]
	startTime      time.Time
	ctx            context.Context
	cancel         context.CancelFunc

	// 生命周期钩子接口
	hooks LifecycleHooks

	// 日志记录器（依赖注入）
	logger Logger
}

// NewBaseServer 创建基础服务器实例
// 参数:
//   - serverType: 服务器类型
//   - serverId: 服务器ID
//   - serverName: 服务器名称
//   - version: 服务器版本
//   - hooks: 生命周期钩子实现
//
// 返回:
//   - *BaseServer: 基础服务器实例
func NewBaseServer(serverType ServerType, serverId, serverName, version string, hooks LifecycleHooks) *BaseServer {
	if hooks == nil {
		panic("LifecycleHooks cannot be nil: must implement OnBeforeStart, OnAfterStart, OnBeforeStop")
	}

	ctx, cancel := context.WithCancel(context.Background())
	bs := &BaseServer{
		BaseObject:     zObject.BaseObject{Id: serverId},
		ServerType:     serverType,
		ServerName:     serverName,
		ServerVersion:  version,
		components:     zMap.NewTypedMap[string, interface{}](),
		stateListeners: zMap.NewTypedMap[uintptr, StateChangeListener](),
		startTime:      time.Now(),
		ctx:            ctx,
		cancel:         cancel,
		hooks:          hooks,
	}
	// 初始化状态为启动中
	bs.state.Store(StateStarting)
	return bs
}

// SetLogger 设置日志记录器（依赖注入）
func (s *BaseServer) SetLogger(logger Logger) {
	s.logger = logger
}

// GetLogger 获取日志记录器
func (s *BaseServer) GetLogger() Logger {
	return s.logger
}

// IsServerRunning 检查服务器是否运行
func (s *BaseServer) IsServerRunning() bool {
	return s.isRunning.Load()
}

// GetContext 获取服务器上下文
func (s *BaseServer) GetContext() context.Context {
	return s.ctx
}

// GetServerType 获取服务器类型
func (s *BaseServer) GetServerType() ServerType {
	return s.ServerType
}

// GetServerName 获取服务器名称
func (s *BaseServer) GetServerName() string {
	return s.ServerName
}

// GetServerVersion 获取服务器版本
func (s *BaseServer) GetServerVersion() string {
	return s.ServerVersion
}
