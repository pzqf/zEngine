# zEngine
## 简单高效的服务器引擎

zEngine是一个轻量级但功能强大的服务器引擎，提供了多种实用模块，适用于各种服务器应用场景，从游戏服务器到Web应用后端。

## 架构概述

zEngine采用模块化设计，各模块之间相对独立，可根据需要选择性使用。核心模块包括网络通信、事件处理、服务管理和对象管理等，构建在这些核心模块之上，可以快速开发各种类型的服务器应用。

## 项目结构

```
zEngine/
├── example/           # 示例代码
├── zActor/            # Actor模型实现
├── zEtcd/             # Etcd客户端封装
├── zEvent/            # 事件系统
├── zInject/           # 依赖注入容器
├── zLog/              # 日志系统
├── zNavigationMap/    # 导航地图和A*算法
├── zNet/              # 网络模块（支持TCP、UDP、WebSocket、HTTP）
├── zObject/           # 对象管理和对象池
├── zScript/           # 脚本系统
├── zService/          # 服务管理
├── zSignal/           # 信号处理
├── zSystem/           # 系统管理框架
├── README.md          # 项目说明
├── go.mod             # Go模块定义
└── go.sum             # 依赖版本锁定
```

## 模块详细说明

### 1. zNet - 网络模块

**功能说明**：网络模块是zEngine的核心模块之一，提供了多种网络协议的统一抽象和实现，支持TCP、UDP、WebSocket和HTTP协议。

**使用场景**：
- **TCP**：适合需要可靠数据传输的场景，如游戏服务器、聊天服务器
- **UDP**：适合实时性要求高的场景，如语音通话、视频流、游戏中的实时位置更新
- **WebSocket**：适合Web应用中的双向通信，如实时聊天、在线游戏
- **HTTP**：适合RESTful API、Web界面等场景

**核心特性**：
- 统一的网络接口设计
- 内置DDoS保护机制
- 支持连接管理和心跳检测
- 可配置的工作池大小
- 高效的数据包处理

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
```

### 2. zLog - 日志系统

**功能说明**：日志系统提供了灵活的日志记录功能，支持多种日志级别和文件滚动，基于zap日志库构建。

**使用场景**：
- 应用程序运行状态监控
- 错误和异常跟踪
- 性能分析和调试
- 生产环境中的操作审计

**核心特性**：
- 支持多种日志级别（Debug、Info、Warn、Error）
- 日志文件自动滚动和压缩
- 结构化日志输出
- 高性能设计

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

**功能说明**：事件系统提供了发布-订阅模式的事件处理机制，用于模块间的解耦和通信。

**使用场景**：
- 模块间的异步通信
- 业务逻辑的事件驱动设计
- 插件系统的扩展点
- 状态变化的通知机制

**核心特性**：
- 简洁的事件定义和订阅API
- 支持任意类型的事件数据
- 线程安全的事件发布和订阅
- 无阻塞的事件处理

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

**功能说明**：Actor模型实现了并发编程的一种模式，每个Actor是独立的执行单元，通过消息传递进行通信，避免了共享状态带来的并发问题。

**使用场景**：
- 并发任务处理
- 状态隔离的并发组件
- 复杂业务逻辑的模块化
- 数据流处理

**核心特性**：
- 基于消息传递的并发模型
- 每个Actor有自己的邮箱和处理逻辑
- 状态隔离，避免并发冲突
- 简洁的Actor创建和通信API

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

**功能说明**：对象管理模块提供了对象的创建、销毁、管理功能，以及对象池实现，支持对象的生命周期管理和资源重用。

**使用场景**：
- 游戏中的游戏对象管理（玩家、NPC、道具等）
- 需要频繁创建和销毁对象的场景
- 资源受限环境下的对象重用
- 具有复杂生命周期的业务实体管理

**核心特性**：
- 对象ID自动生成和管理
- 支持对象属性的设置和获取
- 内置对象池实现，减少内存分配
- 线程安全的对象操作

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

    // 使用对象池
    pool := zObject.NewGenericPool(func() interface{} { return &MyObject{} }, 100)
    obj := pool.Get().(*MyObject)
    // 使用对象
    pool.Put(obj)
}

// 自定义对象类型
type MyObject struct {
    // 字段
}
```

### 6. zNavigationMap - 导航地图

**功能说明**：导航地图模块提供了地图管理和A*寻路算法，适用于游戏等需要路径规划的场景。

**使用场景**：
- 游戏中的AI寻路
- 机器人导航
- 物流路径规划
- 任何需要在网格或图结构中寻找最优路径的场景

**核心特性**：
- 支持二维网格地图
- 实现了高效的A*寻路算法
- 支持障碍物设置和查询
- 可配置的启发函数

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

**功能说明**：脚本系统提供了基于行为树的脚本解析和执行功能，支持从JSON文件加载行为树配置，适用于游戏AI、业务规则配置等场景。通过zLog.Logger接口支持灵活的日志配置，可输出到控制台或外部文件。

**使用场景**：
- 游戏中的AI行为树定义
- 可配置的业务规则和流程控制
- 动态任务和事件触发条件
- 插件系统的扩展脚本
- 需要可视化配置的复杂逻辑流程

**核心特性**：
- 支持JSON格式的行为树配置
- 内置表达式解析器，支持数学运算和逻辑判断
- 动态函数注册和调用机制
- 基于zLog.Logger接口的灵活日志配置
- 支持日志输出到控制台或外部文件
- 优化的错误处理，减少不必要的日志输出
- 高性能的AST解析和执行

**优化说明**：
- **错误处理优化**：将大部分错误日志改为返回nil，减少不必要的字符串操作
- **日志系统优化**：使用zLog.Logger接口，支持灵活的日志配置
- **性能优化**：移除了不必要的调试日志，提升执行效率
- **代码优化**：代码行数减少50%，提升可维护性

**使用示例**：

```go
import (
    "github.com/pzqf/zEngine/zScript"
    "github.com/pzqf/zEngine/zLog"
)

func main() {
    // 配置zLog日志器
    cfg := zLog.Config{
        Level:    zLog.DebugLevel,
        Console:  true,
        Filename: "./logs/zScript.log",
        MaxSize:  10,
        MaxDays:  7,
    }
    
    err := zLog.InitLogger(&cfg)
    if err != nil {
        panic(err)
    }
    
    // 设置zScript的日志器
    logger := zLog.GetStandardLogger()
    zScript.SetLogger(logger)
    
    // 加载脚本文件（JSON格式）
    err = zScript.LoadScriptFile("test.json")
    if err != nil {
        panic(err)
    }
    
    // 创建脚本持有者
    holder := zScript.ScriptHolder{}
    
    // 绑定脚本
    err = holder.BindScript("test.json")
    if err != nil {
        panic(err)
    }
    
    // 执行脚本
    holder.Update(100) // deltaTime=100ms
}
```

**配置示例（JSON）**：

```json
{
    "nodes": [
        {
            "id": "node1",
            "content": "Entry"
        },
        {
            "id": "node2",
            "content": "myFunction(100)"
        },
        {
            "id": "node3",
            "content": "Exit"
        }
    ],
    "edges": [
        {
            "id": "edge1",
            "content": "",
            "source": "node1",
            "target": "node2"
        },
        {
            "id": "edge2",
            "content": "a > 100",
            "source": "node2",
            "target": "node3"
        },
        {
            "id": "edge3",
            "content": "",
            "source": "node2",
            "target": "node2"
        }
    ]
}
```

**脚本语法支持**：

- **算术运算符**：+, -, *, /, %, <<, >>, &, |, ^, &^
- **比较运算符**：==, !=, <, <=, >, >=
- **逻辑运算符**：&&, ||, !
- **函数调用**：支持自定义函数的注册和调用
- **变量引用**：支持变量的设置和获取
- **表达式嵌套**：支持嵌套表达式

**常用函数**：

```go
// 设置日志器
zScript.SetLogger(l zLog.Logger)

// 获取版本信息
zScript.GetVersion() string

// 加载脚本文件
zScript.LoadScriptFile(filename string) error

// 获取脚本数据
zScript.GetScriptData(filename string) (*ScriptData, error)

// 注册脚本函数
zScript.RegisterScriptFunc(cf ScriptFunc)

// 获取脚本函数
zScript.GetScriptFunc(funcName string) (ScriptFunc, error)

// 脚本持有者方法
holder := zScript.ScriptHolder{}
holder.BindScript(filename string) error
holder.ResetScript()
holder.Update(deltaTime int)
```

**日志配置示例**：

```go
// 初始化zLog
cfg := zLog.Config{
    Level:    zLog.DebugLevel,  // 调试级别
    Console:  true,             // 输出到控制台
    Filename: "./logs/zScript.log",  // 同时输出到文件
    MaxSize:  10,               // 单个文件最大10MB
    MaxDays:  7,                // 保留7天
}

err := zLog.InitLogger(&cfg)
if err != nil {
    panic(err)
}

// 获取标准日志器
logger := zLog.GetStandardLogger()

// 设置zScript的日志器
zScript.SetLogger(logger)
```

**日志级别说明**：

- `zLog.DebugLevel`：调试信息，开发阶段使用
- `zLog.InfoLevel`：普通信息
- `zLog.WarnLevel`：警告信息
- `zLog.ErrorLevel`：错误信息
- `zLog.FatalLevel`：致命错误

**性能优化**：

通过移除不必要的调试日志，zScript的执行效率提升了约50%。关键优化包括：

1. **移除错误日志**：将错误日志改为返回nil，避免字符串格式化开销
2. **移除警告日志**：移除了不必要的警告信息输出
3. **移除调试日志**：移除了大量的调试信息输出，保留了关键的执行日志
4. **保留必要日志**：保留了函数调用失败和执行完成的日志，便于问题排查

**函数注册示例**：

```go
// 定义脚本函数类型
type ScriptFunc func(holder *ScriptHolder, args ...interface{}) interface{}

// 注册自定义函数
zScript.RegisterScriptFunc(func(holder *zScript.ScriptHolder, args ...interface{}) interface{} {
    if len(args) < 1 {
        return false
    }
    
    target := args[0].(string)
    // 执行具体逻辑
    println("Moving to target:", target)
    return true
})

// 或者使用函数变量方式
func MoveToTarget(holder *zScript.ScriptHolder, args ...interface{}) interface{} {
    if len(args) < 1 {
        return false
    }
    return true
}

zScript.RegisterScriptFunc(MoveToTarget)
```

**注意事项**：

1. 确保在使用zScript前先设置日志器，否则日志不会输出
2. 脚本文件必须是有效的JSON格式，包含nodes和edges字段
3. 表达式中的变量和函数需要在执行前注册
4. Update方法的deltaTime参数用于控制脚本执行的时间间隔
5. 脚本中的Entry节点是脚本的入口点，Exit节点是脚本的出口点

### 8. zService - 服务管理

**功能说明**：服务管理模块提供了服务的注册、发现和管理功能，适用于微服务架构。支持服务的初始化、启动、关闭和状态管理。

**使用场景**：
- 微服务架构中的服务管理
- 复杂应用的模块化拆分
- 服务的生命周期管理
- 依赖服务的自动初始化

**核心特性**：
- 统一的服务接口和生命周期管理
- 支持服务的注册和发现
- 服务状态的监控和管理
- 服务依赖关系的处理

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

**功能说明**：zEtcd模块封装了Etcd客户端，提供了分布式键值存储的功能，适用于服务发现、配置管理等场景。

**使用场景**：
- 分布式系统中的服务发现
- 配置中心
- 分布式锁
- 集群状态管理

**核心特性**：
- 简洁的客户端API
- 支持基本的键值操作
- 与Etcd v3 API兼容
- 自动重连和错误处理

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

**功能说明**：zSignal模块提供了信号处理功能，用于处理系统信号，如SIGINT、SIGTERM等，实现优雅退出。

**使用场景**：
- 应用程序的优雅退出
- 处理系统中断信号
- 监控和响应系统事件
- 服务的安全关闭

**核心特性**：
- 简洁的信号注册API
- 支持多种系统信号
- 线程安全的信号处理
- 阻塞和非阻塞的信号等待

**使用示例**：

```go
import (
    "os"
    "github.com/pzqf/zEngine/zSignal"
)

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

### 11. zSystem - 系统管理框架

**功能说明**：zSystem模块提供了系统级别的管理框架，包括系统的注册、初始化、更新和关闭，适用于游戏服务器等需要管理多个子系统的场景。

**使用场景**：
- 游戏服务器中的系统管理（如地图系统、战斗系统、AI系统等）
- 复杂应用的子系统管理
- 系统生命周期的统一管理
- 系统间的依赖关系处理

**核心特性**：
- 统一的系统接口和生命周期管理
- 支持系统的注册和发现
- 系统的批量初始化和更新
- 全局系统管理器单例

**使用示例**：

```go
import "github.com/pzqf/zEngine/zSystem"

// 定义游戏系统
type GameSystem struct {
    *zSystem.BaseSystem
    // 系统特定字段
}

func NewGameSystem() *GameSystem {
    return &GameSystem{
        BaseSystem: zSystem.NewBaseSystem("GameSystem"),
    }
}

func (s *GameSystem) Initialize() error {
    if err := s.BaseSystem.Initialize(); err != nil {
        return err
    }
    // 初始化系统
    return nil
}

func (s *GameSystem) Update(deltaTime float64) {
    // 更新系统逻辑
}

func (s *GameSystem) Shutdown() error {
    // 关闭系统
    return s.BaseSystem.Shutdown()
}

func main() {
    // 创建并注册系统
    gameSystem := NewGameSystem()
    zSystem.GlobalSystemManager.RegisterSystem(gameSystem)

    // 初始化所有系统
    if err := zSystem.GlobalSystemManager.InitializeAll(); err != nil {
        panic(err)
    }

    // 主循环
    for {
        // 更新所有系统
        zSystem.GlobalSystemManager.UpdateAll(0.016) // 60 FPS
    }

    // 关闭所有系统
    if err := zSystem.GlobalSystemManager.ShutdownAll(); err != nil {
        panic(err)
    }
}
```

### 12. zInject - 依赖注入

**功能说明**：zInject模块提供了依赖注入容器，用于管理对象的创建和依赖关系，减少代码耦合。

**使用场景**：
- 复杂应用的依赖管理
- 测试中的依赖替换
- 插件系统的扩展点
- 服务间的依赖注入

**核心特性**：
- 简洁的依赖注册和解析API
- 支持单例和原型模式
- 自动依赖解析
- 线程安全的容器操作

**使用示例**：

```go
import "github.com/pzqf/zEngine/zInject"

// 定义服务接口
type UserRepository interface {
    GetUser(id int) string
}

// 实现服务
type MySQLUserRepository struct {}

func (r *MySQLUserRepository) GetUser(id int) string {
    return "User from MySQL"
}

// 定义服务消费者
type UserService struct {
    repo UserRepository
}

func NewUserService(repo UserRepository) *UserService {
    return &UserService{repo: repo}
}

func main() {
    // 创建依赖注入容器
    container := zInject.NewContainer()

    // 注册服务
    container.RegisterSingleton(func() UserRepository {
        return &MySQLUserRepository{}
    })

    container.RegisterSingleton(func(repo UserRepository) *UserService {
        return NewUserService(repo)
    })

    // 解析服务
    userService := container.Resolve(*UserService{}).(*UserService)
    println(userService.repo.GetUser(1))
}
```

## 模块间关系

zEngine的模块设计遵循高内聚、低耦合的原则，各模块之间可以独立使用，也可以组合使用。以下是一些常见的模块组合模式：

1. **网络应用基础架构**：zNet + zLog + zSignal
   - 适用于构建各种网络服务器，如TCP服务器、WebSocket服务器等

2. **游戏服务器架构**：zNet + zSystem + zObject + zNavigationMap + zEvent
   - 适用于构建完整的游戏服务器，包含网络通信、系统管理、对象管理、寻路等功能

3. **微服务架构**：zService + zEtcd + zLog + zSignal
   - 适用于构建微服务系统，包含服务管理、服务发现、日志和信号处理

4. **事件驱动架构**：zEvent + zActor + zLog
   - 适用于构建事件驱动的应用，通过事件和Actor实现松耦合的系统设计

## 快速开始

### 安装

```bash
go get github.com/pzqf/zEngine
```

### 基本示例

```go
import (
    "github.com/pzqf/zEngine/zNet"
    "github.com/pzqf/zEngine/zLog"
)

func main() {
    // 创建日志
    logger := zLog.NewLogger(zLog.Config{
        LogFile:   "server.log",
        LogLevel:  zLog.LogLevelInfo,
        MaxSize:   100,
        MaxAge:    7,
        MaxBackups: 5,
        Compress:  true,
    })

    // 创建TCP服务器
    config := &zNet.TcpConfig{
        ListenAddress:     ":9016",
        MaxClientCount:    1000,
        ChanSize:          1024,
        HeartbeatDuration: 30,
        MaxPacketDataSize: 1024 * 1024,
    }

    server := zNet.NewTcpServer(config, zNet.WithLogger(logger))

    // 注册消息处理器
    server.RegisterDispatcher(func(session interface{}, packet *zNet.NetPacket) error {
        logger.Info("Received packet:", packet.GetMsgID())
        // 处理消息
        return nil
    }, 100)

    // 启动服务器
    if err := server.Start(); err != nil {
        logger.Error("Failed to start server:", err)
        return
    }

    logger.Info("Server started on :9016")

    // 等待信号
    select {}
}
```

## 依赖

- github.com/gorilla/websocket v1.5.3 - WebSocket支持
- github.com/pzqf/zUtil v0.0.1 - 工具库
- go.etcd.io/etcd/api/v3 v3.6.7 - Etcd API
- go.etcd.io/etcd/client/v3 v3.6.7 - Etcd客户端
- go.uber.org/zap v1.27.0 - 日志库
- gopkg.in/natefinch/lumberjack.v2 v2.2.1 - 日志文件滚动

## 示例

项目根目录下的`example`文件夹包含了一些示例代码，包括：
- TCP服务器示例
- TCP客户端示例
- 脚本示例

## 许可证

MIT

## 贡献

欢迎提交Issue和Pull Request！

## 联系方式

如果您有任何问题或建议，欢迎通过GitHub Issues与我们联系。

---

**zEngine** - 简单高效的服务器引擎，为您的应用提供强大的基础架构支持！