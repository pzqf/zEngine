# zEngine

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## 项目概述

zEngine 是一个轻量级**分布式游戏服务器引擎框架**，采用 Go 语言开发，提供网络通信、日志、Actor 并发模型、服务管理、分布式锁等核心模块，适用于游戏服务器及高并发后端应用。

### 设计理念

- **模块化**：各模块独立，可选择性使用
- **高性能**：基于 Go 并发特性，无锁设计优先
- **易扩展**：清晰的接口设计，便于扩展新功能
- **类型安全**：利用 Go 泛型提供类型安全的容器和工具

### 适用范围

**适合**：需要自建高并发长连接后端的场景——游戏服务器、实时对战/房间服务、IoT 网关、
需要自定义二进制协议 + 加密 + 防重放的 TCP/UDP/WebSocket 服务。你想要"骨架 + 并发/网络/
生命周期原语"，但自己掌控业务与协议。

**不适合**：普通 Web/CRUD（用 Echo/Gin 等更省事）；开箱即用的成品游戏服务器（zEngine 只是引擎，
不含玩法——需要成品参考见 [zMmoServer](https://github.com/pzqf/zMmoServer)）；对第三方依赖零容忍的场景
（zEngine 依赖 etcd/zap 等；若只要纯工具且零依赖，用下层的 [zUtil](https://github.com/pzqf/zUtil)）。

**定位**：`0.0.x`，接口仍可能调整；生产使用请自行压测与固化。架构全貌见 [ARCHITECTURE.md](ARCHITECTURE.md)。

## 技术栈

| 类别 | 技术 |
|------|------|
| **开发语言** | Go 1.25+ |
| **网络协议** | TCP / UDP / WebSocket / HTTP |
| **加密技术** | AES-GCM / ECDH (P-256) / 密钥轮换 |
| **日志框架** | zap + lumberjack |
| **并发模型** | Actor / Event-Driven |
| **服务发现** | etcd |
| **监控** | Prometheus |

## 项目结构

```
zEngine/
├── zNet/          # 网络层 - TCP/UDP/WebSocket/HTTP 服务器和客户端
├── zLog/          # 日志系统 - 基于 zap 的结构化日志，支持异步写入
├── zActor/        # Actor 并发模型 - 消息优先级、监督策略
├── zEvent/        # 事件总线 - 发布/订阅模式，同步/异步
├── zObject/       # 对象管理 - 泛型对象管理器、对象池
├── zService/      # 服务管理 - 拓扑排序初始化、依赖注入
├── zInject/       # 依赖注入容器 - 工厂/单例模式
├── zServer/       # 服务器框架 - 7种状态、生命周期管理、组件管理
├── zScript/       # 脚本系统 - Go AST 表达式求值、状态机
├── zNavMap/       # 导航寻路 - A* 算法
├── zDistributed/  # 分布式工具 - etcd 排他锁/共享锁
├── zMetrics/      # 监控指标 - Prometheus Counter/Gauge/Histogram
├── zSignal/       # 信号处理 - 优雅退出
└── zConfig/       # 配置管理 - ini/yaml 加载 + 热更新 watcher
```

模块如何组合成一个进程、数据如何流动、并发与安全模型 —— 见 [ARCHITECTURE.md](ARCHITECTURE.md)。

## 快速开始

```bash
go get github.com/pzqf/zEngine
```

### 30 行起一个 TCP 回显服务器

```go
package main

import "github.com/pzqf/zEngine/zNet"

const ProtoEcho zNet.ProtoIdType = 1

func main() {
    svr := zNet.NewTcpServer(&zNet.TcpConfig{
        ListenAddress:     "0.0.0.0:9000",
        MaxClientCount:    10000,
        ChanSize:          1024,
        HeartbeatDuration: 30,
        MaxPacketDataSize: 1 << 20,
        DisableEncryption: true, // 示例简化；生产建议开 ECDH+AES
    })
    // 收到任意包即原样回显
    svr.RegisterDispatcher(func(session zNet.Session, pkt *zNet.NetPacket) error {
        return session.Send(pkt.ProtoId, pkt.Data)
    })
    if err := svr.Start(); err != nil {
        panic(err)
    }
    select {} // 阻塞主 goroutine
}
```

### 对应的客户端

```go
cli := zNet.NewTcpClient(&zNet.TcpClientConfig{
    ServerAddr: "127.0.0.1", ServerPort: 9000,
    HeartbeatDuration: 30, MaxPacketDataSize: 1 << 20,
    DisableEncryption: true, ChanSize: 100,
})
cli.RegisterDispatcher(func(s zNet.Session, pkt *zNet.NetPacket) error {
    fmt.Printf("收到回显: proto=%d data=%s\n", pkt.ProtoId, pkt.Data)
    return nil
})
if err := cli.Connect(); err != nil { panic(err) }
cli.Send(ProtoEcho, []byte("hello zEngine"))
```

### 用 zServer 托管完整生命周期

真实服务器把网络、服务、状态机交给 `zServer.BaseServer` 统一管理：

```go
type GameServer struct {
    *zServer.BaseServer
    tcp *zNet.TcpServer
}

func (s *GameServer) OnBeforeStart() error {
    s.tcp = zNet.NewTcpServer(cfg)
    s.tcp.RegisterDispatcher(s.dispatch) // 按 ProtoId 分发；有状态逻辑投递给 Actor
    return s.Initialize()                 // Starting → Initializing
}
func (s *GameServer) OnAfterStart() error {
    if err := s.tcp.Start(); err != nil { return err }
    if err := s.Ready(); err != nil { return err }
    return s.Healthy()                    // Ready → Healthy
}
func (s *GameServer) OnBeforeStop() { /* 停监听 → 排空 → 关连接（优雅关闭） */ }

func main() {
    s := &GameServer{}
    s.BaseServer = zServer.NewBaseServer(
        zServer.ServerType("game"), "101", "GameServer", "0.0.1", s)
    if err := s.Run(); err != nil { // 依次驱动 OnBeforeStart → OnAfterStart → 等待退出 → OnBeforeStop
        panic(err)
    }
}
```

更完整的落地用例（四层分布式 MMORPG 服务端）见 [zMmoServer](https://github.com/pzqf/zMmoServer)。

## 核心模块

### zNet - 网络模块

zNet 是核心网络模块，支持 TCP/UDP/WebSocket/HTTP 四种协议，专为游戏服务器高并发场景设计。

**主要特性**：
- TCP 服务器/客户端（支持自动重连）
- UDP 服务器/客户端
- WebSocket 服务器/客户端
- HTTP 服务器
- DDoS 防护（连接限速、包频率限速、流量限速、IP 黑名单）- 无锁设计
- ECDH 密钥交换（P-256 曲线）
- 密钥轮换管理器
- 序列号校验（防重放攻击）
- Snappy 压缩
- 工作池模式

**关键类型**：
- `NetPacket` - 网络数据包（36字节头：ProtoId + Version + DataSize + IsCompressed + Sequence + Timestamp + KeyID）
- `TcpServer` / `TcpClient` - TCP 服务器/客户端
- `DDoSProtection` - DDoS 防护（无锁设计，基于 zMap.TypedMap + atomic）

### zLog - 日志系统

基于 zap + lumberjack 的结构化日志系统。

**主要特性**：
- 多级别日志（Debug/Info/Warn/Error/DPanic/Panic/Fatal）
- 控制台/文件双输出，支持级别分离
- 异步写入（AsyncWriter，可配置缓冲区大小和刷新间隔）
- 日志采样（减少高频日志量）
- 文件轮转（大小/天数/备份数限制）
- 多日志器管理（LoggerManager）
- 支持 `{ServerID}` 占位符

**关键类型**：
- `Config` - 日志配置（Level/Console/ConsoleLevel/Filename/MaxSize/MaxDays/MaxBackups/Compress/ShowCaller/Stacktrace/Sampling/Async/AsyncBufferSize/AsyncFlushInterval）

### zActor - Actor 并发模型

基于消息传递的 Actor 并发模型，天然串行化消息处理，避免锁竞争。

**主要特性**：
- 双通道设计（普通消息 + 高优先级消息）
- 监督策略（Restart/Stop/Escalate）
- 生命周期钩子（OnStart/OnStop/OnRestart）
- 消息优先级（High/Normal/Low）

**关键类型**：
- `BaseActor` - 基础 Actor 实现
- `SupervisorConfig` - 监督配置（策略、最大重启次数、重启窗口、退避时间）

### zServer - 服务器框架

提供完整的服务器生命周期管理和状态机。

**7种服务器状态**：
`Starting` → `Initializing` → `Ready` → `Healthy` → `Draining` → `Maintenance` → `Stopped`

**主要特性**：
- 状态机（合法转换校验）
- 组件注册/获取
- 状态变化监听器
- 状态报告
- 优雅关闭

### zObject - 对象管理

**关键类型**：
- `TypedObjectManager[ID, T]` - 泛型对象管理器（基于分片 Map）
- `GenericPool` - 通用对象池（基于 channel）

### zService - 服务管理

**主要特性**：
- 拓扑排序初始化/关闭（自动处理循环依赖检测）
- 依赖注入（RegisterDependency/RegisterSingleton/ResolveDependency）
- 服务注册器（ServiceRegistry + ServiceFactory）

### zDistributed - 分布式锁

基于 etcd 的分布式锁，支持排他锁和共享锁。

**关键类型**：
- `EtcdLock` - etcd 分布式锁实现
- `LockManager` - 锁管理器
- `LockWithAutoRenew` - 带自动续期的加锁

### zEvent - 事件总线

发布/订阅模式的事件总线，支持同步和异步事件处理。

**注意**：`Unsubscribe` 方法当前未实现（Go 函数值无法直接比较）。

### zScript - 脚本系统

基于 Go AST 的表达式求值与状态机脚本引擎。

**说明**：`RegisterScriptFunc` 对重复注册采用 `panic`——这是**启动期 fail-fast**（同名脚本函数
重复绑定属编程错误，与 stdlib `http.Handle` 对重复 pattern 的处理同理）。全局表已加 `sync.RWMutex`
并发保护，调试输出已改用日志。

### zNavMap - 导航寻路

基于 A* 算法的导航寻路，支持网格地图和可攀爬高度差。

### zMetrics - 监控指标

基于 Prometheus 的指标管理器，支持 Counter/Gauge/Histogram 三种指标类型，含网络指标（连接数/延迟/流量/错误率）。

**集成**：zNet 已提供上报接口 `NetworkMetricsRecorder`，用 `zNet.WithServerMetrics(recorder)`
注入即可（`zMetrics.NetworkMetrics` 满足该接口）——连接数/延迟/流量/错误率交由 zNet 自动采集。

### zInject - 依赖注入

轻量级依赖注入容器，支持 Factory 和 Singleton 两种依赖类型。

### zConfig - 配置管理

基于 ini/yaml 的配置加载，支持文件监听热更新（watcher）。

## 依赖

| 依赖 | 版本 | 用途 |
|------|------|------|
| `github.com/pzqf/zUtil` | 本地 replace | 工具库（zMap/zCrypto/zConcurrency） |
| `go.uber.org/zap` | v1.27.0 | 高性能日志 |
| `gopkg.in/natefinish/lumberjack.v2` | v2.2.1 | 日志轮转 |
| `go.etcd.io/etcd/client/v3` | v3.6.9 | etcd 客户端 |
| `github.com/gorilla/websocket` | v1.5.3 | WebSocket |
| `github.com/golang/snappy` | v1.0.0 | Snappy 压缩 |
| `github.com/prometheus/client_golang` | v1.23.2 | Prometheus 指标 |

## 已知问题与待优化

| 优先级 | 问题 | 说明 |
|--------|------|------|
| 低（可选） | zScript.RegisterScriptFunc 重复注册 `panic` | 属启动期 fail-fast，非运行时隐患；若要改为返回 error 会波及全部调用方，按需权衡 |

> **已解决**（2026-07 成熟化改造，逐条经代码核实）：
> - `zScript` 全局 `map` 非线程安全 → 已加 `sync.RWMutex`（`funcListMu` / `ScriptHolder.mu`）
> - `zScript` 调试输出 `fmt.Println` → 已改用日志
> - `zNet option.go` 用 reflect 做类型分支 → 已改为函数式 Options（`WithMaxClientCount` 等），去 reflect
> - `zNet TcpServerSession.onClose` 被调用两次 → 已用 `sync.Once`（`closeOnce`）幂等收口
> - `zMetrics` 与 `zNet` 未集成 → 已提供 `NetworkMetricsRecorder` 接口 + `WithServerMetrics` 注入
> - `zEvent.Unsubscribe` 已实现（Subscribe 返回 SubscriptionID，按句柄退订）
> - `zServer 与 zService 职责重叠`已解耦（zService 改为独立可选模块）

## 安装

```bash
go get github.com/pzqf/zEngine
```

## 许可证

MIT License

---

*最后更新: 2026-07-25*
