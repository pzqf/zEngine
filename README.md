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
└── zSystem/       # ECS 系统 - SystemManager + StateManager
```

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

**已知问题**：
- 全局 `map` 非线程安全
- 重复注册使用 `panic`（应改为返回错误）
- 调试输出使用 `fmt.Println`（应使用日志系统）

### zNavMap - 导航寻路

基于 A* 算法的导航寻路，支持网格地图和可攀爬高度差。

### zMetrics - 监控指标

基于 Prometheus 的指标管理器，支持 Counter/Gauge/Histogram 三种指标类型，含网络指标（连接数/延迟/流量/错误率）。

**注意**：zMetrics 与 zNet 当前未集成，网络指标需要手动采集。

### zInject - 依赖注入

轻量级依赖注入容器，支持 Factory 和 Singleton 两种依赖类型。

### zSystem - ECS 系统

系统管理器 + 状态管理器，支持系统注册/初始化/更新/关闭的完整生命周期。

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
| 高 | zScript 全局 map 非线程安全 | `scriptFileList` 和 `funcList` 需加并发保护 |
| 高 | zScript.RegisterScriptFunc 使用 panic | 应改为返回 error |
| 中 | zNet option.go 使用 reflect 做类型分支 | 性能差且脆弱，建议接口断言 |
| 中 | zNet TcpServerSession.onClose 被调用两次 | defer 和方法末尾重复调用 |
| 中 | zMetrics 与 zNet 未集成 | 网络指标需手动采集 |
| 低 | zEvent.Unsubscribe 未实现 | Go 函数值无法直接比较 |
| 低 | zServer 与 zService 职责重叠 | 两者都管理服务生命周期 |

## 安装

```bash
go get github.com/pzqf/zEngine
```

## 许可证

MIT License

---

*最后更新: 2026-04-14*
