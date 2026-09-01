# zEngine 架构说明

**文档版本**：0.0.16

本文讲清楚 zEngine 各模块**如何组合成一个可运行的服务器**、数据如何在其中流动、以及背后的
并发与安全模型。想快速上手看 [README.md](README.md) 的「快速开始」；想理解设计全貌看这里。
判断能力是否应进入引擎、公共 API 如何兼容演进，见 [层次边界与演进原则.md](docs/层次边界与演进原则.md)。

## 1. 定位

zEngine 是一套**分布式游戏服务器引擎的基础设施层**——它提供搭一个高并发 TCP/UDP/WS 服务器
所需的骨架（网络、生命周期、并发模型、日志、指标、配置、分布式锁），但**不含任何游戏玩法**。
玩法/业务由上层项目实现（参考 [zMmoServer](https://github.com/pzqf/zMmoServer)，它用 zEngine
构建了四进程 realm MMO 服务端骨架，并已有部分真实调用链；生产成熟度仍以其审计基线和执行计划为准）。

它的下面只有一层 [zUtil](https://github.com/pzqf/zUtil)——零第三方依赖的通用工具库（并发安全容器、
加密、缓存等）。依赖方向单向向下，无循环：

```
   上层业务（如 zMmoServer 的 Global/Gateway/Game/Map）
              │  使用
   ┌──────────▼───────────────────────────────────┐
   │                  zEngine                       │
   │  传输 zNet · 运行时 zActor/zObject/zEvent      │
   │  原语 zAoi/zInstance/zNavMap/zScript           │
   │  框架 zServer/zService/zInject                 │
   │  设施 zLog/zMetrics/zHealth/zConfig/zSignal     │
   │  协调 zDistributed/zConsistency                 │
   └──────────┬───────────────────────────────────┘
              │  依赖
   ┌──────────▼───────────┐
   │  zUtil（零第三方依赖） │  zMap/zCrypto/zConcurrency/...
   └──────────────────────┘
```

## 2. 分层与模块职责

zEngine 的 18 个模块按角色分五层。上层可自由挑选，模块间尽量解耦（如 zService 与 zServer
已解耦为独立可选模块）。

以下表格描述模块责任和当前 API 存在性，不表示每个模块都已接入 zMmoServer 生产路径，也不表示其在
饱和、部分失败、重启和滚动升级下已经闭环。当前限制会在对应行直接标出。

### 传输层
| 模块 | 职责 |
|------|------|
| **zNet** | 引擎核心。TCP/UDP/WebSocket/HTTP 的服务器与客户端；`NetPacket` 定长包头；TCP/UDP/WS 共用分配前 header/wire-size 校验、端点级 byte order、解密/解压后大小限制、有界发送、context/deadline 和三种 admission；DDoS 防护、ECDH + AES-GCM、序列号、Snappy、工作池。业务 ACK 与协议版本仍由上层/VER-01 定义。 |

### 运行时（承载业务逻辑的并发原语）
| 模块 | 职责 |
|------|------|
| **zActor** | 两种串行原语：`Runner` context API 提供四态、可取消 admission、停止排空、执行上下文重入/环检测和 panic 隔离；旧无 context 包装仅作执行域外兼容。`BaseActor` 提供稳定非阻塞 mailbox admission、正常停止排空、priority 有界公平、独立 latest-wins、监督/abandoned 统计和锁外 generation hook；无父树的 Escalate 启动即拒绝。 |
| **zObject** | 泛型对象管理 `TypedObjectManager[ID,T]`（分片 Map 支撑）+ 通用对象池 `GenericPool`。 |
| **zEvent** | 进程内发布/订阅；默认构造仅有订阅表、零 goroutine，checked API 返回 closed/no-executor/full 等 admission 结果。异步 executor 显式注入且由调用方持有，handler 按单事件订阅顺序在锁外执行并隔离 panic；旧自有 pool 构造仅作兼容。 |
| **zInstance** | 泛型实例池：checked 创建/销毁、容量预留、亲和选择、显式销毁和空置回收；creating/active/destroying + generation/revision 保护两阶段状态，build/Occupancy/onEvict/Close 均在池锁外执行，`reserved` 阻止显式销毁越过在途进入。 |

### 服务框架（把零件组织成一个进程）
| 模块 | 职责 |
|------|------|
| **zServer** | 服务器状态机、进程 context、组件注册和生命周期钩子入口；当前启动失败回滚、并发 Stop、signal 释放和四进程 `OnAfterStop` 调用尚未闭环。 |
| **zService** | 可选服务依赖 DAG 和初始化/关闭骨架；当前缺依赖、初始化回滚和关闭错误聚合仍待加固，`ServiceManager` 没有 zMmoServer 生产接线。 |
| **zInject** | 轻量依赖注入容器，Factory / Singleton 两种依赖类型。 |

### 基础设施
| 模块 | 职责 |
|------|------|
| **zLog** | 基于 zap + lumberjack 的结构化日志；异步写入、采样、文件轮转、多日志器、`{ServerID}` 占位符。 |
| **zMetrics** | Prometheus Counter/Gauge/Histogram checked schema registry（name/type/help/const labels/buckets）与运行时/网络采集；失败注册不缓存，MemoryMonitor handler 同步且 Stop 可立即唤醒。HTTP listener/mux 的进程适配在 zCommon，由四角色显式持有。 |
| **zHealth** | 可插拔健康检查和报告模型；Memory/Goroutine 有检查逻辑，Disk/Time 仍固定返回 Healthy，不能直接作为生产 readiness。 |
| **zConfig** | ini/yaml 配置加载和 watcher API；生产主要使用文件解析，ConfigWatcher 尚无 zMmoServer 生产构造。 |
| **zSignal** | 进程信号处理，驱动优雅退出。 |
| **zDistributed** | 基于官方 etcd Session/Mutex 的 fenced 排他锁：Acquire 返回只读 LockHandle，lease/owner key 丢失会失效；Release compare-delete，create revision 提供单调 fence，Validate/GuardedTxn 可原子校验受保护 etcd 写入。retry 可取消，shared 明确 unsupported；旧 Lock/Unlock 仅作 advisory 兼容。当前无 zMmoServer 生产调用。 |
| **zConsistency** | context/error 感知的 Memory/SQL Outbox/Inbox：严格三阶段/三态、显式终结、重试/dead-letter 和 fail-closed；`SQLExecutor` 允许 store 绑定 DB/Tx，业务提交点和事务 ownership 仍在上层。真实 MySQL E3 与 zMmoServer grant 生产调用已通过。`InMemoryTransactionCoordinator` 只是一致加锁、锁外回调的 experimental 单进程原语，当前无生产构造，不承诺 durable/recovery 或传输 exactly-once；旧 `TransactionManager` 仅作兼容。 |

### 可复用游戏领域原语（可选）
| 模块 | 职责 |
|------|------|
| **zAoi** | 2D 网格空间索引和可见性 delta 原语；checked 构造、尾格覆盖和实体权威索引拒绝非法尺寸/重复/不存在，移动按旧/新可见集合产生双向 leave/enter，并只向真实观察者产生 move。客户端协议和 watcher 会话路由仍由上层负责；上层双 realm 同实例、双向互见和真实 watcher 移动 E4 已通过。 |
| **zNavMap** | 实验性的 A* 网格寻路；当前无 zMmoServer 生产调用，预算、取消和路径契约待真实用例验证。 |
| **zScript** | 实验性的 Go AST 表达式与状态机运行时；当前无 zMmoServer 生产调用，预算、取消和版本语义未闭合。 |

## 3. 一个服务器是怎么搭起来的

典型进程按下面的方式组合 `zServer.BaseServer`、zNet 和业务钩子。该图是当前成功路径的使用方式和
目标关闭顺序，不是“启动失败回滚、排空和重复 Stop 已全部验证”的成熟度证明：

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

当前 `Start` 失败不会完整逆序回滚已启动资源，`Stop` 也尚未形成并发幂等契约；四进程实现的
`OnAfterStop` 不在 `LifecycleHooks` 接口中，因此不会由 zServer 调用。完整启动事务、真实 Draining 和
资源释放属于后续 `LIF-01/DRN-01`，不能由七态名称反推已经完成。

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
  │                                    ├─ 按端点 byte order 解析 36B header
  │                                    ├─ 分配前校验 ProtoId/DataSize/wire limit/压缩标志
  │                                    ├─ 读取 body → 解密预检 → Snappy DecodedLen 预检
  │                                    ├─ 解密/解压后二次断言 → 校验序列号（防重放）
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
- **线格式归端点所有**：server/client 配置 `WireByteOrderLittle` 或 `WireByteOrderBig`，session 构造时
  快照为不可变 `PacketCodec`。旧全局 byte order 只给兼容 API 和未显式配置的新端点提供默认值。
- **坏帧策略明确**：TCP 不可信长度会破坏后续流边界，因此拒绝并断连接；UDP/WebSocket 保留消息边界，
  因此丢当前消息并继续。三种传输都在分配/切片前拒绝负数、超限和非法头字段。
- **解码资源独立受限**：`MaxWirePacketSize` 和 `MaxDecodedPacketSize` 分别限制线上 payload 与各解码阶段；
  已读完且边界完整的 payload 若解密、解压或 decoded-size 校验失败，六类收包路径都丢当前包并继续会话。
- **发送有界**：旧 `session.Send` 是默认 5 秒的 `ReliableCommand` 兼容包装；新代码用
  `ContextSession.SendWithOptions` 选择 `LatestFrame`、`BestEffortEvent` 或 `ReliableCommand`。前两类立即
  返回或覆盖，可靠命令只等到 admission deadline，不代表远端业务已提交。
- **worker admission 可见**：`WorkerPool.Submit` 满队列立即返回稳定错误，需等待容量时用
  `SubmitWithContext`；Stop 原子拒绝新任务并排空已接纳任务。TCP receive 不再忽略提交错误。
- **路由生命周期明确**：`RequestRouter` 拒绝重复 requestID，Close 会失败全部 pending；构造期使用
  `MessageRouter.TryRegisterHandler`，装配完成后 Seal，handler 始终在锁外执行。
- **有状态逻辑走 Actor**：dispatcher 拿到消息后投递给对应实体的 Actor（`SendMessage`），
  由 Actor 串行处理，避免对共享状态加锁。调用方必须处理 full/stopping/stopped 等 admission error；
  返回 nil 表示消息所有权已转移，正常 Stop 会排空。回包按消息语义选择有界发送等级。
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
| 前置帧校验 | 在 body 分配/切片前验证 ProtoId、DataSize、wire-size 和压缩标志；错误日志限频并累计指标 |
| 解码后上限 | AES-GCM 解密前预检明文长度；Snappy `DecodedLen` 在分配前校验并在解压后二次断言 |

## 7. 与分布式设施的关系

- **服务发现**：zEngine 依赖 etcd 客户端；具体的注册/发现封装通常放在业务侧（如 zMmoServer 的
  `discovery` 包统一 etcd 接入），zEngine 侧提供 `zDistributed` 分布式锁作协调原语。
- **ownership/fencing**：`FencedLock` 返回 owner/lease/fence handle，旧 lease 失效后 compare-delete 和
  guarded transaction 都不能越过新 owner。它只提供 game-agnostic 机制；resource key、业务 owner 状态机、
  epoch 持久化及非 etcd 存储如何拒绝旧 fence 仍由上层 `OWN-01` 定义。当前 zMmoServer 无生产接线。
- **一致性存储**：V2 Outbox 把持久入队、传输完成和业务应用分开，调用者按自身 ACK 契约显式终结；
  V2 Inbox 把首次接受、处理中和已处理分开，存储失败不默认接受。内置 Memory/SQL 共享相同状态契约并已
  通过并发/重启测试；SQL store 可绑定调用者的 DB/Tx executor 参与本地事务。旧 API 与
  `OutboxMessage.TargetMapID` 尚未删除。zEngine 不定义 destination、ProtoId、业务 ACK 或资产事务，
  commit/rollback、schema、事务边界和恢复策略仍由上层装配并逐调用链验证。
- **可观测**：zMetrics 提供 checked Prometheus collector registry；通过 `WithServerMetrics` 显式注入后，zNet 自动记录连接、
  流量、总解码错误、解码错误分类、发送队列容量/深度、三类拒绝或丢弃以及 send/worker admission
  等待。zMmoServer 的 zCommon 适配使用每实例私有 mux/listener 和可取消 Stop 暴露 `/metrics`；不在该端口
  冒充 readiness。四角色已显式启动/停止该资源，完整启动事务和真实健康仍由 LIF-01/HLT-01 验收。

## 8. 扩展点一览

| 想做什么 | 用哪里 |
|----------|--------|
| 加一种协议消息 | 定义 ProtoId，在 dispatcher / `MessageRouter` 注册 handler |
| 加一类有状态实体 | 定义 Actor（嵌 `BaseActor`），dispatcher 投递消息 |
| 加一个后台服务 | 实现服务并用 zService 声明依赖，拓扑初始化 |
| 加一个进程级组件 | `BaseServer.RegisterComponent` |
| 加指标 | zMetrics `Register*Checked` 注册 Counter/Gauge/Histogram，并处理 schema 错误 |
| 加分布式协调 | zDistributed `FencedLock.Acquire` + `LockHandle`；etcd 写入用 `GuardedTxn`，业务 ownership 状态和存储 fence 留上层 |

---

*配套阅读：[README.md](README.md)（模块清单 + 快速开始）。上层完整用例见
[zMmoServer](https://github.com/pzqf/zMmoServer)。*

## 文档变更记录

| 日期 | 版本 | 变更 |
|------|------|------|
| 2026-09-01 | 0.0.16 | 完成 CON-03：记录进程内 experimental 事务协调器的准确命名、并发状态机、旧 API 兼容及无 durable/生产接线边界。 |
| 2026-09-01 | 0.0.15 | 完成 CON-02：记录 SQL store 的 DB/Tx executor 事务参与边界及 zMmoServer grant 生产验证；引擎不拥有业务事务、ACK 或恢复策略。 |
| 2026-09-01 | 0.0.14 | 完成 CON-01：记录 V2 context/error、Outbox 严格三阶段/显式终结、Inbox 三态/fail-closed 和真实 MySQL 并发/重启 E3；事务原子性、上层 ACK 和 experimental 2PC 边界不变。 |
| 2026-09-01 | 0.0.13 | 完成 LOCK-01：记录 fenced handle、compare-delete、释放竞态、lease/owner 失效、guarded etcd 写入和真实重连 E3；业务 owner 状态机仍留 zMmoServer。 |
| 2026-09-01 | 0.0.12 | 完成 MET-01：记录 checked collector schema、无幽灵缓存、MemoryMonitor 立即停止，以及上层四角色私有 metrics listener/mux ownership；readiness 与完整启动事务边界不变。 |
| 2026-09-01 | 0.0.11 | 完成 EVT-01：记录 EventBus 零资源默认构造、显式 executor ownership、checked admission、锁外顺序回调、全局重建和 GameServer 对象 emitter 移除；进程根停机装配仍归 LIF-01。 |
| 2026-09-01 | 0.0.10 | 完成 INS-01：记录 zInstance 两阶段状态、锁外上层回调、checked 错误、reservation 销毁门禁和 MapServer 上层回归；分线/副本/跨服策略仍留应用层。 |
| 2026-09-01 | 0.0.9 | 记录 AOI-01 上层双 realm 同实例、双向互见和真实 watcher 移动 E4 已通过；不扩大 zAoi 的引擎职责。 |
| 2026-08-31 | 0.0.8 | 记录 AOI-01 checked 构造、权威实体归属和完整 watcher/target delta；上层 race 已通过，真实移动 E4 待依赖恢复。 |
| 2026-08-31 | 0.0.7 | 记录 ACT-02 BaseActor admission、停止排空、公平优先级、latest-wins、监督与 generation hook 契约，以及 Player 上层处理边界。 |
| 2026-08-31 | 0.0.6 | 收紧 ACT-01 成熟度表述：严格重入保证属于 context API；无 context 旧包装仅作执行域外兼容并等待单独退役。 |
| 2026-08-31 | 0.0.5 | 记录 ACT-01 Runner 四态、context/error、停止排空、重入检测及 MapServer 单写者回归；保留 BaseActor 在 ACT-02 的边界。 |
| 2026-08-31 | 0.0.4 | 记录 NET-03 的有界发送、三种 admission、WorkerPool drain-on-Stop、router 生命周期和背压指标边界。 |
| 2026-08-31 | 0.0.3 | 记录 NET-02 的 wire/decoded 两级上限、解码预检/二次断言、六类收包失败语义和分类指标。 |
| 2026-08-31 | 0.0.2 | 记录 NET-01 的共享前置帧校验、端点级 byte order、TCP 与 datagram 错误策略，并保留 NET-02/03 未完成边界。 |
| 2026-08-31 | 0.0.1 | 区分模块职责/API 存在与生产成熟度，校正生命周期、健康检查、分布式协调、一致性和实验包的当前边界。 |
