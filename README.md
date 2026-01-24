# zEngine
## 简单高效的服务器引擎

zEngine是一个轻量级但功能强大的服务器引擎，提供了多种实用模块，适用于各种服务器应用场景。

## 项目结构

```
zEngine/
├── example/           # 示例代码
├── zActor/            # Actor模型实现
├── zEtcd/             # Etcd客户端封装
├── zEvent/            # 事件系统
├── zLog/              # 日志系统
├── zNavigationMap/    # 导航地图和A*算法
├── zNet/              # 网络模块（支持TCP、UDP、WebSocket、HTTP）
├── zObject/           # 对象管理
├── zScript/           # 脚本系统
├── zService/          # 服务管理
├── zSignal/           # 信号处理
├── README.md          # 项目说明
├── go.mod             # Go模块定义
└── go.sum             # 依赖版本锁定
```

## 模块介绍

### 1. zNet - 网络模块

网络模块是zEngine的核心模块之一，支持多种网络协议：
- **TCP**：可靠的面向连接的协议，适合需要保证数据传输可靠性的场景
- **UDP**：无连接的协议，适合实时性要求高的场景
- **WebSocket**：基于HTTP的双向通信协议，适合Web应用
- **HTTP**：标准的HTTP协议，适合RESTful API等场景

**DDoS保护**：
- **ConnectionLimiter**：限制每个IP的最大连接数
- **PacketLimiter**：限制每个IP的最大数据包数
- **TrafficLimiter**：限制每个IP的最大流量

**使用示例**：

```go
// TCP服务器
import (
    "github.com/pzqf/zEngine/zNet"
    "github.com/pzqf/zEngine/zLog"
)

func main() {
    // 创建日志
    logger := zLog.NewLogger(zLog.Config{
        LogFile:   "server.log",
        LogLevel:  zLog.LogLevelDebug,
        MaxSize:   100,
        MaxAge:    7,
        MaxBackups: 5,
        Compress:  true,
    })

    // 创建TCP服务器配置
    config := &zNet.TcpConfig{
        ListenAddress:     ":9016",
        MaxClientCount:    10000,
        ChanSize:          1024,
        HeartbeatDuration: 30,
        MaxPacketDataSize: 1024 * 1024,
    }

    // 创建TCP服务器
    server := zNet.NewTcpServer(config,
        zNet.WithLogger(logger),
        zNet.WithWorkerPoolSize(100),
    )

    // 注册消息处理器
    server.RegisterDispatcher(func(session interface{}, packet *zNet.NetPacket) error {
        // 处理接收到的数据包
        return nil
    }, 100)

    // 启动服务器
    if err := server.Start(); err != nil {
        panic(err)
    }

    // 等待退出
    select {}
}

// WebSocket服务器
func createWebSocketServer() {
    wsConfig := &zNet.WebSocketConfig{
        ListenAddress:     ":9018",
        MaxClientCount:    10000,
        HeartbeatDuration: 30,
        MaxPacketDataSize: 1024 * 1024,
    }

    wsServer := zNet.NewWebSocketServer(wsConfig,
        zNet.WithLogger(logger),
        zNet.WithWorkerPoolSize(100),
    )

    wsServer.RegisterDispatcher(func(session interface{}, packet *zNet.NetPacket) error {
        // 处理接收到的数据包
        return nil
    }, 100)

    if err := wsServer.Start(); err != nil {
        panic(err)
    }
}
```

### 2. zLog - 日志系统

日志系统提供了灵活的日志记录功能，支持多种日志级别和文件滚动。

**使用示例**：

```go
import "github.com/pzqf/zEngine/zLog"

func main() {
    // 创建日志配置
    config := zLog.Config{
        LogFile:   "app.log",
        LogLevel:  zLog.LogLevelDebug,
        MaxSize:   100,  // MB
        MaxAge:    7,    // 天
        MaxBackups: 5,   // 最大备份数
        Compress:  true, // 是否压缩
    }

    // 创建日志记录器
    logger := zLog.NewLogger(config)

    // 记录不同级别的日志
    logger.Debug("Debug message")
    logger.Info("Info message")
    logger.Warn("Warning message")
    logger.Error("Error message")
}
```

### 3. zEvent - 事件系统

事件系统提供了发布-订阅模式的事件处理机制，用于模块间的解耦。

**使用示例**：

```go
import "github.com/pzqf/zEngine/zEvent"

// 定义事件类型
const (
    EventTypeUserLogin zEvent.EventType = iota
    EventTypeUserLogout
)

func main() {
    // 创建事件总线
    eventBus := zEvent.NewEventBus()

    // 订阅事件
    eventBus.Subscribe(EventTypeUserLogin, func(event zEvent.Event) {
        // 处理用户登录事件
        data := event.Data().(map[string]interface{})
        userId := data["userId"].(int64)
        println("User logged in:", userId)
    })

    // 发布事件
    event := zEvent.NewEvent(EventTypeUserLogin, map[string]interface{}{
        "userId": 12345,
        "name":   "John Doe",
    })
    eventBus.Publish(event)
}
```

### 4. zActor - Actor模型

Actor模型实现了并发编程的一种模式，每个Actor是独立的执行单元，通过消息传递进行通信。

**使用示例**：

```go
import "github.com/pzqf/zEngine/zActor"

// 定义Actor消息
const (
    MessageTypePing zActor.MessageType = iota
    MessageTypePong
)

func main() {
    // 创建Actor系统
    actorSystem := zActor.NewActorSystem()

    // 创建Actor
    actor := actorSystem.Spawn(func(context *zActor.ActorContext) {
        for {
            select {
            case msg := <-context.Mailbox:
                switch msg.Type {
                case MessageTypePing:
                    // 处理Ping消息
                    println("Received ping")
                    // 发送Pong消息
                    context.Send(msg.Sender, zActor.NewMessage(MessageTypePong, nil))
                }
            }
        }
    })

    // 向Actor发送消息
    actor.Send(zActor.NewMessage(MessageTypePing, nil))
}
```

### 5. zObject - 对象管理

对象管理模块提供了对象的创建、销毁和管理功能，支持对象的生命周期管理。

**使用示例**：

```go
import "github.com/pzqf/zEngine/zObject"

// 定义对象类型
const (
    ObjectTypePlayer zObject.ObjectType = iota
    ObjectTypeNPC
)

func main() {
    // 创建对象管理器
    objMgr := zObject.NewObjectManager()

    // 创建对象
    player := objMgr.CreateObject(ObjectTypePlayer)
    player.SetAttr("name", "Player1")
    player.SetAttr("level", 10)

    // 获取对象
    obj := objMgr.GetObject(player.GetId())
    if obj != nil {
        name := obj.GetAttr("name").(string)
        level := obj.GetAttr("level").(int)
        println("Player:", name, "Level:", level)
    }

    // 销毁对象
    objMgr.DestroyObject(player.GetId())
}
```

### 6. zNavigationMap - 导航地图

导航地图模块提供了地图管理和A*寻路算法，适用于游戏等需要路径规划的场景。

**使用示例**：

```go
import "github.com/pzqf/zEngine/zNavigationMap"

func main() {
    // 创建导航地图
    navMap := zNavigationMap.NewMap(100, 100) // 100x100的地图

    // 设置障碍物
    navMap.SetBlock(10, 10, true)
    navMap.SetBlock(10, 11, true)

    // 寻路
    path, err := navMap.FindPath(0, 0, 50, 50)
    if err == nil {
        // 输出路径点
        for _, point := range path {
            println("Path point:", point.X, point.Y)
        }
    }
}
```

### 7. zScript - 脚本系统

脚本系统提供了简单的脚本解析和执行功能，支持基本的脚本语法。

**使用示例**：

```go
import "github.com/pzqf/zEngine/zScript"

func main() {
    // 创建脚本
    script := zScript.NewScript()

    // 注册函数
    script.RegisterFunction("add", func(args []interface{}) interface{} {
        a := args[0].(int)
        b := args[1].(int)
        return a + b
    })

    // 执行脚本
    result, err := script.Execute("add(1, 2)")
    if err == nil {
        println("Result:", result)
    }
}
```

### 8. zService - 服务管理

服务管理模块提供了服务的注册、发现和管理功能，适用于微服务架构。支持服务的初始化、启动、关闭和状态管理。

**使用示例**：

```go
import "github.com/pzqf/zEngine/zService"

// 定义用户服务
type UserService struct {
    zService.BaseService
    users map[string]string
}

func NewUserService() *UserService {
    return &UserService{
        BaseService: *zService.NewBaseService("user"),
        users:       make(map[string]string),
    }
}

func (s *UserService) Init() error {
    s.SetState(zService.ServiceStateInit)
    // 初始化服务资源
    return nil
}

func (s *UserService) Serve() {
    s.SetState(zService.ServiceStateRunning)
    // 启动服务逻辑，可启动后台协程
    // 注意：Serve()方法返回后，服务状态不会自动设置为停止，需要手动管理
}

func (s *UserService) Close() error {
    s.SetState(zService.ServiceStateStopping)
    // 清理服务资源
    s.SetState(zService.ServiceStateStopped)
    return nil
}

func (s *UserService) RegisterUser(username, password string) {
    s.users[username] = password
}

func main() {
    // 创建服务管理器
    svcMgr := zService.NewServiceManager()

    // 添加服务
    userService := NewUserService()
    if err := svcMgr.AddService(userService); err != nil {
        panic(err)
    }

    // 初始化所有服务
    if err := svcMgr.InitServices(); err != nil {
        panic(err)
    }

    // 启动所有服务
    svcMgr.ServeServices()

    // 获取服务
    userService, err := svcMgr.GetService("user")
    if err != nil {
        panic(err)
    }
    userService.(*UserService).RegisterUser("john", "password")

    // 关闭所有服务
    if err := svcMgr.CloseServices(); err != nil {
        panic(err)
    }
}
```

### 9. zEtcd - Etcd客户端

zEtcd模块封装了Etcd客户端，提供了分布式键值存储的功能。

**使用示例**：

```go
import "github.com/pzqf/zEngine/zEtcd"

func main() {
    // 创建Etcd客户端
    etcdClient, err := zEtcd.NewEtcdClient([]string{"localhost:2379"})
    if err != nil {
        panic(err)
    }

    // 设置键值
    err = etcdClient.Set("key", "value")
    if err != nil {
        panic(err)
    }

    // 获取键值
    value, err := etcdClient.Get("key")
    if err != nil {
        panic(err)
    }
    println("Value:", value)

    // 删除键
    err = etcdClient.Delete("key")
    if err != nil {
        panic(err)
    }
}
```

### 10. zSignal - 信号处理

zSignal模块提供了信号处理功能，用于处理系统信号。

**使用示例**：

```go
import "github.com/pzqf/zEngine/zSignal"

func main() {
    // 注册信号处理函数
    zSignal.RegisterSignalHandler(func(signal os.Signal) {
        println("Received signal:", signal)
        // 处理信号，例如优雅退出
    })

    // 等待信号
    zSignal.WaitForSignal()
}
```

## 安装

```bash
go get github.com/pzqf/zEngine
```

## 依赖

- github.com/panjf2000/ants - 协程池
- github.com/gorilla/websocket - WebSocket支持
- go.etcd.io/etcd/client/v3 - Etcd客户端
- go.uber.org/zap - 日志库
- gopkg.in/natefinch/lumberjack.v2 - 日志文件滚动

## 示例

项目根目录下的`example`文件夹包含了一些示例代码，包括：
- TCP服务器示例
- TCP客户端示例
- 脚本示例

## 许可证

MIT

## 贡献

欢迎提交Issue和Pull Request！
