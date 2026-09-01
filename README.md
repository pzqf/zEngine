# zEngine

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**文档版本**：0.0.14

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

**定位**：`0.0.x`，接口仍可能调整；生产使用请自行压测与固化。架构全貌见 [ARCHITECTURE.md](ARCHITECTURE.md)，
引擎与上层游戏业务的归属规则见 [层次边界与演进原则.md](docs/层次边界与演进原则.md)。

**能力口径**：下文“支持”表示包或 API 当前存在，不等于 zMmoServer 已生产接线，也不等于饱和、部分失败、
重启和滚动升级已经通过验证。当前成熟度和加固任务以 zMmoServer 的架构审计基线与执行计划为准。

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
├── zAoi/          # AOI 原语 - 2D 网格空间索引与可见性事件
├── zEvent/        # 事件总线 - 发布/订阅模式，同步/异步
├── zObject/       # 对象管理 - 泛型对象管理器、对象池
├── zInstance/     # 实例池 - checked 创建/销毁、容量预留、亲和选择、空置回收
├── zService/      # 服务管理 - 拓扑排序初始化、依赖注入
├── zInject/       # 依赖注入容器 - 工厂/单例模式
├── zServer/       # 服务器框架 - 7种状态、生命周期管理、组件管理
├── zScript/       # 脚本系统 - Go AST 表达式求值、状态机
├── zNavMap/       # 导航寻路 - A* 算法
├── zDistributed/  # 分布式工具 - etcd lease、排他锁与 fencing 演进
├── zConsistency/  # Outbox/Inbox 存储与实验性事务协调原语
├── zHealth/       # liveness/readiness/diagnostics 检查原语
├── zMetrics/      # 监控指标 - checked schema registry + 运行时/网络采集
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
        MaxWirePacketSize:    1 << 20,
        MaxDecodedPacketSize: 1 << 20,
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
    HeartbeatDuration: 30,
    MaxWirePacketSize: 1 << 20, MaxDecodedPacketSize: 1 << 20,
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
- TCP/UDP/WebSocket 共用前置包头校验：业务 ProtoId、保留控制 ID、非负 DataSize、wire-size 上限、
  压缩标志和消息边界在 body 分配/切片前验证
- wire/decoded 两级独立资源上限：Snappy 先用 `DecodedLen` 预检再分配并二次断言；AES-GCM 在解密前
  预检明文长度；解密或解压失败只丢当前完整 payload，不把旧数据继续交给 dispatcher
- 端点级字节序：`TcpConfig/UdpConfig/WebSocketConfig/TcpClientConfig.ByteOrder` 支持 `little`/`big`；
  UDP/WebSocket 客户端在连接前使用 `SetWireByteOrder`；已构造 session 保存不可变快照，不受后续
  全局默认值变化影响
- TCP/UDP/WebSocket server session 使用有界发送队列；client 直写使用 context-aware 串行 gate；旧
  `Send` 是默认 5 秒超时的兼容包装
- `LatestFrame`、`BestEffortEvent`、`ReliableCommand` 三种 admission 机制；只定义排队/丢弃/超时，
  不替上层定义业务 ACK、持久化或提交成功
- WorkerPool 非阻塞 `Submit`、可取消 `SubmitWithContext` 和并发安全的 drain-on-Stop；
  `RequestRouter` 支持重复 ID 拒绝和 `Close(cause)`，`MessageRouter` 支持严格注册与 seal

**关键类型**：
- `NetPacket` - 网络数据包（36字节头：ProtoId + Version + DataSize + IsCompressed + Sequence + Timestamp + KeyID）
- `PacketCodec` / `WireByteOrder` - 端点级包头编解码和共享 header/frame 校验
- `MaxWirePacketSize` / `MaxDecodedPacketSize` - 线包与解码后 payload 的独立上限，默认均为 1 MiB
- `ContextSession` / `SendOptions` / `DeliveryClass` - 有 deadline 的发送与三种队列 admission 机制
- `RequestRouter` / `MessageRouter` - 有关闭语义的请求关联器和可冻结消息路由表
- `TcpServer` / `TcpClient` - TCP 服务器/客户端
- `DDoSProtection` - DDoS 防护（无锁设计，基于 zMap.TypedMap + atomic）

`SetByteOrder/GetByteOrder` 为兼容入口：只决定旧 `NetPacket.Marshal/UnmarshalHead` 和后续新建端点的默认值，
不会修改已有端点。非法 TCP 头会关闭流连接；UDP/WebSocket 非法消息丢当前帧并保留会话，重复错误日志限频，
服务端累计 decoding-error metrics，并可选细分 wire oversize、decoded oversize、decrypt 和 decompress；
WebSocket 还在库级读取阶段应用 wire-size limit。

`MaxPacketDataSize` 仍作为 `MaxWirePacketSize` 的旧配置键保留；显式新字段优先，零值和负值采用有界默认值。
UDP/WebSocket client 可在连接前用 `SetPacketSizeLimits` 配置两级上限。服务端发送队列已固定容量、拒绝、
等待和关闭语义，真实 TCP 停读测试覆盖 writer 阻塞后的 deadline admission 与关闭；这仍不等于应用层
可靠投递，`ReliableCommand` 成功只表示已进入发送队列或完成有界直写。

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
- `BaseActor.SendMessage/SendPriority` 非阻塞返回稳定 admission 错误；正常停止关闭 admission 后排空
- High/Normal/Low 三条有界队列采用有界高优先级 burst；latest-wins 使用独立单槽
- Restart/Stop 监督失败可观察；无父监督树的 `Escalate` 仅保留源码名称，`Start` 明确返回 unsupported
- 生命周期钩子（OnStart/OnStop/OnRestart）在 mutex 外按 generation 快照执行，重入返回稳定状态结果
- `ActorStats` 暴露队列深度、接纳/拒绝、latest 替换、panic/restart、abandoned 和 hook failure
- `Runner.DoContext` 同步单写者调用：未启动/停止中/已停止返回稳定错误，队列等待可取消
- `Runner.PostContext/TryPost` 分别提供可取消等待和非阻塞 admission
- `Runner.StopContext` 关闭 admission 后排空已接纳命令，并等待独占 goroutine 退出
- 执行上下文记录 Runner 调用链，同 Runner 重入和跨 Runner 环返回稳定错误；命令 panic 隔离并可观察

**关键类型**：
- `BaseActor` - 类型化消息 Actor；旧公开 `ActorMsgChan`/`PriorityActorMessage` 仅作 deprecated 兼容
- `Runner` - 闭包式单写者执行器；旧 `Do/Post/PostTick/Stop` 暂留 deprecated 兼容包装

严格的重入/环检测只适用于显式传递 `execCtx` 的新 API。旧包装没有 context 参数，只允许从所有
Runner 执行域之外调用；`Do/Post/Stop` 在 Runner 回调内使用仍可能发生旧式阻塞或死锁。zMmoServer
生产调用已迁移到新 API，旧包装将在独立兼容清理任务中删除。
- `SupervisorConfig` - 监督配置（策略、最大重启次数、重启窗口、退避时间）

`Runner` 的严格重入/环检测依赖显式执行上下文；`BaseActor` 不提供同步跨 Actor 调用链，而提供非阻塞
mailbox admission、停止排空和监督结果。zMmoServer 的唯一生产承重类型是 GameServer `Player`：
`PlayerManager.RouteMessage` 原样返回 admission error，资产 grant 拒绝时释放 Inbox 以允许重投，AOI/
队伍/交易等尽力通知只记录拒绝。跨进程 ACK 和资产事务原子性不属于 Actor 契约。

### zAoi - 网格可见性原语

- `NewGridManagerChecked` 拒绝非法地图/网格尺寸，非整倍数地图使用尾格覆盖；旧构造暂留兼容
- GridManager 内部持有实体唯一位置；重复 Add 和不存在的 Move/Remove 返回稳定错误
- Remove 不再信任调用方坐标；`MoveEntityTo` 按权威旧位置计算旧可见、新可见和交集
- 旧/新差集产生双向 leave/enter；交集只产生 `Watcher=真实邻居, Target=移动者` 的 move
- 索引变更先提交，listener 后在内部锁外接收值化事件

zMmoServer 的 MapServer 已迁到 checked/权威位置 API，Map watcher 路由和 Game Player Actor 推送通过
重复 `-race`。客户端空间可见性仍只由 MapServer AOI 产生；双 realm 同实例、双向互见以及一个客户端
移动后另一客户端收到真实 watcher move 的跨进程 E4 已通过。该结果不外推为 owner、持久事务、
kill/restart 恢复或滚动兼容闭环。

### zServer - 服务器框架

提供服务器状态机、生命周期钩子和组件注册骨架。当前启动失败逆序回滚、并发/重复 Stop、进程资源统一
关闭和四进程排空尚未闭环，不能把状态到达 `Healthy/Stopped` 等同于资源已经可接流量或已经全部释放。

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

### zDistributed - 分布式锁与协调原语

基于 etcd 官方 `concurrency.Session/Mutex` 的排他 ownership 原语。`FencedLock.Acquire` 返回只读
`LockHandle`（resource key、唯一 owner key、LeaseID、以 create revision 表示的 FencingToken）；lease 或
owner key 丢失会使 handle 失效。`Release` 以 owner/fence/lease compare-delete，旧 handle 不能删除新 owner；
`Validate` 和 `GuardedTxn` 可把 ownership compare 放进同一 etcd 事务。获取 context 只约束获取过程，
不偷走已取得 lease 的生命周期；retry 有确定次数和可取消等待。

旧 `DistributedLock.Lock/Unlock` 保留兼容，但因不能把 fencing token 传给受保护写入，只能视为 advisory。
`LockTypeShared` 明确返回 unsupported，watchdog 不再静默重获新 fence。上述机制已通过真实 etcd 的 lease
丢失、旧 owner 攻击、client 重连/自然过期和并发 `-race`；zMmoServer 生产调用仍为零，resource key、
owner 状态机和业务存储 fence 接线属于 `OWN-01`，不能从包级 E3 外推为业务 ownership 闭环。

**关键类型**：
- `FencedLock` / `EtcdLock` - 显式 handle 的 fenced 排他锁
- `LockHandle` - owner/lease/fencing 身份与失效通知
- `GuardedTxn` - 与 ownership compare 原子组合的 etcd 写入
- `LockManager` - 锁管理器
- `DistributedLock` / `LockWithAutoRenew` - deprecated/advisory 兼容入口

### zConsistency - 可靠消息存储与实验性协调

`OutboxStoreV2` / `InboxStoreV2` 为持久操作提供 context、error 和稳定的状态结果。Outbox 严格区分
`Enqueued -> Transported -> Applied`：写入 socket 只能标记 `Transported`，收到业务 ACK 后才能标记
`Applied`；调用者必须以实际终态显式 `Complete`，重试耗尽可进入 dead-letter。Inbox 的 `Acquire` 明确
返回 `Accepted/InProgress/Processed`，只有 `Accepted` 拥有本次处理权；SQL 故障返回错误并 fail closed。

内置 Memory 旧 API 与 V2 API 共用同一转换锁，输入和返回的 payload 使用防御性复制；旧接口及适配器
仍保留兼容，但第三方旧 Outbox 实现无法持久表达中间状态，严格可靠路径应直接实现 V2。Memory/SQL 的
状态、重启保持和并发转换已通过重复 `-race`，SQL 路径还通过真实 MySQL 的无 `SKIP` E3。

这些机制只承诺“至少一次投递 + 幂等效果”，不承诺跨进程 exactly-once。Inbox 与上层业务变更尚未
共享本地事务，zMmoServer 仍有默认 Memory 路径，Map 侧已处理完成但响应发送失败时也没有响应缓存；
业务 ACK、路由、资产事务、恢复状态机和 commit point 继续留在上层 `CON-02`。单进程 2PC 没有持久
日志或跨进程 fencing，仍是 experimental，归 `CON-03` 决策。

### zHealth - 健康检查原语

当前 MemoryChecker、GoroutineChecker 有实际检查逻辑；DiskChecker 和 TimeChecker 只是占位实现并固定
返回 Healthy，GCChecker 还会主动触发 `debug.FreeOSMemory`。两套 checker 模型、空检查集合和生命周期
语义尚未统一，且当前没有包级测试，不能把这些类型直接当作生产 readiness。

### zEvent - 事件总线

`NewEventBus` 只创建订阅表，不启动 goroutine；`SubscribeChecked/PublishChecked/
PublishSyncChecked/UnsubscribeChecked` 返回稳定的输入、关闭、无 executor 和队列满错误。异步处理必须用
`NewEventBusWithExecutor` 显式借用 owner 管理的 executor，EventBus Close 不越权停止它；旧
`NewEventBusWithPool` 仅作自有 worker 的 deprecated 兼容入口。每个异步事件只提交一个任务，快照中的
handler 按订阅顺序、在总线锁外执行，panic 逐个隔离。

zMmoServer 已删除零订阅的 GameObject/Player 对象级 emitter；当前只有 Map/Chat 全局发布，生产订阅为零，
因此默认全局 bus 不需要 executor。将来出现真实异步订阅时，由进程根注入并在 `LIF-01` 统一关闭；zEvent
不承诺跨进程可靠投递。

### zScript - 脚本系统

基于 Go AST 的表达式求值与状态机脚本实验能力；当前没有 zMmoServer 生产调用，执行预算、取消和版本/
热更语义尚未形成稳定契约。

**说明**：`RegisterScriptFunc` 对重复注册采用 `panic`——这是**启动期 fail-fast**（同名脚本函数
重复绑定属编程错误，与 stdlib `http.Handle` 对重复 pattern 的处理同理）。全局表已加 `sync.RWMutex`
并发保护，调试输出已改用日志。

### zNavMap - 导航寻路

基于 A* 算法的导航寻路实验能力；当前没有 zMmoServer 生产调用，搜索预算、取消和路径结果契约仍待
真实 AI/移动用例验证。

### zMetrics - 监控指标

基于 Prometheus 的指标管理器，支持 Counter/Gauge/Histogram 三种指标类型，含网络指标（连接数/延迟/流量/
错误率）以及发送队列容量/深度、三类拒绝或丢弃、发送/worker admission 等待时间和 worker 拒绝计数。

`Register*Checked` 按 name、类型、help、const labels 和有效 histogram buckets 校验 schema；同 schema
返回已注册 collector，冲突或 Prometheus registry 拒绝会返回稳定错误，失败 collector 不进入缓存。
旧 `Register*` API 暂留兼容并在失败时返回 nil。`ResetNetworkMetrics` 只重置内存网络 recorder；旧
`ResetAll` 仅作同语义 deprecated 包装，不再用名称暗示会重置 Prometheus collectors。MemoryMonitor 的
handler 注册/快照已同步，Stop 通过独立停止通道立即唤醒并等待采集 goroutine。

**集成**：zNet 已提供上报接口 `NetworkMetricsRecorder`，用 `zNet.WithServerMetrics(recorder)`
注入即可（`zMetrics.NetworkMetrics` 满足该接口）——连接数/延迟/流量/错误率交由 zNet 自动采集。

### zInject - 依赖注入

轻量级依赖注入容器，支持 Factory 和 Singleton 两种依赖类型。

### zConfig - 配置管理

当前生产调用主要使用 ini/yaml 文件解析。ConfigWatcher/provider API 存在，但没有 zMmoServer 生产构造，
初始加载失败、重连快照、并发 Start/Stop 和回调所有权尚未形成已验证的热更新闭环。

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

## 文档变更记录

| 日期 | 版本 | 变更 |
|------|------|------|
| 2026-09-01 | 0.0.14 | 完成 CON-01：增加 context/error V2 API、严格 Outbox 三阶段与显式终结、Inbox 三态/fail-closed、旧 API 兼容；Memory/SQL 并发与真实 MySQL 重启 E3 通过，事务原子性仍归 CON-02。 |
| 2026-09-01 | 0.0.13 | 完成 LOCK-01 引擎机制：官方 etcd session/mutex、只读 LockHandle、compare-delete、单调 fencing、guarded transaction、确定性 retry 和 lease/reconnect E3；zMmoServer 业务 ownership 仍待 OWN-01。 |
| 2026-09-01 | 0.0.12 | 完成 MET-01 引擎机制：checked collector schema/registry 冲突返回错误且不缓存幽灵 collector，MemoryMonitor handler 同步并可立即停止；业务指标和 readiness 仍留上层。 |
| 2026-09-01 | 0.0.11 | 完成 EVT-01：默认 EventBus 零 goroutine，checked admission、显式 executor ownership、锁外顺序 handler、原子关闭和全局重建通过重复 race；GameObject 对象级 emitter 已移除。 |
| 2026-09-01 | 0.0.10 | 完成 INS-01：实例 build/Occupancy/onEvict/Close 全部锁外执行，checked API 可传播构建/admission 错误，reservation 阻止在途销毁；MapServer 真实分线/副本/跨服调用链已迁移并通过重复 race。 |
| 2026-09-01 | 0.0.9 | 记录 AOI-01 双 realm 同实例、双向互见和真实 watcher 移动 E4 已通过；保留 ownership、事务和恢复边界。 |
| 2026-08-31 | 0.0.8 | 记录 AOI-01 checked 构造、实体权威索引、完整双向 delta、锁外 listener 和 Map/Game 回归；真实移动 E4 待依赖恢复。 |
| 2026-08-31 | 0.0.7 | 完成 ACT-02 文档：记录 BaseActor 有界 admission、停止排空、优先级公平、latest-wins、监督降级、generation hook 和 Player 生产调用边界。 |
| 2026-08-31 | 0.0.6 | 明确旧 Runner 包装不具备执行上下文重入保护，只允许从执行域外调用；生产路径继续以 context API 为严格契约。 |
| 2026-08-31 | 0.0.5 | 完成 ACT-01 文档：记录 Runner 四态、context/error、停止排空、重入检测、panic 结果和 MapServer MAP-2 迁移边界。 |
| 2026-08-31 | 0.0.4 | 完成 NET-03 文档：记录有界发送、WorkerPool admission/Stop、router 生命周期、三种投递等级、背压指标及可靠性边界。 |
| 2026-08-31 | 0.0.3 | 完成 NET-02 文档：增加 wire/decoded 两级上限、预分配校验、失败语义、详细指标和旧配置兼容说明。 |
| 2026-08-31 | 0.0.2 | 完成 NET-01 文档：记录 TCP/UDP/WebSocket 前置帧校验、默认 wire-size 上限、端点级 byte order、错误策略和 NET-02/03 边界。 |
| 2026-08-31 | 0.0.1 | 区分 API 存在与生产成熟度，校正 zServer、zDistributed、zConsistency、zHealth、zEvent、zNavMap、zScript 和 zConfig 的当前限制。 |

*最后更新: 2026-09-01*
