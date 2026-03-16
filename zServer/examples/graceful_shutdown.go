package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pzqf/zEngine/zServer"
)

// 示例：多种方式触发服务器优雅退出

// 示例1：通过 GM 命令触发
type GMCommandHandler struct {
	server *zServer.BaseServer
}

func NewGMCommandHandler(server *zServer.BaseServer) *GMCommandHandler {
	return &GMCommandHandler{server: server}
}

func (h *GMCommandHandler) HandleCommand(cmd string) {
	fmt.Printf("收到 GM 命令: %s\n", cmd)
	switch cmd {
	case "shutdown", "stop", "exit":
		fmt.Println("执行服务器关闭...")
		h.server.Shutdown()
	default:
		fmt.Printf("未知命令: %s\n", cmd)
	}
}

// 示例2：通过 HTTP 接口触发
type HTTPShutdownHandler struct {
	server *zServer.BaseServer
}

func NewHTTPShutdownHandler(server *zServer.BaseServer) *HTTPShutdownHandler {
	return &HTTPShutdownHandler{server: server}
}

func (h *HTTPShutdownHandler) StartHTTPServer(addr string) {
	http.HandleFunc("/shutdown", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		fmt.Println("收到 HTTP 关闭请求")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Server shutdown initiated"))
		go h.server.Shutdown()
	})

	fmt.Printf("HTTP 关闭接口启动在: http://%s/shutdown\n", addr)
	go http.ListenAndServe(addr, nil)
}

// 示例3：通过信号处理触发
func SetupSignalHandler(server *zServer.BaseServer) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		fmt.Printf("收到信号: %v\n", sig)
		server.Shutdown()
	}()
}

// 示例4：通过定时任务触发
func SetupGracefulShutdownTimer(server *zServer.BaseServer, duration time.Duration) {
	time.AfterFunc(duration, func() {
		fmt.Printf("定时关闭时间到，执行关闭...\n")
		server.Shutdown()
	})
}

// 示例5：通过健康检查触发
type HealthChecker struct {
	server       *zServer.BaseServer
	shouldStop   bool
	stopChan     chan struct{}
}

func NewHealthChecker(server *zServer.BaseServer) *HealthChecker {
	return &HealthChecker{
		server:   server,
		stopChan: make(chan struct{}),
	}
}

func (h *HealthChecker) Start(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if h.shouldStop {
				fmt.Println("健康检查失败，触发关闭")
				h.server.Shutdown()
				return
			}
		case <-h.stopChan:
			return
		}
	}
}

func (h *HealthChecker) Stop() {
	close(h.stopChan)
}

func (h *HealthChecker) SetShouldStop(shouldStop bool) {
	h.shouldStop = shouldStop
}

func main() {
	fmt.Println("=== 服务器优雅退出示例 ===")
	fmt.Println()

	// 创建示例服务器
	server := zServer.NewBaseServer(
		"example",
		"example-1",
		"Example Server",
		"1.0.0",
		&ExampleHooks{},
	)

	// 方式1：GM 命令
	gmHandler := NewGMCommandHandler(server)
	fmt.Println("方式1: GM 命令触发")
	fmt.Println("  输入 'shutdown' 命令来关闭服务器")
	go func() {
		for {
			var cmd string
			fmt.Scanln(&cmd)
			gmHandler.HandleCommand(cmd)
		}
	}()

	// 方式2：HTTP 接口
	httpHandler := NewHTTPShutdownHandler(server)
	httpHandler.StartHTTPServer(":8080")

	// 方式3：信号处理（Ctrl+C, kill）
	SetupSignalHandler(server)
	fmt.Println("方式3: 信号触发")
	fmt.Println("  按 Ctrl+C 或使用 kill 命令")

	// 方式4：定时关闭（可选）
	// SetupGracefulShutdownTimer(server, 30*time.Second)
	// fmt.Println("方式4: 定时关闭（30秒后自动关闭）")

	// 方式5：健康检查（可选）
	healthChecker := NewHealthChecker(server)
	go healthChecker.Start(10 * time.Second)
	fmt.Println("方式5: 健康检查失败触发")
	fmt.Println("  可以通过外部设置 shouldStop 来触发")

	fmt.Println()
	fmt.Println("=== 服务器启动中 ===")
	fmt.Println("可以通过以下方式关闭服务器：")
	fmt.Println("1. 输入 'shutdown' 命令")
	fmt.Println("2. 访问 http://localhost:8080/shutdown")
	fmt.Println("3. 按 Ctrl+C")
	fmt.Println("4. 使用 kill 命令")
	fmt.Println()

	// 启动服务器（阻塞）
	// 注意：这里只是示例，实际使用时需要设置 logger
	// server.SetLogger(logger)
	// server.Run()

	// 模拟运行
	time.Sleep(5 * time.Minute)
}

type ExampleHooks struct{}

func (h *ExampleHooks) OnBeforeStart() error {
	fmt.Println("OnBeforeStart: 执行启动前的准备工作")
	return nil
}

func (h *ExampleHooks) OnAfterStart() error {
	fmt.Println("OnAfterStart: 执行启动后的工作")
	return nil
}

func (h *ExampleHooks) OnBeforeStop() {
	fmt.Println("OnBeforeStop: 执行停止前的清理工作")
}
