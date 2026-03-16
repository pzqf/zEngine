package zServer

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/pzqf/zEngine/zLog"
	"github.com/pzqf/zEngine/zObject"
	"github.com/pzqf/zUtil/zMap"
)

// Logger 日志接口（与 zNet 保持一致）
type Logger = zLog.Logger

// ServerType 服务器类型
// 使用 string 类型，允许用户自定义服务器类型
type ServerType string

// LifecycleHooks 生命周期钩子接口
// 子类必须实现这三个方法
type LifecycleHooks interface {
	OnBeforeStart() error
	OnAfterStart() error
	OnBeforeStop()
}

// BaseServer 基础服务器结构
type BaseServer struct {
	zObject.BaseObject
	ServerType    ServerType
	ServerName    string
	ServerVersion string
	isRunning     atomic.Bool
	components    *zMap.TypedMap[string, interface{}]
	ctx           context.Context
	cancel        context.CancelFunc

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
	return &BaseServer{
		BaseObject:    zObject.BaseObject{Id: serverId},
		ServerType:    serverType,
		ServerName:    serverName,
		ServerVersion: version,
		components:    zMap.NewTypedMap[string, interface{}](),
		ctx:           ctx,
		cancel:        cancel,
		hooks:         hooks,
	}
}

// SetLogger 设置日志记录器（依赖注入）
func (s *BaseServer) SetLogger(logger Logger) {
	s.logger = logger
}

// GetLogger 获取日志记录器
func (s *BaseServer) GetLogger() Logger {
	return s.logger
}

// Start 启动服务器
// 返回:
//   - error: 启动失败时返回错误
func (s *BaseServer) Start() error {
	// 原子操作：检查并设置运行状态
	if !s.isRunning.CompareAndSwap(false, true) {
		return fmt.Errorf("server already running")
	}

	logger := s.logger
	serverType := s.ServerType

	if logger != nil {
		logger.Info("Starting %s Server...", serverType)
	}

	// 启动前的准备工作 - 调用子类实现（可能会注册组件）
	if err := s.hooks.OnBeforeStart(); err != nil {
		s.isRunning.Store(false)
		if logger != nil {
			logger.Error("Failed to execute OnBeforeStart: %v", err)
		}
		return err
	}

	// 启动后的工作 - 调用子类实现
	if err := s.hooks.OnAfterStart(); err != nil {
		s.isRunning.Store(false)
		if logger != nil {
			logger.Error("Failed to execute OnAfterStart: %v", err)
		}
		return err
	}

	if logger != nil {
		logger.Info("%s Server started successfully!", serverType)
	}
	return nil
}

// Stop 停止服务器
func (s *BaseServer) Stop() {
	// 原子操作：检查并设置运行状态
	if !s.isRunning.CompareAndSwap(true, false) {
		return
	}

	logger := s.logger
	serverType := s.ServerType

	if logger != nil {
		logger.Info("Stopping %s Server...", serverType)
	}

	// 停止前的工作 - 调用子类实现
	s.hooks.OnBeforeStop()

	// 取消上下文
	s.cancel()

	if logger != nil {
		logger.Info("%s Server stopped gracefully", serverType)
	}
}

// Shutdown 优雅关闭服务器（可被外部调用）
// 可以通过 GM 命令、HTTP 接口等方式触发
func (s *BaseServer) Shutdown() {
	if s.logger != nil {
		s.logger.Info("Shutdown requested")
	}
	s.Stop()
}

// Run 运行服务器（阻塞方法）
// 启动顺序：
// 1. 检查日志是否初始化
// 2. 启动服务器（内部调用 OnBeforeStart 和 OnAfterStart）
// 3. 等待退出信号
// 4. 停止服务器（内部调用 OnBeforeStop）
//
// 支持的退出方式：
// - Ctrl+C (SIGINT)
// - kill 命令 (SIGTERM)
// - 调用 Shutdown() 方法
// - Context 取消
//
// 返回:
//   - error: 如果日志未初始化或启动失败返回错误
func (s *BaseServer) Run() error {
	logger := s.GetLogger()

	// ========== 第一步：检查日志是否初始化 ==========
	if logger == nil {
		return fmt.Errorf("logger not initialized: must call SetLogger() before Run()")
	}

	// ========== 第二步：启动服务器 ==========
	logger.Info("Starting server...")
	if err := s.Start(); err != nil {
		logger.Error("Failed to start server: %v", err)
		return err
	}
	logger.Info("=====Server started successfully=====")

	// ========== 第三步：等待退出信号 ==========
	logger.Info("Server is running. Press Ctrl+C to stop.")

	// 创建信号通道
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 等待退出信号或上下文取消
	select {
	case sig := <-sigChan:
		logger.Info("Shutdown signal received (%v), stopping server...", sig)
	case <-s.ctx.Done():
		logger.Info("Context canceled, stopping server...")
	}

	// ========== 第四步：停止服务器 ==========
	s.Stop()

	logger.Info("Server shutdown complete")
	return nil
}

// Wait 等待服务器停止
func (s *BaseServer) Wait() {
	<-s.ctx.Done()
}

// RegisterComponent 注册组件
func (s *BaseServer) RegisterComponent(name string, component interface{}) {
	s.components.Store(name, component)
	if s.logger != nil {
		s.logger.Info("Component registered: %s", name)
	}
}

// GetComponent 获取组件
func (s *BaseServer) GetComponent(name string) interface{} {
	value, _ := s.components.Load(name)
	return value
}

// GetComponents 获取所有组件
func (s *BaseServer) GetComponents() map[string]interface{} {
	components := make(map[string]interface{})
	s.components.Range(func(key string, value interface{}) bool {
		components[key] = value
		return true
	})
	return components
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
