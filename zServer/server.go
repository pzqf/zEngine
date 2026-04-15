package zServer

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/pzqf/zEngine/zLog"
	"github.com/pzqf/zEngine/zObject"
	"github.com/pzqf/zEngine/zService"
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
	stateListeners *zMap.TypedMap[uintptr, StateChangeListener]
	startTime      time.Time
	ctx            context.Context
	cancel         context.CancelFunc
	serviceMgr     *zService.ServiceManager
	hooks          LifecycleHooks
	logger         Logger
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
		stateListeners: zMap.NewTypedMap[uintptr, StateChangeListener](),
		startTime:      time.Now(),
		ctx:            ctx,
		cancel:         cancel,
		serviceMgr:     zService.NewServiceManager(),
		hooks:          hooks,
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

func (s *BaseServer) GetServiceManager() *zService.ServiceManager {
	return s.serviceMgr
}

func (s *BaseServer) AddService(svc zService.Service, deps ...interface{}) error {
	return s.serviceMgr.AddService(svc, deps...)
}

func (s *BaseServer) GetService(id interface{}) (zService.Service, error) {
	return s.serviceMgr.GetService(id)
}

func (s *BaseServer) InitServices() error {
	return s.serviceMgr.InitServices()
}

func (s *BaseServer) ServeServices() {
	s.serviceMgr.ServeServices()
}

func (s *BaseServer) CloseServices() error {
	return s.serviceMgr.CloseServices()
}

func (s *BaseServer) RegisterDependency(name string, factory interface{}) {
	s.serviceMgr.RegisterDependency(name, factory)
}

func (s *BaseServer) RegisterSingleton(name string, instance interface{}) {
	s.serviceMgr.RegisterSingleton(name, instance)
}

func (s *BaseServer) ResolveDependency(name string) (interface{}, error) {
	return s.serviceMgr.ResolveDependency(name)
}
