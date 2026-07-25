package zServer

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// LifecycleHooks 生命周期钩子接口
// 子类必须实现这三个方法
type LifecycleHooks interface {
	OnBeforeStart() error
	OnAfterStart() error
	OnBeforeStop()
}

// Start 启动服务器：要求当前状态为 Starting，依次执行 OnBeforeStart、OnAfterStart 钩子。
//
// 状态机契约（成熟化改造 Phase 1.3.1 明确）：
//   - Start 只负责失败兜底——任一钩子返回错误即置 Stopped 并返回该错误。
//   - **成功路径的状态推进（Initializing→Ready→Healthy）由业务在钩子内自行驱动**，
//     框架不自动推进，以保留灵活性（如在 OnBeforeStart 内 Initialize() 后校验依赖，
//     在 OnAfterStart 内 Ready() 再 Healthy()）。便捷方法见 Initialize()/Ready()/Healthy()。
//   - 因此若业务未在钩子内驱动状态，Start() 成功返回后服务器仍停留在 Starting。
//     此为有意设计（业务驱动），而非缺陷；Stop() 则由框架驱动 Draining→Stopped。
//
// 返回:
//   - error: 启动失败时返回错误（此时状态已被置为 Stopped）
func (s *BaseServer) Start() error {
	// 检查服务器状态，如果不是 Starting 状态，则返回错误
	if state := s.GetState(); state != StateStarting {
		return fmt.Errorf("server already running or stopped")
	}

	logger := s.logger
	serverType := s.ServerType

	if logger != nil {
		logger.Info("Starting %s Server...", serverType)
	}

	// 启动前的准备工作 - 调用子类实现（可能会注册组件）
	if err := s.hooks.OnBeforeStart(); err != nil {
		// 设置状态为已停止
		s.SetState(StateStopped, "start failed")
		if logger != nil {
			logger.Error("Failed to execute OnBeforeStart: %v", err)
		}
		return err
	}

	// 启动后的工作 - 调用子类实现
	if err := s.hooks.OnAfterStart(); err != nil {
		// 设置状态为已停止
		s.SetState(StateStopped, "start failed")
		if logger != nil {
			logger.Error("Failed to execute OnAfterStart: %v", err)
		}
		return err
	}

	// OPT-11: 启动成功后置 isRunning=true。此前从无处 Store(true)→IsRunning() 恒 false，是个
	// 永远返回假的死 API。现在与生命周期同步（Stop 里置 false）。
	s.isRunning.Store(true)

	if logger != nil {
		logger.Info("%s Server started successfully!", serverType)
	}
	return nil
}

// Stop 停止服务器
func (s *BaseServer) Stop() {
	// 检查服务器状态，如果已经是停止状态，则直接返回
	if state := s.GetState(); state.IsTerminal() {
		return
	}

	logger := s.logger
	serverType := s.ServerType

	if logger != nil {
		logger.Info("Stopping %s Server...", serverType)
	}

	// 设置状态为流量排空
	if err := s.SetState(StateDraining, "server stopping"); err != nil && logger != nil {
		logger.Warn("Failed to set draining state", "error", err)
	}

	// OPT-11: 置 isRunning=false（与 Start 的 true 对称）。
	s.isRunning.Store(false)

	// 停止前的工作 - 调用子类实现
	s.hooks.OnBeforeStop()

	// 取消上下文
	s.cancel()

	// 设置状态为已停止
	if err := s.SetState(StateStopped, "server stopped"); err != nil && logger != nil {
		logger.Warn("Failed to set stopped state", "error", err)
	}

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
