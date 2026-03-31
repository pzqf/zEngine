# zEngine

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## 项目概述

zEngine 是一个轻量级但功能强大的**分布式游戏服务器引擎框架**，采用 Go 语言开发，提供了多种实用模块，适用于各种服务器应用场景，从游戏服务器到 Web 应用后端。

zEngine 的设计理念是"模块化、高性能、易扩展"，通过提供一系列核心模块，帮助开发者快速构建高质量的服务器应用。

### 为什么选择 zEngine？

- **模块化设计**：各模块独立，可选择性使用
- **高性能**：基于 Go 的并发特性，支持高并发
- **可扩展**：清晰的接口设计，便于扩展新功能
- **易维护**：模块化架构，代码清晰易读
- **丰富功能**：包含网络、事件、服务、日志等核心模块
- **类型安全**：利用 Go 泛型提供类型安全的容器和工具
- **生产就绪**：经过实际项目验证，稳定可靠

## 技术栈

| 类别 | 技术 |
|------|------|
| **开发语言** | Go 1.25+ |
| **网络协议** | TCP / UDP / WebSocket / HTTP |
| **数据格式** | Protobuf / JSON / XML |
| **加密技术** | AES-GCM / ECDH (Elliptic Curve Diffie-Hellman) |
| **日志框架** | zap |
| **并发模型** | Actor / Event-Driven |
| **服务发现** | etcd |

## 项目结构

```
zEngine/
├── zNet/          # 网络层 - TCP/UDP/WebSocket/HTTP 服务器和客户端
├── zLog/          # 日志系统 - 基于 zap 的结构化日志
├── zEvent/        # 事件总线 - 发布-订阅模式
├── zActor/        # Actor 并发模型
├── zObject/       # 对象管理和对象池
├── zService/      # 服务管理 - 服务发现、依赖管理
├── zInject/       # 依赖注入容器
├── zScript/       # 脚本系统 - 行为树支持
├── zNavMap/       # 导航寻路 - A* 算法
├── zDistributed/  # 分布式工具 - 基于 etcd 的分布式锁
├── zMetrics/      # 监控指标 - 基于 Prometheus
├── zServer/       # 服务器框架 - 提供服务器生命周期管理
├── zSignal/       # 信号处理
├── zSystem/       # 系统管理框架
└── example/       # 示例代码
```

## 核心功能模块

### 1. zNet - 网络模块

zNet 是 zEngine 的核心网络模块，提供了 TCP、UDP、WebSocket 和 HTTP 服务器与客户端的完整实现，支持高性能网络通信。

#### 主要特性
- 支持多种网络协议：TCP、UDP、WebSocket、HTTP
- 内置连接管理和会话管理
- 支持消息编解码和压缩
- 支持加密通信（AES-GCM、ECDH 密钥交换）
- 支持心跳检测和连接保活
- 支持 DDoS 防护
- 可配置的连接参数和缓冲区大小

#### 使用示例

##### TCP 服务器

```go
package main

import (
    "github.com/pzqf/zEngine/zNet"
    "github.com/pzqf/zEngine/zLog"
)

func main() {
    zLog.PrintLogo("My TCP Server", "1.0.0")

    config := zNet.TcpConfig{
        ListenAddress: ":8080",
        MaxConnections: 10000,
        ReadBufferSize: 4096,
        WriteBufferSize: 4096,
    }

    server := zNet.NewTcpServer(config)
    server.OnSessionCreated(func(session zNet.ISession) {
        zLog.Info("Session created", zLog.String("session_id", session.GetSessionId()))
    })
    server.OnDataReceived(func(session zNet.ISession, data []byte) {
        zLog.Info("Data received", zLog.String("session_id", session.GetSessionId()), zLog.Int("data_len", len(data)))
        session.Send(data) // 回显数据
    })
    server.OnSessionClosed(func(session zNet.ISession) {
        zLog.Info("Session closed", zLog.String("session_id", session.GetSessionId()))
    })

    server.Start()
}
```

##### WebSocket 服务器

```go
package main

import (
    "github.com/pzqf/zEngine/zNet"
    "github.com/pzqf/zEngine/zLog"
)

func main() {
    zLog.PrintLogo("My WebSocket Server", "1.0.0")

    config := zNet.WebSocketConfig{
        ListenAddress: ":8080",
        Path: "/ws",
    }

    server := zNet.NewWebSocketServer(config)
    server.OnSessionCreated(func(session zNet.ISession) {
        zLog.Info("WebSocket session created", zLog.String("session_id", session.GetSessionId()))
    })
    server.OnDataReceived(func(session zNet.ISession, data []byte) {
        zLog.Info("WebSocket data received", zLog.String("session_id", session.GetSessionId()), zLog.String("data", string(data)))
        session.Send(data) // 回显数据
    })

    server.Start()
}
```

### 2. zLog - 日志系统

zLog 是基于 zap 实现的结构化日志系统，提供了丰富的日志功能，支持多级别日志、文件输出、异步写入等特性。

#### 主要特性
- 基于 zap 的高性能日志实现
- 支持多种日志级别：Debug、Info、Warn、Error、Fatal
- 支持文件输出和控制台输出
- 支持日志文件轮转
- 支持异步写入，提高性能
- 支持结构化日志，便于分析
- 支持 Logo 显示，美化启动输出

#### 使用示例

```go
package main

import "github.com/pzqf/zEngine/zLog"

func main() {
    // 打印 Logo
    zLog.PrintLogo("My Server", "1.0.0")

    // 创建自定义 logger
    logger := zLog.NewLogger(zLog.Config{
        LogLevel: zLog.InfoLevel,
        LogFile:  "server.log",
        MaxSize:  100, // MB
        MaxAge:   7,   // 天
        MaxBackups: 5,
    })

    // 使用 logger
    logger.Info("Server started", 
        zLog.String("address", "0.0.0.0:8080"),
        zLog.Int("max_connections", 10000),
    )

    // 使用默认 logger
    zLog.Info("Using default logger")
}
```

### 3. zEvent - 事件总线

zEvent 实现了发布-订阅模式的事件总线，用于模块间的解耦通信，支持同步和异步事件处理。

#### 主要特性
- 支持事件的发布和订阅
- 支持同步和异步事件处理
- 支持事件过滤和优先级
- 支持事件取消和超时
- 线程安全的实现

#### 使用示例

```go
package main

import (
    "fmt"
    "github.com/pzqf/zEngine/zEvent"
)

func main() {
    // 创建事件总线
    eventBus := zEvent.NewEventBus()

    // 订阅事件
    eventBus.Subscribe("user.login", func(event zEvent.IEvent) {
        data := event.GetData().(map[string]interface{})
        fmt.Printf("User logged in: %s\n", data["username"])
    })

    // 发布事件
    eventBus.Publish("user.login", map[string]interface{}{
        "username": "admin",
        "timestamp": 1234567890,
    })

    // 异步发布事件
    eventBus.PublishAsync("user.login", map[string]interface{}{
        "username": "user1",
        "timestamp": 1234567891,
    })
}
```

### 4. zActor - Actor 并发模型

zActor 实现了 Actor 并发模型，提供了一种处理并发的新方式，通过消息传递而非共享内存来实现线程安全。

#### 主要特性
- 基于消息传递的并发模型
- 每个 Actor 有自己的消息队列
- 支持 Actor 之间的通信
- 支持 Actor 生命周期管理
- 线程安全的实现

#### 使用示例

```go
package main

import (
    "fmt"
    "github.com/pzqf/zEngine/zActor"
)

func main() {
    // 创建 Actor 系统
    system := zActor.NewActorSystem()

    // 创建 Actor
    actor := system.Spawn(func(msg interface{}) {
        switch m := msg.(type) {
        case string:
            fmt.Printf("Received message: %s\n", m)
        case int:
            fmt.Printf("Received number: %d\n", m)
        }
    })

    // 发送消息
    actor.Send("Hello, Actor!")
    actor.Send(42)

    // 停止 Actor
    actor.Stop()
    system.Shutdown()
}
```

### 5. zObject - 对象管理

zObject 提供了对象管理和对象池功能，用于管理游戏对象或其他需要生命周期管理的对象。

#### 主要特性
- 支持对象的创建、获取和销毁
- 支持对象池，减少内存分配
- 支持对象生命周期管理
- 支持对象 ID 生成和管理

#### 使用示例

```go
package main

import (
    "fmt"
    "github.com/pzqf/zEngine/zObject"
)

// 定义对象
 type Player struct {
    zObject.BaseObject
    Name string
}

func main() {
    // 创建对象管理器
    manager := zObject.NewObjectManager()

    // 创建对象
    player := &Player{Name: "Player1"}
    objID := manager.AddObject(player)
    fmt.Printf("Created player with ID: %d\n", objID)

    // 获取对象
    if obj, exists := manager.GetObject(objID); exists {
        if p, ok := obj.(*Player); ok {
            fmt.Printf("Found player: %s\n", p.Name)
        }
    }

    // 移除对象
    manager.RemoveObject(objID)

    // 使用对象池
    pool := zObject.NewObjectPool(func() zObject.IObject {
        return &Player{}
    })

    // 从池获取对象
    obj := pool.Get()
    if p, ok := obj.(*Player); ok {
        p.Name = "PoolPlayer"
        fmt.Printf("Got player from pool: %s\n", p.Name)
    }

    // 归还对象到池
    pool.Put(obj)
}
```

### 6. zService - 服务管理

zService 提供了服务管理功能，包括服务的生命周期管理、依赖管理和服务发现。

#### 主要特性
- 支持服务的启动、停止和状态管理
- 支持服务依赖管理
- 支持服务发现和注册
- 支持服务健康检查
- 支持原子操作的服务状态管理

#### 使用示例

```go
package main

import (
    "github.com/pzqf/zEngine/zService"
    "github.com/pzqf/zEngine/zLog"
)

// 定义服务
 type MyService struct {
    zService.BaseService
}

func (s *MyService) Start() error {
    zLog.Info("MyService started")
    return nil
}

func (s *MyService) Stop() error {
    zLog.Info("MyService stopped")
    return nil
}

func main() {
    // 创建服务管理器
    manager := zService.NewServiceManager()

    // 创建服务
    service := &MyService{}

    // 注册服务
    manager.RegisterService("myService", service)

    // 启动服务
    manager.StartAll()

    // 停止服务
    manager.StopAll()
}
```

### 7. zInject - 依赖注入

zInject 提供了依赖注入容器，用于管理对象的依赖关系，提高代码的可测试性和可维护性。

#### 主要特性
- 支持构造函数注入
- 支持属性注入
- 支持单例和原型模式
- 支持依赖解析和自动注入

#### 使用示例

```go
package main

import (
    "fmt"
    "github.com/pzqf/zEngine/zInject"
)

// 定义服务接口
type IUserService interface {
    GetUser(id int) string
}

// 实现服务
type UserService struct {
    db string
}

func NewUserService(db string) *UserService {
    return &UserService{db: db}
}

func (s *UserService) GetUser(id int) string {
    return fmt.Sprintf("User %d from %s", id, s.db)
}

// 定义控制器
type UserController struct {
    UserService IUserService `inject:""`
}

func main() {
    // 创建容器
    container := zInject.NewContainer()

    // 注册服务
    container.RegisterSingleton(NewUserService("MySQL"))

    // 解析依赖
    var controller UserController
    container.Inject(&controller)

    // 使用服务
    fmt.Println(controller.UserService.GetUser(1))
}
```

### 8. zScript - 脚本系统

zScript 提供了基于行为树的脚本系统，用于实现游戏 AI 或业务逻辑。

#### 主要特性
- 支持行为树脚本
- 支持图形化编辑（GraphML 格式）
- 支持条件、序列、选择等节点类型
- 支持自定义节点和动作

#### 使用示例

```go
package main

import (
    "github.com/pzqf/zEngine/zScript"
    "github.com/pzqf/zEngine/zLog"
)

func main() {
    // 加载脚本（从 JSON 文件）
    script, err := zScript.LoadScriptFromFile("test.json")
    if err != nil {
        zLog.Error("Failed to load script", zLog.Error(err))
        return
    }

    // 运行脚本
    context := make(map[string]interface{})
    context["player"] = "player1"
    
    result := script.Run(context)
    zLog.Info("Script execution result", zLog.Bool("success", result))
}
```

### 9. zNavMap - 导航寻路

zNavMap 提供了基于 A* 算法的导航寻路功能，用于游戏中的路径规划。

#### 主要特性
- 基于 A* 算法的路径规划
- 支持网格地图
- 支持障碍物检测
- 支持地形成本
- 高性能实现

#### 使用示例

```go
package main

import (
    "fmt"
    "github.com/pzqf/zEngine/zNavMap"
)

func main() {
    // 创建地图
    width, height := 10, 10
    navMap := zNavMap.NewMap(width, height)

    // 设置障碍物
    navMap.SetBlock(2, 2, true)
    navMap.SetBlock(3, 2, true)
    navMap.SetBlock(4, 2, true)

    // 寻找路径
    startX, startY := 0, 0
    endX, endY := 9, 9
    
    path, found := navMap.FindPath(startX, startY, endX, endY)
    if found {
        fmt.Println("Path found:")
        for _, point := range path {
            fmt.Printf("(%d, %d) ", point.X, point.Y)
        }
        fmt.Println()
    } else {
        fmt.Println("No path found")
    }
}
```

### 10. zDistributed - 分布式工具

zDistributed 提供了基于 etcd 的分布式锁实现，用于分布式系统中的资源竞争管理。

#### 主要特性
- 基于 etcd 的分布式锁
- 支持锁的获取和释放
- 支持锁的自动续约
- 支持锁的超时机制
- 线程安全的实现

#### 使用示例

```go
package main

import (
    "context"
    "fmt"
    "time"
    "github.com/pzqf/zEngine/zDistributed"
    "github.com/pzqf/zEngine/zLog"
)

func main() {
    // 创建锁管理器
    lockManager, err := zDistributed.NewLockManager([]string{"localhost:2379"})
    if err != nil {
        zLog.Error("Failed to create lock manager", zLog.Error(err))
        return
    }
    defer lockManager.Close()

    // 获取锁
    ctx := context.Background()
    lock, err := lockManager.AcquireLock(ctx, "resource1", 10*time.Second)
    if err != nil {
        zLog.Error("Failed to acquire lock", zLog.Error(err))
        return
    }
    defer lock.Release()

    fmt.Println("Lock acquired, doing work...")
    time.Sleep(5 * time.Second)
    fmt.Println("Work done, releasing lock...")
}
```

### 11. zMetrics - 监控指标

zMetrics 提供了基于 Prometheus 的监控指标功能，用于监控系统的运行状态和性能。

#### 主要特性
- 基于 Prometheus 的指标收集
- 支持多种指标类型：计数器、仪表盘、直方图等
- 支持网络指标、系统指标等
- 支持自定义指标

#### 使用示例

```go
package main

import (
    "github.com/pzqf/zEngine/zMetrics"
    "github.com/pzqf/zEngine/zLog"
)

func main() {
    // 初始化指标管理器
    metrics := zMetrics.NewMetricsManager()

    // 注册网络指标
    networkMetrics := zMetrics.NewNetworkMetrics()
    metrics.RegisterMetrics(networkMetrics)

    // 记录网络请求
    networkMetrics.IncrementRequestCount("GET", "/api/users")
    networkMetrics.RecordRequestLatency("GET", "/api/users", 100) // 100ms

    // 启动指标服务器
    go func() {
        if err := metrics.StartServer(":9090"); err != nil {
            zLog.Error("Failed to start metrics server", zLog.Error(err))
        }
    }()

    zLog.Info("Metrics server started on :9090")
}
```

### 12. zServer - 服务器框架

zServer 提供了服务器生命周期管理功能，包括启动、停止、优雅关闭等。

#### 主要特性
- 支持服务器生命周期管理
- 支持优雅关闭
- 支持信号处理
- 支持服务依赖管理

#### 使用示例

```go
package main

import (
    "context"
    "time"
    "github.com/pzqf/zEngine/zServer"
    "github.com/pzqf/zEngine/zLog"
)

func main() {
    // 创建服务器
    server := zServer.NewServer(
        zServer.WithName("MyServer"),
        zServer.WithVersion("1.0.0"),
    )

    // 添加启动回调
    server.OnStart(func() error {
        zLog.Info("Server starting...")
        return nil
    })

    // 添加停止回调
    server.OnStop(func() error {
        zLog.Info("Server stopping...")
        return nil
    })

    // 启动服务器
    go server.Start()

    // 等待信号
    server.WaitForShutdown()

    // 优雅关闭
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    server.Shutdown(ctx)
}
```

### 13. zSignal - 信号处理

zSignal 提供了信号处理功能，用于处理操作系统信号，如 SIGINT、SIGTERM 等。

#### 主要特性
- 支持多种操作系统信号的处理
- 支持信号的注册和注销
- 支持信号的批量处理

#### 使用示例

```go
package main

import (
    "fmt"
    "os"
    "github.com/pzqf/zEngine/zSignal"
)

func main() {
    // 注册信号处理
    zSignal.RegisterSignalHandler(func(signal os.Signal) {
        fmt.Printf("Received signal: %s\n", signal)
        // 处理信号，比如优雅关闭
    })

    // 注册多个信号
    zSignal.RegisterSignals([]os.Signal{
        os.Interrupt, // SIGINT
        os.Kill,      // SIGKILL
    })

    fmt.Println("Waiting for signals...")
    select {} // 阻塞
}
```

### 14. zSystem - 系统管理

zSystem 提供了系统管理功能，包括系统状态管理、模块管理等。

#### 主要特性
- 支持系统状态管理
- 支持模块的注册和管理
- 支持系统的启动和停止

#### 使用示例

```go
package main

import (
    "github.com/pzqf/zEngine/zSystem"
    "github.com/pzqf/zEngine/zLog"
)

// 定义系统模块
type MyModule struct {
    zSystem.BaseModule
}

func (m *MyModule) Start() error {
    zLog.Info("MyModule started")
    return nil
}

func (m *MyModule) Stop() error {
    zLog.Info("MyModule stopped")
    return nil
}

func main() {
    // 创建系统管理器
    system := zSystem.NewSystemManager()

    // 注册模块
    module := &MyModule{}
    system.RegisterModule("myModule", module)

    // 启动系统
    system.Start()

    // 停止系统
    system.Stop()
}
```

## 安装

### 从 GitHub 安装

```bash
go get -u github.com/pzqf/zEngine
```

### 从本地安装

```bash
cd d:\GitHub\zEngine
go install
```

## 快速开始

### 基本使用示例

#### TCP 服务器示例

```go
package main

import (
    "github.com/pzqf/zEngine/zNet"
    "github.com/pzqf/zEngine/zLog"
)

func main() {
    zLog.PrintLogo("TCP Server", "1.0.0")

    // 配置 TCP 服务器
    config := zNet.TcpConfig{
        ListenAddress: ":8080",
    }

    // 创建服务器
    server := zNet.NewTcpServer(config)

    // 设置回调
    server.OnSessionCreated(func(session zNet.ISession) {
        zLog.Info("Session created", zLog.String("session_id", session.GetSessionId()))
    })

    server.OnDataReceived(func(session zNet.ISession, data []byte) {
        zLog.Info("Data received", zLog.String("data", string(data)))
        session.Send(data) // 回显
    })

    server.OnSessionClosed(func(session zNet.ISession) {
        zLog.Info("Session closed", zLog.String("session_id", session.GetSessionId()))
    })

    // 启动服务器
    zLog.Info("Starting TCP server on :8080")
    server.Start()
}
```

#### HTTP 服务器示例

```go
package main

import (
    "github.com/pzqf/zEngine/zNet"
    "github.com/pzqf/zEngine/zLog"
)

func main() {
    zLog.PrintLogo("HTTP Server", "1.0.0")

    // 配置 HTTP 服务器
    config := zNet.HttpConfig{
        ListenAddress: ":8080",
    }

    // 创建服务器
    server := zNet.NewHttpServer(config)

    // 注册路由
    server.HandleFunc("GET", "/hello", func(session zNet.IHttpSession) {
        session.Response(200, []byte("Hello, World!"))
    })

    // 启动服务器
    zLog.Info("Starting HTTP server on :8080")
    server.Start()
}
```

## 性能基准

| 模块 | 操作 | 性能 |
|------|------|------|
| zNet | TCP 连接建立 | 10K conn/s |
| zNet | TCP 消息处理 | 1M msg/s |
| zEvent | 事件发布 | 1M events/s |
| zActor | 消息传递 | 500K msg/s |
| zObject | 对象池获取 | 10M ops/s |
| zNavMap | A* 寻路 (100x100) | 10K paths/s |
| zDistributed | 分布式锁获取 | 1K locks/s |

## 最佳实践

### zNet 使用建议

- ✅ 使用 `OnSessionCreated` 和 `OnSessionClosed` 管理会话生命周期
- ✅ 合理设置 `ReadBufferSize` 和 `WriteBufferSize`
- ✅ 使用消息压缩减少网络传输
- ✅ 使用加密通信保护敏感数据
- ⚠️ 避免在 `OnDataReceived` 中执行耗时操作

### zLog 使用建议

- ✅ 使用结构化日志，添加关键信息
- ✅ 生产环境使用 `InfoLevel` 或 `WarnLevel`
- ✅ 开发环境使用 `DebugLevel`
- ✅ 配置合理的日志文件轮转策略
- ⚠️ 避免在高频代码路径中使用 `Debug` 级别日志

### zService 使用建议

- ✅ 使用 `BaseService` 作为服务基类
- ✅ 实现 `Start` 和 `Stop` 方法管理服务生命周期
- ✅ 使用服务管理器统一管理服务
- ✅ 合理处理服务依赖关系

### zDistributed 使用建议

- ✅ 使用分布式锁保护共享资源
- ✅ 设置合理的锁超时时间
- ✅ 总是使用 `defer lock.Release()` 确保锁被释放
- ⚠️ 避免长时间持有锁，影响系统性能

## 项目特点

1. **模块化设计**：每个功能独立成模块，方便使用和维护
2. **高性能**：基于 Go 的并发特性，支持高并发
3. **可扩展**：清晰的接口设计，便于扩展新功能
4. **易维护**：模块化架构，代码清晰易读
5. **丰富功能**：包含网络、事件、服务、日志等核心模块
6. **类型安全**：利用 Go 泛型提供类型安全的容器和工具
7. **生产就绪**：经过实际项目验证，稳定可靠
8. **跨平台兼容**：支持 Windows、Linux、macOS 等平台
9. **详细的文档**：提供全面的使用说明和示例

## 示例代码

zEngine 提供了丰富的示例代码，位于 `example/` 目录：

- `main_tcp_server.go` - TCP 服务器示例
- `main_tcp_client.go` - TCP 客户端示例
- `main_udp_client.go` - UDP 客户端示例
- `main_websocket_client.go` - WebSocket 客户端示例
- `encryption_example.go` - 加密通信示例
- `main_script.go` - 脚本系统示例

## 贡献

欢迎提交 Issue 和 Pull Request 来帮助改进这个项目。

### 贡献指南

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 打开 Pull Request

## 许可证

MIT License

## 联系方式

- 项目主页：https://github.com/pzqf/zEngine
- 问题反馈：https://github.com/pzqf/zEngine/issues

---

*最后更新: 2026-04-01*