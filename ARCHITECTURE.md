# zEngine 架构说明

本文讲清楚 zEngine 各模块**如何组合成一个可运行的服务器**、数据如何在其中流动、以及背后的
并发与安全模型。想快速上手看 [README.md](README.md) 的「快速开始」；想理解设计全貌看这里。

## 1. 定位

zEngine 是一套**分布式游戏服务器引擎的基础设施层**——它提供搭一个高并发 TCP/UDP/WS 服务器
所需的骨架（网络、生命周期、并发模型、日志、指标、配置、分布式锁），但**不含任何游戏玩法**。
玩法/业务由上层项目实现（参考 [zMmoServer](https://github.com/pzqf/zMmoServer)，它把 zEngine
当作引擎搭出了完整的 MMORPG 服务端）。

它的下面只有一层 [zUtil](https://github.com/pzqf/zUtil)——零第三方依赖的通用工具库（并发安全容器、
加密、缓存等）。依赖方向单向向下，无循环：

```
   上层业务（如 zMmoServer 的 Global/Gateway/Game/Map）
              │  使用
   ┌──────────▼───────────────────────────────────┐
   │                  zEngine                       │
   │  传输 zNet · 运行时 zActor/zObject/zEvent      │
   │  框架 zServer/zService/zInject                 │
   │  设施 zLog/zMetrics/zConfig/zSignal/zDistributed│
   │  领域工具 zNavMap/zScript                       │
   └──────────┬───────────────────────────────────┘
              │  依赖
   ┌──────────▼───────────┐
   │  zUtil（零第三方依赖） │  zMap/zCrypto/zConcurrency/...
   └──────────────────────┘
```

## 2. 分层与模块职责

zEngine 的 14 个模块按角色分四层。上层可自由挑选，模块间尽量解耦（如 zService 与 zServer
已解耦为独立可选模块）。

### 传输层
| 模块 | 职责 |
|------|------|
| **zNet** | 引擎核心。TCP/UDP/WebSocket/HTTP 的服务器与客户端；`NetPacket` 定长包头（ProtoId/Version/DataSize/IsCompressed/Sequence/Timestamp/KeyID）；DDoS 防护（无锁）；ECDH 密钥交换 + AES-GCM + 密钥轮换；序列号防重放；Snappy 压缩；工作池。 |

### 运行时（承载业务逻辑的并发原语）
| 模块 | 职责 |
|------|------|
| **zActor** | 消息传递 Actor 模型，串行化消息处理规避锁竞争；双通道（普通/高优先级）；监督策略（Restart/Stop/Escalate）+ 生命周期钩子。 |
| **zObject** | 泛型对象管理 `TypedObjectManager[ID,T]`（分片 Map 支撑）+ 通用对象池 `GenericPool`。 |
| **zEvent** | 发布/订阅事件总线，同步/异步；`Subscribe` 返回句柄，按句柄 `Unsubscribe`。 |

### 服务框架（把零件组织成一个进程）
| 模块 | 职责 |
|------|------|
| **zServer** | 服务器生命周期与状态机（7 态）。`BaseServer` + `LifecycleHooks`（`OnBeforeStart/OnAfterStart/OnBeforeStop`）；组件注册；优雅关闭。 |
| **zService** | 服务依赖管理：拓扑排序初始化/关闭（循环依赖检测）；依赖注入（Register/Resolve）。可独立使用。 |
| **zInject** | 轻量依赖注入容器，Factory / Singleton 两种依赖类型。 |

### 基础设施
| 模块 | 职责 |
|------|------|
| **zLog** | 基于 zap + lumberjack 的结构化日志；异步写入、采样、文件轮转、多日志器、`{ServerID}` 占位符。 |
| **zMetrics** | Prometheus Counter/Gauge/Histogram 指标管理器（含网络指标定义）。 |
| **zConfig** | ini/yaml 配置加载 + 文件监听热更新。 |
| **zSignal** | 进程信号处理，驱动优雅退出。 |
| **zDistributed** | 基于 etcd 的分布式锁（排他/共享）+ 自动续期 + 锁管理器。 |

### 领域工具（可选）
| 模块 | 职责 |
|------|------|
| **zNavMap** | A* 网格寻路，支持可攀爬高度差。 |
| **zScript** | 基于 Go AST 的表达式求值与状态机脚本引擎。 |

## 3. 一个服务器是怎么搭起来的

典型进程用 `zServer.BaseServer` 托管生命周期，在钩子里拉起 zNet、初始化各服务、推进状态机：

```
main()
 └─ NewBaseServer(type, id, name, version, hooks) → Run()
     ├─ OnBeforeStart()   // 建 zNet server、注册 handler、zService 拓扑初始化各依赖
     │      └─ Initialize()            // Starting → Initializing
     ├─ OnAfterStart()    // 启动监听、启动后台任务
     │      └─ Ready() → Healthy()     // Initializing → Ready → Healthy
     ├─ （运行中：对外提供服务，zSignal 等待退出信号）
     └─ OnBeforeStop()    // 反序优雅关闭：停监听 → 排空 → 关连接/服务
            //  Healthy → Draining → Stopped
```

**状态机契约**：框架只在钩子失败时兜底置 `Stopped`；**成功路径的状态推进由业务在钩子内自行驱动**
（`Initialize()/Ready()/Healthy()`），以保留灵活性（如在 `OnBeforeStart` 内初始化完再校验依赖）。

完整状态流：`Starting → Initializing → Ready → Healthy → Draining → Maintenance → Stopped`，
非法转换会被状态机拒绝。

## 4. 网络数据流

一条客户端消息的完整往返：

```
Client                                             Server
  │  cli.Send(protoId, data)                         │
  ├─ 组 NetPacket（定长头 + payload）                  │
  ├─ [可选] Snappy 压缩 → AES-GCM 加密 → 序列号        │
  │            ───────────  TCP  ───────────►         │
  │                                    解密/解压/校验序列号（防重放）
  │                                    ├─ 反序列化 NetPacket
  │                                    ├─ DDoS 门（连接/包频/流量/IP 黑名单）
  │                                    └─ dispatcher(HandlerFun)
  │                                          │  session, *NetPacket
  │                                          ├─ 按 pkt.ProtoId 路由（MessageRouter）
  │                                          ├─ [可选] 投递给某 Actor 串行处理
  │                                          └─ session.Send(protoId, data) 回包
  │            ◄───────────  TCP  ───────────         │
  └─ dispatcher 收回包                                 │
```

- **接入是回调式**：`server.RegisterDispatcher(func(session, *NetPacket) error)`。业务通常在
  dispatcher 里按 `ProtoId` 分发（`MessageRouter.RegisterHandler(protoId, h)`）。
- **有状态逻辑走 Actor**：dispatcher 拿到消息后投递给对应实体的 Actor（`SendMessage`），
  由 Actor 串行处理，避免对共享状态加锁。回包用 `session.Send`。
- **安全默认开**：ECDH(P-256) 协商会话密钥 → AES-GCM 加密 → 密钥轮换 → 序列号窗口防重放。
  示例/内网可 `DisableEncryption: true` 简化。

## 5. 并发模型

两条主线，配合使用：

1. **Actor 串行化**：每个有状态实体（玩家、地图……）由一个 Actor 独占一条消息队列，
   所有改动经消息串行执行——把"多 goroutine 抢一份状态"变成"单 goroutine 顺序处理"，
   从根上规避数据竞争（配合 `go test -race` 作回归护栏）。
2. **无锁数据结构**：跨实体的高频共享结构（连接表、DDoS 计数）用 zUtil 的分片 Map + atomic，
   避免全局锁成为瓶颈。

## 6. 安全模型（zNet）

| 机制 | 作用 |
|------|------|
| ECDH (P-256) 密钥交换 | 每连接协商独立会话密钥，不明文传密钥 |
| AES-GCM | 载荷加密 + 完整性校验 |
| 密钥轮换 | 长连接周期性换密钥，限制单密钥暴露面 |
| 序列号窗口 | 防重放：滑动窗口拒绝重复/过期序列号 |
| DDoS 防护 | 连接限速 / 包频限速 / 流量限速 / IP 黑名单，无锁实现 |

## 7. 与分布式设施的关系

- **服务发现**：zEngine 依赖 etcd 客户端；具体的注册/发现封装通常放在业务侧（如 zMmoServer 的
  `discovery` 包统一 etcd 接入），zEngine 侧提供 `zDistributed` 分布式锁作协调原语。
- **可观测**：zMetrics 暴露 Prometheus 指标；注意当前 zMetrics 与 zNet **未自动集成**，
  网络指标需业务手动采集上报（见 README「已知问题」）。

## 8. 扩展点一览

| 想做什么 | 用哪里 |
|----------|--------|
| 加一种协议消息 | 定义 ProtoId，在 dispatcher / `MessageRouter` 注册 handler |
| 加一类有状态实体 | 定义 Actor（嵌 `BaseActor`），dispatcher 投递消息 |
| 加一个后台服务 | 实现服务并用 zService 声明依赖，拓扑初始化 |
| 加一个进程级组件 | `BaseServer.RegisterComponent` |
| 加指标 | zMetrics 注册 Counter/Gauge/Histogram |
| 加分布式协调 | zDistributed 排他/共享锁 |

---

*配套阅读：[README.md](README.md)（模块清单 + 快速开始）。上层完整用例见
[zMmoServer](https://github.com/pzqf/zMmoServer)。*
