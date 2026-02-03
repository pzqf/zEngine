# zEngine

## 🏗️ 架构设计文档

zEngine是一个轻量级但功能强大的服务器引擎，提供了多种实用模块，适用于各种服务器应用场景，从游戏服务器到Web应用后端。

---

## 📋 目录

- [项目概述](#-项目概述)
- [架构设计理念](#-架构设计理念)
- [核心设计模式](#-核心设计模式)
- [模块架构详解](#-模块架构详解)
- [实现方式详解](#-实现方式详解)
- [新手使用指引](#-新手使用指引)
- [最佳实践](#-最佳实践)

---

## 🎯 项目概述

### 项目定位

zEngine是一个**模块化、高性能、可扩展**的服务器引擎框架，专为游戏服务器和Web应用后端设计。

### 技术栈

- **语言**：Go 1.25+
- **核心依赖**：zap、sync.Map、context
- **网络协议**：TCP、UDP、WebSocket、HTTP
- **数据格式**：Protobuf、JSON、XML
- **加密技术**：AES-GCM、ECDH (Elliptic Curve Diffie-Hellman)

### 项目特点

1. **模块化设计** - 各模块独立，可选择性使用
2. **高性能** - 基于Go的并发特性，支持高并发
3. **可扩展** - 清晰的接口设计，便于扩展新功能
4. **易维护** - 模块化架构，代码清晰易读
5. **丰富的功能** - 包含网络、事件、服务、日志等核心模块

---

## 🧠 架构设计理念

### 1. 模块化设计理念

```
核心原则：单一职责、低耦合、高内聚

每个模块负责一个特定的功能：
├── zNet/      - 网络通信（TCP/UDP/HTTP/WebSocket）
├── zLog/      - 日志系统
├── zEvent/    - 事件处理（发布-订阅模式）
├── zActor/    - Actor并发模型
├── zObject/   - 对象管理和对象池
├── zService/  - 服务管理（服务发现、依赖管理）
├── zScript/   - 脚本系统（行为树）
├── zNavigationMap/ - 导航寻路（A*算法）
├── zEtcd/     - Etcd客户端
├── zSignal/   - 信号处理
└── zSystem/   - 系统管理框架
```

### 2. 依赖注入理念

**理念：通过依赖注入实现解耦**

```go
// 注入容器
type Container struct {
    providers map[string]Provider
    singletons map[string]interface{}
}

// 单例注入
container.RegisterSingleton("db", NewDatabase())

// 工厂注入
container.Register("userService", func() interface{} {
    return NewUserService()
})
```

**优势**：
- 降低代码耦合度
- 提高代码可测试性
- 便于替换实现
- 实现自动依赖解析

### 3. 事件驱动理念

**理念：通过事件实现模块间解耦通信**

```go
// 事件总线
type EventBus struct {
    handlers map[EventType][]EventHandler
    mu       sync.RWMutex
}

// 事件处理流程
func (eb *EventBus) Publish(event Event) {
    for _, handler := range eb.handlers[event.Type()] {
        go handler(event)
    }
}
```

**优势**：
- 模块间解耦
- 异步处理，提高吞吐量
- 支持多订阅者
- 便于监控和扩展

### 4. Actor并发理念

**理念：基于消息传递的并发模型**

```go
// Actor定义
type Actor struct {
    id      string
    mailbox chan Message
    handler MessageHandler
}

// 消息处理
func (a *Actor) Process() {
    for msg := <-a.mailbox {
        a.handler(msg)
    }
}
```

**优势**：
- 状态隔离，避免竞态条件
- 消息驱动，代码清晰
- 天然的并发扩展性
- 便于调试和监控

### 5. 服务架构理念

**理念：模块化服务管理，支持依赖注入**

```go
// 服务基类
type BaseService struct {
    zObject.BaseObject
    state ServiceState
    mu    sync.RWMutex
}

// 服务状态管理
type ServiceState int

const (
    ServiceStateCreated ServiceState = iota
    ServiceStateInit
    ServiceStateRunning
    ServiceStateStopping
    ServiceStateStopped
)
```

**优势**：
- 统一的服务生命周期管理
- 支持服务依赖自动解析
- 服务状态监控
- 并行启动，提高启动效率

---

## 🎨 核心设计模式

### 1. 单例模式 (Singleton)

**应用场景**：
- 全局配置管理
- 数据库连接池
- 日志系统
- 事件总线

**实现方式**：
```go
// 使用 sync.Once 确保线程安全
var (
    configInstance *Config
    configOnce     sync.Once
)

func GetConfig() *Config {
    configOnce.Do(func() {
        configInstance = &Config{}
    })
    return configInstance
}
```

**优势**：
- 全局唯一实例
- 延迟初始化
- 线程安全
- 减少内存占用

### 2. 工厂模式 (Factory)

**应用场景**：
- 对象创建复杂或有多种变体
- 需要解耦对象创建和使用
- 支持动态创建

**实现方式**：
```go
// 对象创建工厂
type GameObjectFactory struct{}

func (f *GameObjectFactory) CreateObject(objType int) *GameObject {
    switch objType {
    case PlayerType:
        return &Player{}
    case NpcType:
        return &Npc{}
    case MonsterType:
        return &Monster{}
    default:
        return &GameObject{}
    }
}
```

**优势**：
- 解耦创建和使用
- 支持多态
- 便于扩展新类型
- 集中管理对象创建

### 3. 观察者模式 (Observer)

**应用场景**：
- 事件通知
- 状态变化通知
- 多订阅者监听

**实现方式**：
```go
// 事件总线实现
type EventBus struct {
    handlers map[EventType][]func(interface{})
    mu       sync.RWMutex
}

func (eb *EventBus) Subscribe(et EventType, handler func(interface{})) {
    eb.mu.Lock()
    defer eb.mu.Unlock()
    eb.handlers[et] = append(eb.handlers[et], handler)
}

func (eb *EventBus) Publish(et EventType, data interface{}) {
    eb.mu.RLock()
    defer eb.mu.RUnlock()
    for _, handler := range eb.handlers[et] {
        go handler(data)
    }
}
```

**优势**：
- 松耦合
- 支持多订阅者
- 事件广播
- 动态订阅/取消

### 4. 策略模式 (Strategy)

**应用场景**：
- 算法选择
- 行为变化
- 运行时动态切换

**实现方式**：
```go
// 加密策略接口
type EncryptStrategy interface {
    Encrypt(data []byte) []byte
    Decrypt(data []byte) []byte
}

// 具体策略实现
type AESStrategy struct{}
type RSAStrategy struct{}

// 策略使用
type CryptoService struct {
    strategy EncryptStrategy
}

func (cs *CryptoService) SetStrategy(strategy EncryptStrategy) {
    cs.strategy = strategy
}
```

**优势**：
- 运行时动态切换策略
- 避免复杂的条件判断
- 易于扩展新策略
- 代码清晰易读

### 5. 模板方法模式 (Template Method)

**应用场景**：
- 算法框架固定，细节变化
- 生命周期管理
- 统一接口，可变实现

**实现方式**：
```go
// 服务生命周期模板
type BaseService struct {
    state ServiceState
}

func (s *BaseService) Start() error {
    if err := s.Initialize(); err != nil {
        return err
    }
    if err := s.Run(); err != nil {
        return err
    }
    return nil
}

// 子类实现具体逻辑
func (s *MyService) Initialize() error { /* 子类实现 */ }
func (s *MyService) Run() error { /* 子类实现 */ }
```

**优势**：
- 固定算法骨架
- 灵活的实现细节
- 代码复用
- 统一的接口规范

### 6. 装饰器模式 (Decorator)

**应用场景**：
- 动态添加功能
- 扩展对象行为
- 替代继承

**实现方式**：
```go
// 网络连接装饰器
type ConnectionDecorator struct {
    Connection
}

func (cd *ConnectionDecorator) Send(data []byte) error {
    // 添加加密
    encrypted := cd.Encrypt(data)
    return cd.Connection.Send(encrypted)
}

func (cd *ConnectionDecorator) Recv() ([]byte, error) {
    data, err := cd.Connection.Recv()
    if err != nil {
        return nil, err
    }
    // 添加解密
    return cd.Decrypt(data), nil
}
```

**优势**：
- 动态扩展功能
- 避免类爆炸
- 灵活的组合
- 保持接口一致性

### 7. 适配器模式 (Adapter)

**应用场景**：
- 接口适配
- 第三方库集成
- 遗留系统迁移

**实现方式**：
```go
// 协议适配器
type ProtocolAdapter struct {
    original Protocol
}

func (pa *ProtocolAdapter) Decode(data []byte) (*Packet, error) {
    // 适配旧协议
    oldData := pa.AdaptToOldFormat(data)
    return pa.original.Decode(oldData)
}

func (pa *ProtocolAdapter) Encode(packet *Packet) ([]byte, error) {
    data, err := pa.original.Encode(packet)
    if err != nil {
        return nil, err
    }
    // 适配新协议
    return pa.AdaptToNewFormat(data), nil
}
```

**优势**：
- 接口统一
- 兼容旧系统
- 灵活的适配
- 代码解耦

### 8. 桥接模式 (Bridge)

**应用场景**：
- 分离抽象和实现
- 支持多维度变化
- 避免类爆炸

**实现方式**：
```go
// 网络桥接
type Network interface {
    Send(data []byte) error
    Recv() ([]byte, error)
}

type TCPNetwork struct{}
type WebSocketNetwork struct{}

type NetworkManager struct {
    network Network
}

func (nm *NetworkManager) SetNetwork(network Network) {
    nm.network = network
}
```

**优势**：
- 分离抽象与实现
- 支持独立扩展
- 降低耦合度
- 提高可维护性

### 9. 享元模式 (Flyweight)

**应用场景**：
- 大量细粒度对象
- 内存资源限制
- 对象共享复用

**实现方式**：
```go
// 对象池实现
type ObjectPool struct {
    pool  chan *GameObject
    newFunc func() *GameObject
}

func (op *ObjectPool) Get() *GameObject {
    select {
    case obj := <-op.pool:
        return obj
    default:
        return op.newFunc()
    }
}

func (op *ObjectPool) Put(obj *GameObject) {
    select {
    case op.pool <- obj:
    default:
        // 池已满，忽略
    }
}
```

**优势**：
- 减少内存占用
- 提高对象创建效率
- 支持资源复用
- 降低GC压力

### 10. 状态模式 (State)

**应用场景**：
- 对象状态变化多
- 条件判断复杂
- 状态驱动行为

**实现方式**：
```go
// 连接状态机
type ConnectionState interface {
    Connect(conn *Connection)
    Send(conn *Connection, data []byte) error
    Close(conn *Connection)
}

type ConnectingState struct{}
type ConnectedState struct{}
type ClosedState struct{}

func (s *ConnectingState) Send(conn *Connection, data []byte) error {
    return errors.New("连接中，无法发送")
}

func (s *ConnectedState) Send(conn *Connection, data []byte) error {
    return conn.network.Send(data)
}
```

**优势**：
- 清晰的状态转换
- 避免复杂的条件判断
- 便于扩展新状态
- 代码结构清晰

---

## 🏗️ 模块架构详解

### 1. zNet - 网络模块

#### 架构设计

```
┌─────────────────────────────────────────────────┐
│                  TcpServer                       │
├─────────────────────────────────────────────────┤
│  AcceptLoop()    ConnectionManager    WorkerPool │
├─────────────────────────────────────────────────┤
│                  TcpConfig                       │
│  ListenAddress  MaxClientCount  ChanSize        │
│  HeartbeatDuration  MaxPacketDataSize            │
└─────────────────────────────────────────────────┘
         │
         └──> ┌─────────────────────────────────────────┐
              │               Connection                │
              ├─────────────────────────────────────────┤
              │  Session  SendChan  RecvChan   Status  │
              ├─────────────────────────────────────────┤
              │  Send(data)  Recv()  Close()            │
              └─────────────────────────────────────────┘
```

#### 设计理念

**核心理念：高性能、可扩展、线程安全**

1. **事件驱动模型**
   - 基于epoll/kqueue的事件循环
   - 非阻塞I/O操作
   - 高并发处理能力

2. **连接管理**
   - 连接池管理空闲连接
   - 连接状态监控
   - 心跳检测机制

3. **工作池模式**
   - 多goroutine处理请求
   - 动态调整工作者数量
   - 负载均衡

4. **类型安全**
   - 使用 TypedMap 实现类型安全的连接管理
   - 编译时检查类型，避免运行时panic
   - 无需类型断言，代码更简洁

#### 实现方式

```go
// TCP服务器实现
func (s *TcpServer) Start() error {
    ln, err := net.Listen("tcp", s.config.ListenAddress)
    if err != nil {
        return err
    }

    // 启动接受循环
    go s.acceptLoop(ln)

    // 启动工作池
    s.startWorkerPool()

    return nil
}

// 接受新连接
func (s *TcpServer) acceptLoop(ln net.Listener) {
    for {
        conn, err := ln.Accept()
        if err != nil {
            // 处理错误
            continue
        }

        // 创建会话
        session := zNet.NewSession(conn)

        // 分配工作者
        worker := s.getWorker()
        worker.sessionChan <- session
    }
}
```

#### 性能优化

1. **连接复用** - 连接池管理，减少创建销毁开销
2. **零拷贝** - 避免不必要的数据复制
3. **批量处理** - 批量发送/接收数据
4. **内存池** - 减少内存分配和GC压力
5. **DDoS防护** - 限流、限速、连接限制

### 2. zLog - 日志模块

#### 架构设计

```
┌─────────────────────────────────────────────────┐
│                    Logger                        │
├─────────────────────────────────────────────────┤
│  Debug()  Info()  Warn()  Error()  Fatal()      │
├─────────────────────────────────────────────────┤
│                    Config                        │
│  LogLevel  LogFile  MaxSize  MaxAge  Compress   │
└─────────────────────────────────────────────────┘
         │
         └──> ┌─────────────────────────────────────────┐
              │             ZapLogger                   │
              ├─────────────────────────────────────────┤
              │  Writer   Encoder   Sampler             │
              └─────────────────────────────────────────┘
         │
         └──> ┌─────────────────────────────────────────┐
              │            Rotation                     │
              ├─────────────────────────────────────────┤
              │  RotateSize  RotateAge  RotateCompress  │
              └─────────────────────────────────────────┘
```

#### 设计理念

**核心理念：结构化、高性能、可配置**

1. **结构化日志**
   - JSON格式输出
   - 支持自定义字段
   - 便于日志分析

2. **多级别日志**
   - Debug、Info、Warn、Error、Fatal
   - 可配置的日志级别
   - 动态日志级别调整

3. **文件轮转**
   - 基于大小轮转
   - 基于时间轮转
   - 自动压缩旧日志

#### 实现方式

```go
// 日志初始化
func NewLogger(config Config) *Logger {
    zcfg := zap.NewProductionConfig()
    zcfg.OutputPaths = []string{config.LogFile, "stdout"}
    zcfg.Level = zap.NewAtomicLevelAt(zap.Level(config.LogLevel))

    logger, err := zcfg.Build()
    if err != nil {
        panic(err)
    }

    return &Logger{
        logger: logger,
        config: config,
    }
}

// 日志轮转
func (l *Logger) rotate() {
    // 检查文件大小
    if fileSize >= l.config.MaxSize {
        // 执行轮转
        l.rotateFile()
    }
    // 检查文件年龄
    if fileAge >= l.config.MaxAge {
        // 执行轮转
        l.rotateFile()
    }
}
```

#### 性能优化

1. **异步写入** - 日志缓冲，批量写入
2. **内存池** - 日志对象复用
3. **无锁并发** - CAS操作，减少锁竞争
4. **零拷贝** - 避免字符串复制

### 3. zEvent - 事件模块

#### 架构设计

```
┌─────────────────────────────────────────────────┐
│                   EventBus                       │
├─────────────────────────────────────────────────┤
│  handlers: map[EventType][]EventHandler         │
│  mu: sync.RWMutex                               │
│  running: atomic.Bool                           │
├─────────────────────────────────────────────────┤
│  Subscribe(EventType, EventHandler)             │
│  Unsubscribe(EventType, EventHandler)           │
│  Publish(Event)                                 │
│  PublishSync(Event)                             │
└─────────────────────────────────────────────────┘
         │
         └──> ┌─────────────────────────────────────────┐
              │                Event                   │
              ├─────────────────────────────────────────┤
              │  Type() EventType                    │
              │  Data() interface{}                  │
              └─────────────────────────────────────────┘
```

#### 设计理念

**核心理念：发布-订阅模式，异步事件处理**

1. **异步处理**
   - 事件发布不阻塞
   - 支持同步和异步两种方式
   - 提高系统响应能力

2. **多订阅者**
   - 同一事件可被多个订阅者处理
   - 订阅者独立处理
   - 支持动态订阅/取消订阅

3. **线程安全**
   - 读写锁保护
   - 原子操作
   - 并发安全的事件处理

#### 实现方式

```go
// 事件总线实现
type EventBus struct {
    handlers map[EventType][]EventHandler
    mu       sync.RWMutex
    running  atomic.Bool
}

// 订阅事件
func (eb *EventBus) Subscribe(et EventType, handler EventHandler) {
    eb.mu.Lock()
    defer eb.mu.Unlock()

    eb.handlers[et] = append(eb.handlers[et], handler)
}

// 发布事件（异步）
func (eb *EventBus) Publish(event Event) {
    eb.mu.RLock()
    defer eb.mu.RUnlock()

    handlers, exists := eb.handlers[event.Type()]
    if !exists {
        return
    }

    // 每个处理者独立goroutine
    for _, handler := range handlers {
        go handler(event)
    }
}

// 发布事件（同步）
func (eb *EventBus) PublishSync(event Event) {
    eb.mu.RLock()
    defer eb.mu.RUnlock()

    handlers, exists := eb.handlers[event.Type()]
    if !exists {
        return
    }

    // 同步执行
    for _, handler := range handlers {
        handler(event)
    }
}
```

#### 事件系统优化

1. **事件过滤** - 支持基于条件的事件过滤
2. **事件监控** - 事件处理统计和异常捕获
3. **事件队列** - 缓冲事件，避免突发流量
4. **事件优先级** - 支持不同优先级的事件处理
5. **事件回调** - 支持事件处理结果回调

### 4. zActor - Actor模块

#### 架构设计

```
┌─────────────────────────────────────────────────┐
│                  ActorSystem                     │
├─────────────────────────────────────────────────┤
│  actors: map[string]*Actor                       │
│  mu: sync.RWMutex                               │
├─────────────────────────────────────────────────┤
│  Spawn(name string, handler MessageHandler) *Actor│
│  GetActor(id string) *Actor                    │
│  StopActor(id string)                           │
└─────────────────────────────────────────────────┘
         │
         └──> ┌─────────────────────────────────────────┐
              │                Actor                   │
              ├─────────────────────────────────────────┤
              │  id: string                            │
              │  mailbox: chan Message                 │
              │  handler: MessageHandler              │
              ├─────────────────────────────────────────┤
              │  Send(msg Message)                    │
              │  Process()                             │
              │  Stop()                               │
              └─────────────────────────────────────────┘
         │
         └──> ┌─────────────────────────────────────────┐
              │                Message                 │
              ├─────────────────────────────────────────┤
              │  Type: MessageType                     │
              │  Data: interface{}                    │
              │  Sender: string                        │
              └─────────────────────────────────────────┘
```

#### 设计理念

**核心理念：消息驱动的并发模型**

1. **状态隔离**
   - 每个Actor有自己的状态
   - 状态修改仅在Actor内部
   - 避免竞态条件

2. **消息传递**
   - 所有通信通过消息
   - 消息队列缓冲
   - 有序的消息处理

3. **异步处理**
   - 消息处理不阻塞
   - 每个Actor独立goroutine
   - 高并发处理能力

#### 实现方式

```go
// Actor系统实现
type ActorSystem struct {
    actors map[string]*Actor
    mu     sync.RWMutex
}

// 创建Actor
func (as *ActorSystem) Spawn(name string, handler MessageHandler) *Actor {
    actor := &Actor{
        id:      name,
        mailbox: make(chan Message, 100),
        handler: handler,
    }

    // 启动消息处理循环
    go actor.Process()

    as.mu.Lock()
    defer as.mu.Unlock()
    as.actors[name] = actor

    return actor
}

// Actor消息处理
func (a *Actor) Process() {
    for msg := <-a.mailbox {
        a.handler(msg)
    }
}

// 发送消息
func (a *Actor) Send(msg Message) {
    a.mailbox <- msg
}
```

#### Actor系统优化

1. **消息路由** - 支持消息广播和定向发送
2. **消息优先级** - 支持不同优先级的消息处理
3. **消息压缩** - 减少网络传输量
4. **Actor监控** - Actor状态监控和异常恢复
5. **Actor池** - Actor实例复用，减少创建销毁开销

---

## 🔧 实现方式详解

### 1. 网络模块实现

#### TCP服务器实现

```go
// TcpServer结构体
type TcpServer struct {
    config     *TcpConfig
    logger      zLog.Logger
    workerPool  *WorkerPool
    connMgr     *ConnectionManager
    stopChan    chan struct{}
    wg          sync.WaitGroup
}

// 服务器启动
func (s *TcpServer) Start() error {
    ln, err := net.Listen("tcp", s.config.ListenAddress)
    if err != nil {
        return err
    }

    // 启动接受循环
    s.wg.Add(1)
    go s.acceptLoop(ln)

    // 启动工作池
    s.workerPool.Start()

    return nil
}

// 接受新连接
func (s *TcpServer) acceptLoop(ln net.Listener) {
    defer s.wg.Done()

    for {
        select {
        case <-s.stopChan:
            return
        default:
            // 设置接受超时
            ln.(*net.TcpListener).SetDeadline(time.Now().Add(time.Second))
            conn, err := ln.Accept()
            if err != nil {
                if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
                    continue
                }
                return
            }

            // 创建会话
            session := zNet.NewSession(conn)

            // 分配工作者
            worker := s.workerPool.GetWorker()
            worker.sessionChan <- session
        }
    }
}
```

#### 工作池实现

```go
// Worker结构体
type Worker struct {
    id         int
    sessionChan chan *Session
    stopChan    chan struct{}
    wg          sync.WaitGroup
}

// 工作者处理循环
func (w *Worker) loop() {
    defer w.wg.Done()

    for {
        select {
        case <-w.stopChan:
            return
        case session := <-w.sessionChan:
            // 处理会话
            w.processSession(session)
        }
    }
}

// 处理会话
func (w *Worker) processSession(session *Session) {
    defer func() {
        if r := recover(); r != nil {
            // 处理panic
        }
    }()

    // 启动接收循环
    go session.recvLoop()

    // 启动发送循环
    go session.sendLoop()
}

// 会话接收循环
func (s *Session) recvLoop() {
    defer s.Close()

    buf := make([]byte, 1024)
    for {
        n, err := s.conn.Read(buf)
        if err != nil {
            // 处理错误
            return
        }

        // 处理数据包
        packet, err := s.protocol.Decode(buf[:n])
        if err != nil {
            return
        }

        // 提交到处理队列
        s.handleChan <- packet
    }
}
```

### 2. 日志模块实现

#### 日志初始化

```go
// Logger结构体
type Logger struct {
    logger   *zap.Logger
    sugar    *zap.SugaredLogger
    config   Config
    stopChan chan struct{}
    wg       sync.WaitGroup
}

// 日志初始化
func NewLogger(config Config) *Logger {
    zcfg := zap.NewProductionConfig()
    zcfg.OutputPaths = []string{config.LogFile, "stdout"}
    zcfg.Level = zap.NewAtomicLevelAt(zap.Level(config.LogLevel))
    zcfg.Encoding = "json"

    logger, err := zcfg.Build()
    if err != nil {
        panic(err)
    }

    return &Logger{
        logger: logger,
        sugar:  logger.Sugar(),
        config: config,
    }
}

// 日志轮转
func (l *Logger) rotate() {
    ticker := time.NewTicker(time.Minute)
    defer ticker.Stop()

    for {
        select {
        case <-l.stopChan:
            return
        case <-ticker.C:
            // 检查文件大小
            if l.checkFileSize() {
                l.doRotate()
            }
        }
    }
}

// 执行轮转
func (l *Logger) doRotate() {
    // 关闭当前文件
    l.logger.Sync()

    // 重命名文件
    oldPath := l.config.LogFile
    newPath := fmt.Sprintf("%s.%d", l.config.LogFile, time.Now().Unix())
    os.Rename(oldPath, newPath)

    // 重新打开日志文件
    l.reopenLogFile()
}
```

### 3. 事件模块实现

#### 事件总线实现

```go
// EventBus结构体
type EventBus struct {
    handlers   map[EventType][]EventHandler
    mu         sync.RWMutex
    running    atomic.Bool
    eventChan  chan Event
    workerPool *WorkerPool
}

// 创建事件总线
func NewEventBus() *EventBus {
    bus := &EventBus{
        handlers:  make(map[EventType][]EventHandler),
        eventChan: make(chan Event, 1000),
    }
    bus.running.Store(true)

    // 启动工作池
    bus.workerPool = NewWorkerPool(10, 100)
    bus.workerPool.Start()

    // 启动事件处理循环
    go bus.loop()

    return bus
}

// 事件处理循环
func (eb *EventBus) loop() {
    for eb.running.Load() {
        select {
        case event := <-eb.eventChan:
            // 异步处理事件
            eb.workerPool.Submit(func() {
                eb.processEvent(event)
            })
        }
    }
}

// 处理单个事件
func (eb *EventBus) processEvent(event Event) {
    eb.mu.RLock()
    defer eb.mu.RUnlock()

    handlers, exists := eb.handlers[event.Type()]
    if !exists {
        return
    }

    // 每个处理者独立goroutine
    for _, handler := range handlers {
        go func(h EventHandler, e Event) {
            if r := recover(); r != nil {
                // 处理panic
            }
            h(e)
        }(handler, event)
    }
}
```

### 4. 服务模块实现

#### 服务管理器实现

```go
// ServiceManager结构体
type ServiceManager struct {
    services  map[string]Service
    dependencies map[string][]string
    mu       sync.RWMutex
    wg       sync.WaitGroup
}

// 服务初始化
func (sm *ServiceManager) InitServices() error {
    // 拓扑排序服务依赖
    order := sm.topologicalSort()

    // 初始化服务
    for _, serviceId := range order {
        service, exists := sm.services[serviceId]
        if !exists {
            continue
        }

        if err := service.Init(); err != nil {
            return err
        }
    }

    return nil
}

// 拓扑排序
func (sm *ServiceManager) topologicalSort() []string {
    // Kahn算法实现
    inDegree := make(map[string]int)
    queue := []string{}

    // 初始化入度
    for id := range sm.services {
        inDegree[id] = len(sm.dependencies[id])
        if inDegree[id] == 0 {
            queue = append(queue, id)
        }
    }

    var order []string

    for len(queue) > 0 {
        u := queue[0]
        queue = queue[1:]
        order = append(order, u)

        // 遍历u的所有依赖
        for _, v := range sm.dependencies[u] {
            inDegree[v]--
            if inDegree[v] == 0 {
                queue = append(queue, v)
            }
        }
    }

    return order
}

// 启动服务
func (sm *ServiceManager) ServeServices() {
    for id, service := range sm.services {
        serviceId := id
        sm.wg.Add(1)
        go func(s Service) {
            defer sm.wg.Done()
            s.Serve()
        }(service)
    }
}
```

---

## 📚 新手使用指引

### 快速开始

#### 步骤1：环境准备

```bash
# 安装Go环境
# 下载并安装：https://golang.org/dl/
go version  # 验证安装

# 配置GOPATH
export GOPATH=~/go
export PATH=$PATH:$GOPATH/bin

# 安装依赖管理工具
go install github.com/golang/dlv/cmd/dlv@latest
go install github.com/cosmtrek/air@latest
```

#### 步骤2：克隆项目

```bash
git clone https://github.com/pzqf/zEngine.git
cd zEngine

# 安装依赖
go mod tidy
```

#### 步骤3：运行示例

```bash
# 运行网络示例
go run example/tcp_server/main.go

# 运行日志示例
go run example/log_example/main.go

# 运行事件示例
go run example/event_example/main.go
```

### 模块使用示例

#### 1. 网络模块使用

```go
package main

import (
    "github.com/pzqf/zEngine/zNet"
    "github.com/pzqf/zEngine/zLog"
)

func main() {
    // 初始化日志
    logger := zLog.NewLogger(zLog.Config{
        LogFile:   "server.log",
        LogLevel:  zLog.DebugLevel,
        MaxSize:   100,
        MaxAge:    7,
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

#### 2. 日志模块使用

```go
package main

import (
    "github.com/pzqf/zEngine/zLog"
)

func main() {
    // 初始化日志
    cfg := zLog.Config{
        Level:    zLog.DebugLevel,
        Console:  true,
        Filename: "./logs/app.log",
        MaxSize:  10,
        MaxDays:  7,
        Compress: true,
    }

    err := zLog.InitLogger(&cfg)
    if err != nil {
        panic(err)
    }

    // 使用日志
    zLog.Debug("调试信息", zap.String("key", "value"))
    zLog.Info("普通信息", zap.Int("count", 100))
    zLog.Warn("警告信息", zap.Error(err))
    zLog.Error("错误信息", zap.String("error", "something wrong"))
}
```

#### 3. 事件模块使用

```go
package main

import (
    "github.com/pzqf/zEngine/zEvent"
)

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

#### 4. Actor模块使用

```go
package main

import (
    "github.com/pzqf/zEngine/zActor"
)

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
                    println("Received ping")
                    context.Send(msg.Sender, zActor.NewMessage(MessageTypePong, nil))
                }
            }
        }
    })

    // 向Actor发送消息
    actor.Send(zActor.NewMessage(MessageTypePing, nil))
}
```

#### 5. 服务模块使用

```go
package main

import (
    "github.com/pzqf/zEngine/zService"
)

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
    // 启动服务逻辑
}

func (s *UserService) Close() error {
    s.SetState(zService.ServiceStateStopping)
    // 清理服务资源
    s.SetState(zService.ServiceStateStopped)
    return nil
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
    svc, err := svcMgr.GetService("user")
    if err != nil {
        panic(err)
    }
    svc.(*UserService).users["john"] = "password"

    // 关闭所有服务
    if err := svcMgr.CloseServices(); err != nil {
        panic(err)
    }
}
```

### 开发指南

#### 1. 代码规范

```go
// 结构体命名：大驼峰式
type TcpServer struct {
    // 字段命名：小驼峰式（包内可见）
    config     *TcpConfig
    // 私有字段：小驼峰式
    workerPool *WorkerPool
}

// 函数命名：大驼峰式
func NewTcpServer(config *TcpConfig) *TcpServer {
    return &TcpServer{
        config: config,
    }
}

// 方法命名：大驼峰式
func (s *TcpServer) Start() error {
    // 实现代码
}
```

#### 2. 错误处理

```go
// 返回错误，而不是panic
func DoSomething() error {
    // 检查错误
    if err != nil {
        return err
    }

    // 处理错误
    return nil
}

// 调用错误处理
if err := DoSomething(); err != nil {
    // 记录错误
    zLog.Error("Failed to do something", zap.Error(err))
    // 返回错误
    return err
}
```

#### 3. 并发控制

```go
// 互斥锁
var mu sync.Mutex

func (s *Service) Process() {
    mu.Lock()
    defer mu.Unlock()

    // 临界区代码
}

// 读写锁
var rwmu sync.RWMutex

func (s *Service) Get() {
    rwmu.RLock()
    defer rwmu.RUnlock()

    // 读操作
}

func (s *Service) Set() {
    rwmu.Lock()
    defer rwmu.Unlock()

    // 写操作
}

// 原子操作
var counter atomic.Int32

func (s *Service) Increment() {
    counter.Add(1)
}
```

#### 4. 测试编写

```go
package zNet

import "testing"

func TestTcpServer_Start(t *testing.T) {
    // 测试TCP服务器启动
    config := &TcpConfig{
        ListenAddress: ":0", // 随机端口
    }

    server := NewTcpServer(config)
    err := server.Start()
    if err != nil {
        t.Fatal("Server failed to start:", err)
    }

    defer server.Stop()

    // 验证服务器已启动
    if server.IsRunning() == false {
        t.Error("Server should be running")
    }
}

func TestEventBus_Publish(t *testing.T) {
    // 测试事件总线发布
    bus := NewEventBus()

    received := false
    bus.Subscribe(EventTypeTest, func(event Event) {
        received = true
    })

    bus.Publish(NewEvent(EventTypeTest, nil))

    // 等待事件处理
    time.Sleep(100 * time.Millisecond)

    if !received {
        t.Error("Event should be received")
    }
}
```

---

## 🎯 最佳实践

### 1. 性能优化

#### 网络性能优化

1. **连接复用** - 使用连接池，减少连接创建销毁开销
2. **批量处理** - 批量发送/接收数据，减少系统调用
3. **零拷贝** - 避免不必要的数据复制
4. **异步I/O** - 使用非阻塞I/O，提高并发处理能力
5. **内存池** - 减少内存分配和GC压力

```go
// 连接池实现
type ConnectionPool struct {
    pool  chan *Connection
    newFunc func() *Connection
    mu    sync.Mutex
}

func (cp *ConnectionPool) Get() *Connection {
    select {
    case conn := <-cp.pool:
        return conn
    default:
        return cp.newFunc()
    }
}

func (cp *ConnectionPool) Put(conn *Connection) {
    select {
    case cp.pool <- conn:
    default:
        // 池已满，关闭连接
        conn.Close()
    }
}
```

#### 日志性能优化

1. **异步写入** - 日志缓冲，批量写入
2. **内存缓冲** - 使用内存池，减少分配
3. **结构化日志** - 避免字符串格式化开销
4. **采样策略** - 高频日志采样，减少存储

```go
// 异步日志
type AsyncLogger struct {
    logger *zap.Logger
    chan   chan *LogEntry
    wg     sync.WaitGroup
}

func (l *AsyncLogger) AsyncDebug(msg string, fields ...zap.Field) {
    l.chan <- &LogEntry{
        Level: zap.DebugLevel,
        Msg:   msg,
        Fields: fields,
    }
}

func (l *AsyncLogger) loop() {
    for entry := <-l.chan {
        l.logger.Log(entry.Level, entry.Msg, entry.Fields...)
    }
}
```

#### 事件性能优化

1. **事件过滤** - 避免不必要的事件处理
2. **事件缓冲** - 减少锁竞争
3. **事件合并** - 合并相似事件，减少处理开销
4. **事件优先级** - 重要事件优先处理

```go
// 事件过滤
func (eb *EventBus) Publish(event Event) {
    // 检查事件是否需要处理
    if !eb.shouldProcess(event) {
        return
    }

    // 发布事件
    eb.mu.RLock()
    defer eb.mu.RUnlock()

    handlers, exists := eb.handlers[event.Type()]
    if !exists {
        return
    }

    for _, handler := range handlers {
        go handler(event)
    }
}
```

### 2. 安全性最佳实践

#### 网络安全

1. **连接限制** - 限制每个IP的连接数
2. **流量限制** - 限制每个连接的流量
3. **数据包验证** - 验证数据包格式和内容
4. **加密通信** - 使用TLS加密数据传输

```go
// DDoS防护
type DdosProtector struct {
    ipMap map[string]*IpInfo
    mu    sync.RWMutex
}

func (dp *DdosProtector) Allow(ip string) bool {
    dp.mu.RLock()
    info, exists := dp.ipMap[ip]
    dp.mu.RUnlock()

    if !exists {
        return true
    }

    // 检查连接数
    if info.ConnCount > 100 {
        return false
    }

    // 检查流量
    if info.Traffic > 100*1024*1024 {
        return false
    }

    return true
}
```

#### 代码安全

1. **输入验证** - 验证所有外部输入
2. **SQL注入防护** - 使用参数化查询
3. **XSS防护** - 过滤用户输入
4. **CSRF防护** - 使用Token验证

```go
// 输入验证
func ValidateInput(input string) error {
    if input == "" {
        return errors.New("输入不能为空")
    }

    if len(input) > 100 {
        return errors.New("输入过长")
    }

    // 正则表达式验证
    match := regexp.MustCompile(`^[a-zA-Z0-9_]+$`).MatchString(input)
    if !match {
        return errors.New("输入包含非法字符")
    }

    return nil
}
```

### 3. 可维护性最佳实践

#### 代码组织

1. **模块化** - 按功能划分模块
2. **单一职责** - 每个函数/类只做一件事
3. **接口抽象** - 使用接口，不依赖实现
4. **依赖注入** - 通过依赖注入实现解耦

```go
// 模块化示例
package game

import (
    "github.com/pzqf/zEngine/zActor"
    "github.com/pzqf/zEngine/zEvent"
)

// 战斗模块
type CombatModule struct {
    actor *zActor.Actor
    eventBus *zEvent.EventBus
}

// 移动模块
type MovementModule struct {
    actor *zActor.Actor
    eventBus *zEvent.EventBus
}

// 技能模块
type SkillModule struct {
    actor *zActor.Actor
    eventBus *zEvent.EventBus
}
```

#### 文档编写

1. **代码注释** - 注释复杂逻辑和关键算法
2. **API文档** - 使用godoc生成API文档
3. **README** - 项目功能和使用说明
4. **CHANGELOG** - 版本变更记录

```go
// 战斗系统实现
// 
// 战斗系统负责处理玩家和怪物之间的战斗逻辑，
// 包括攻击、防御、技能释放、状态效果等。
// 
// 核心功能：
// 1. 攻击计算 - 根据属性和技能计算伤害
// 2. 状态管理 - 管理BUFF、DEBUFF等状态
// 3. 仇恨系统 - 管理怪物的目标选择
// 4. 战斗结果 - 处理胜负和奖励
// 
// 使用示例：
//   combat := NewCombatSystem()
//   combat.Attack(attacker, target)
// 
type CombatSystem struct {
    // 技能管理器
    skillMgr *SkillManager
    
    // 状态管理器
    statusMgr *StatusManager
    
    // 仇恨系统
    aggroSystem *AggroSystem
}
```

### 4. 监控和调试

#### 性能监控

```go
// 性能监控
type PerformanceMonitor struct {
    counters map[string]atomic.Int64
    timers   map[string]*Timer
}

func (pm *PerformanceMonitor) Count(name string) {
    pm.counters[name].Add(1)
}

func (pm *PerformanceMonitor) Time(name string) *Timer {
    timer := NewTimer()
    pm.timers[name] = timer
    return timer
}

// 性能统计
type PerformanceStats struct {
    Requests    int64
    Errors      int64
    AvgLatency  time.Duration
    P95Latency  time.Duration
    P99Latency  time.Duration
}

func (pm *PerformanceMonitor) GetStats() *PerformanceStats {
    return &PerformanceStats{
        Requests:    pm.counters["requests"].Load(),
        Errors:      pm.counters["errors"].Load(),
        AvgLatency:  pm.calculateAvgLatency(),
        P95Latency:  pm.calculateP95Latency(),
        P99Latency:  pm.calculateP99Latency(),
    }
}
```

#### 日志监控

```go
// 日志监控
type LogMonitor struct {
    errorChan    chan *LogEntry
    warningChan  chan *LogEntry
    stats        map[string]int64
}

func (lm *LogMonitor) Monitor(logger *zLog.Logger) {
    // 订阅日志事件
    logger.Subscribe(func(entry *LogEntry) {
        switch entry.Level {
        case zap.ErrorLevel:
            lm.errorChan <- entry
        case zap.WarnLevel:
            lm.warningChan <- entry
        }
    })

    // 处理错误日志
    go func() {
        for entry := <-lm.errorChan {
            lm.stats["errors"]++
            // 发送告警
            lm.sendAlert(entry)
        }
    }()
}
```

---

## 📖 参考资源

### 官方文档

- [Go官方文档](https://golang.org/doc/)
- [zap日志文档](https://pkg.go.dev/go.uber.org/zap)
- [zEngine项目文档](https://github.com/pzqf/zEngine)

### 学习资源

- **网络编程**：
  - 《Go网络编程》
  - [Go网络编程实战](https://github.com/astaxie/building-web-applications-with-Go)

- **并发编程**：
  - 《Go并发编程实战》
  - [Go并发模式](https://golang.org/doc/effective_go.html#concurrency)

- **架构设计**：
  - 《设计模式》
  - 《领域驱动设计》
  - 《企业应用架构模式》

### 相关项目

- **游戏引擎**：
  - [zGameServer](https://github.com/pzqf/zGameServer) - 基于zEngine的游戏服务器框架
  - [zUtil](https://github.com/pzqf/zUtil) - 工具库

- **网络库**：
  - [zNet](https://github.com/pzqf/zEngine/tree/master/zNet) - 网络模块
  - [zEvent](https://github.com/pzqf/zEngine/tree/master/zEvent) - 事件模块

- **工具库**：
  - [zLog](https://github.com/pzqf/zEngine/tree/master/zLog) - 日志模块
  - [zActor](https://github.com/pzqf/zEngine/tree/master/zActor) - Actor模块

---

## 🤝 贡献指南

### 如何贡献

1. **Fork项目**
2. **创建分支**
3. **提交更改**
4. **推送分支**
5. **提交Pull Request**

### 代码规范

- 遵循Go代码规范
- 使用gofmt格式化代码
- 添加适当的注释
- 编写单元测试

### 提交规范

- 清晰的提交信息
- 参考conventional commits
- 关联相关Issue

---

## 📄 许可证

本项目采用MIT许可证。详情请参考LICENSE文件。

---

## 📞 联系方式

如有问题或建议，欢迎通过以下方式联系：

- **Issue**：https://github.com/pzqf/zEngine/issues
- **邮件**：pzqf@example.com
- **Gitee**：https://gitee.com/pzqf/zEngine

---

**zEngine** - 简单高效的服务器引擎

---

## 📚 高级特性

### 1. TypedMap 使用说明

zEngine 现在全面使用 zUtil/zMap 提供的 TypedMap，实现类型安全的并发 Map。

#### TypedMap 优势

1. **类型安全** - 编译时检查类型，避免运行时panic
2. **无需类型断言** - 直接使用类型化的值
3. **性能提升** - 避免了反射和类型转换的开销
4. **代码更清晰** - 类型明确，可读性强

#### 使用示例

```go
// 创建类型安全的Map
m := zMap.NewTypedMap[string, int]()

// 存储元素（类型安全）
m.Store("key1", 123)

// 获取元素（类型安全）
value, exists := m.Load("key1")
// value 是 int 类型，可以直接使用

// 遍历元素
m.Range(func(key string, value int) bool {
    fmt.Println(key, value)
    return true
})

// 获取长度
length := m.Len()

// 清空Map
m.Clear()
```

#### 在网络服务器中的使用

```go
type TcpServer struct {
    // 使用 TypedMap 实现类型安全
    clientSessionMap *zMap.TypedMap[uint64, *TcpServerSession]
}

// 查找会话（类型安全）
func (svr *TcpServer) GetSession(sid uint64) *TcpServerSession {
    if client, ok := svr.clientSessionMap.Load(sid); ok {
        return client
    }
    return nil
}

// 获取所有会话
func (svr *TcpServer) GetAllSession() []*TcpServerSession {
    var sessionList []*TcpServerSession
    svr.clientSessionMap.Range(func(sid uint64, value *TcpServerSession) bool {
        sessionList = append(sessionList, value)
        return true
    })
    return sessionList
}
```

#### 完整接口列表

```go
// 核心接口
Load(key K) (V, bool)              // 获取元素
Store(key K, value V)              // 存储元素
Delete(key K)                      // 删除元素
Len() int64                        // 获取元素数量
Range(f func(key K, value V) bool) // 遍历元素
Clear()                            // 清空Map
LoadOrStore(key K, value V) (V, bool)   // 加载或存储
LoadAndDelete(key K) (V, bool)           // 加载并删除
CompareAndDelete(key K, oldValue V) bool // 比较并删除
CompareAndSwap(key K, oldValue V, newValue V) bool // 比较并替换
```

#### 迁移指南

如果您正在从旧的 Map 迁移到 TypedMap：

1. **修改声明**
```go
// 旧版本
clientSessionMap *zMap.Map

// 新版本
clientSessionMap *zMap.TypedMap[uint64, *TcpServerSession]
```

2. **修改初始化**
```go
// 旧版本
clientSessionMap: zMap.NewMap(),

// 新版本
clientSessionMap: zMap.NewTypedMap[uint64, *TcpServerSession](),
```

3. **修改方法调用**
```go
// 旧版本
client, ok := svr.clientSessionMap.Get(sid)
if ok {
    return client.(*TcpServerSession)
}

// 新版本（更简洁、更安全）
client, ok := svr.clientSessionMap.Load(sid)
if ok {
    return client
}
```

4. **修改 Range 回调**
```go
// 旧版本
svr.clientSessionMap.Range(func(key, value interface{}) bool {
    session := value.(*TcpServerSession)
    // ...
    return true
})

// 新版本
svr.clientSessionMap.Range(func(sid uint64, value *TcpServerSession) bool {
    // ...
    return true
})
```

### 2. 加密功能说明

zEngine 的 zNet 模块现在提供了完整的加密支持，包括 ECDH 密钥交换和 AES-GCM 加密。

#### 加密原理

1. **ECDH 密钥交换**
   - 基于椭圆曲线密码学
   - 服务器和客户端各自生成密钥对
   - 交换公钥并计算共享密钥
   - 无需传输实际的加密密钥

2. **AES-GCM 加密**
   - 认证加密模式，同时提供保密性和完整性
   - 自动处理初始化向量 (IV) 和认证标签
   - 高效的加密解密性能

#### 加密流程

```
┌─────────────────────┐                     ┌─────────────────────┐
│       服务器         │                     │       客户端         │
└─────────┬───────────┘                     └─────────┬───────────┘
          │                                           │
          │  1. 生成 ECDH 密钥对                       │  1. 生成 ECDH 密钥对
          │                                           │
          │  2. 发送公钥给客户端                        │  2. 发送公钥给服务器
          │ ─────────────────────────────────────────> │
          │ <───────────────────────────────────────── │
          │                                           │
          │  3. 计算共享密钥                           │  3. 计算共享密钥
          │    sharedKey = privKey * clientPubKey      │    sharedKey = privKey * serverPubKey
          │                                           │
          │  4. 派生 AES 密钥                          │  4. 派生 AES 密钥
          │                                           │
          └─────────────────────┐                     └─────────────────────┐
                                │                                           │
                                ▼                                           ▼
                        ┌─────────────────────┐                     ┌─────────────────────┐
                        │     AES-GCM 加密     │                     │     AES-GCM 加密     │
                        └─────────────────────┘                     └─────────────────────┘
```

#### 安全优势

1. **密钥安全** - 密钥通过 ECDH 协商，无需明文传输
2. **数据完整性** - AES-GCM 提供认证，防止数据篡改
3. **前向保密** - 每次连接生成新的密钥对
4. **高效性能** - AES-GCM 比传统的 AES-CBC 更高效

#### 加密配置

加密功能默认启用，无需额外配置。如果需要禁用加密（仅用于测试环境），可以通过以下方式：

```go
// 创建TCP服务器时禁用加密
server := zNet.NewTcpServer(config,
    zNet.WithLogger(logger),
    zNet.WithEncryptionDisabled(), // 禁用加密（仅测试环境）
)
```

### 3. 多协议客户端支持

zEngine 现在提供了统一的客户端 API，支持 TCP、UDP 和 WebSocket 协议。

#### 客户端特性

1. **统一的 API** - 所有客户端实现相同的接口
2. **自动重连** - 支持网络中断后的自动重连
3. **加密支持** - 所有协议都支持 ECDH + AES-GCM 加密
4. **消息分发** - 支持注册多个消息处理器

#### 客户端创建示例

```go
// TCP客户端
 tcpClient := zNet.NewTcpClient(
    zNet.WithClientLogger(logger),
    zNet.WithClientAutoReconnect(true),
 )

// UDP客户端
 udpClient := zNet.NewUdpClient(
    zNet.WithClientLogger(logger),
 )

// WebSocket客户端
 wsClient := zNet.NewWebSocketClient(
    zNet.WithClientLogger(logger),
 )
```

#### 客户端使用示例

```go
// 连接到服务器
 if err := client.ConnectToServer(address); err != nil {
    panic(err)
 }

// 注册消息处理器
 client.RegisterDispatcher(func(session interface{}, packet *zNet.NetPacket) error {
    // 处理接收到的消息
    return nil
 }, 100)

// 发送消息
 packet := &zNet.NetPacket{
    Cmd:  100,
    Data: []byte("Hello Server!"),
 }
 if err := client.Send(packet); err != nil {
    panic(err)
 }

// 关闭连接
 defer client.Close()
```

---