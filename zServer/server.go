package zServer

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pzqf/zEngine/zLog"
	"github.com/pzqf/zEngine/zObject"
	"github.com/pzqf/zUtil/zMap"
)

type Logger = zLog.Logger

type ServerType string

type BaseServer struct {
	zObject.BaseObject
	ServerType     ServerType
	ServerName     string
	ServerVersion  string
	isRunning      atomic.Bool
	state          atomic.Value
	components     *zMap.TypedMap[string, interface{}]
	stateListeners *zMap.TypedMap[uint64, StateChangeListener]
	listenerSeq    atomic.Uint64 // NET-7: 监听器句柄自增源，替代不可靠的栈地址作 key
	startTime      time.Time
	ctx            context.Context
	cancel         context.CancelFunc
	hooks          LifecycleHooks
	logger         Logger
	healthMu       sync.RWMutex
	healthProvider HealthProvider
	lifecycleMu    sync.Mutex
	startClaimed   bool
	startComplete  bool
	startSucceeded bool
	stopping       atomic.Bool
	startDone      chan struct{}
	stopDone       chan struct{}
	stopOnce       sync.Once
	stopErr        error
	cleanupMu      sync.Mutex
	cleanups       []cleanupEntry
	cleanupNames   map[string]struct{}
	cleanupClosed  bool
	stateMu        sync.Mutex
	signalContext  func() (context.Context, context.CancelFunc)
}

// HealthProvider 由应用装配，zServer 不决定具体依赖是否阻断流量。
type HealthProvider interface {
	HealthStatus() (live, ready, healthy bool)
}

func (s *BaseServer) SetHealthProvider(provider HealthProvider) {
	s.healthMu.Lock()
	s.healthProvider = provider
	s.healthMu.Unlock()
}

func (s *BaseServer) getHealthProvider() HealthProvider {
	s.healthMu.RLock()
	defer s.healthMu.RUnlock()
	return s.healthProvider
}

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
		stateListeners: zMap.NewTypedMap[uint64, StateChangeListener](),
		startTime:      time.Now(),
		ctx:            ctx,
		cancel:         cancel,
		hooks:          hooks,
		startDone:      make(chan struct{}),
		stopDone:       make(chan struct{}),
		cleanupNames:   make(map[string]struct{}),
		signalContext:  defaultSignalContext,
	}
	bs.state.Store(StateStarting)
	return bs
}

func (s *BaseServer) SetLogger(logger Logger) {
	s.logger = logger
}

func (s *BaseServer) GetLogger() Logger {
	return s.logger
}

func (s *BaseServer) IsServerRunning() bool {
	return s.isRunning.Load()
}

func (s *BaseServer) GetContext() context.Context {
	return s.ctx
}

func (s *BaseServer) GetServerType() ServerType {
	return s.ServerType
}

func (s *BaseServer) GetServerName() string {
	return s.ServerName
}

func (s *BaseServer) GetServerVersion() string {
	return s.ServerVersion
}

// 说明（成熟化改造 Phase 1.3）：BaseServer 曾内嵌 zService.ServiceManager 并暴露
// AddService/InitServices/ServeServices/CloseServices/RegisterDependency/Singleton/
// ResolveDependency 一整套服务编排 + DI 门面，但全项目零调用方——各服要么用
// RegisterComponent（组件注册表），要么自建 zInject.Container（如 GameServer）。
// 该门面与组件注册表职责重叠、构成死 API，已移除以厘清职责边界：
//   - zServer：进程生命周期 + 7 态状态机 + 组件注册表（RegisterComponent）。
//   - zService：独立可选的服务编排模块（拓扑排序 Init/Serve/Close + DI），需要时由业务
//     直接 zService.NewServiceManager() 使用，不再由 BaseServer 强制内嵌。
