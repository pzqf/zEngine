package zActor

import (
	"sync"
	"sync/atomic"

	"go.uber.org/zap"

	"github.com/pzqf/zEngine/zLog"
)

// Runner 是一个「单写者闭包串行器」：独占一条 goroutine，把投递进来的闭包命令串行执行，
// 从根上消除对其所保护状态的并发访问（无需在被保护状态上再加锁）。
//
// 它与 BaseActor 互补，面向 BaseActor 不擅长的一类场景——「每种操作一个闭包、且常需同步
// 拿返回值、并伴随固定节拍的帧更新」（典型：游戏地图/房间——攻击要返回伤害、移动要返回成败、
// tick 要合帧）。BaseActor 是类型化消息 + 监督 + fire-and-forget；Runner 提供 BaseActor
// 缺的三块能力：
//
//   - Do：同步「请求-响应」——投递并等待执行完成，返回值靠闭包捕获（BaseActor 只有异步投递）。
//   - PostTick：合帧投递——队列满则丢弃本次（latest-wins），避免慢帧堆积拖垮时序/内存。
//   - 每命令 panic 隔离——一条命令 panic 只被 recover 掉、不影响该 goroutine 继续处理后续命令
//     （BaseActor 的 recover 在 run() 层，一次 panic 触发整个 actor 重启）。
//
// 用法：NewRunner → Start → Do/Post/PostTick → Stop。Do 与 PostTick 投递到同一条命令队列，
// 因此「帧更新」与「网络命令」在该 Runner 内天然串行，无需额外同步。
//
// ⚠ 绝不能在 Runner 的 goroutine 内部再调用自己的 Do（自投递死锁）——内部逻辑直接调用即可。
type Runner struct {
	id       int64
	cmdCh    chan func()
	stopCh   chan struct{}
	stopOnce sync.Once
	started  atomic.Bool
	logger   *zap.Logger
}

// NewRunner 创建一个 Runner。queueSize<=0 时用默认 1024。需调用 Start 才开始处理命令。
func NewRunner(id int64, queueSize int) *Runner {
	if queueSize <= 0 {
		queueSize = 1024
	}
	return &Runner{
		id:     id,
		cmdCh:  make(chan func(), queueSize),
		stopCh: make(chan struct{}),
		logger: zLog.GetLogger(),
	}
}

// Start 启动处理 goroutine（幂等：重复调用无副作用）。
func (r *Runner) Start() {
	if r.started.Swap(true) {
		return
	}
	go r.run()
}

// IsRunning 报告是否已 Start 且未 Stop。
func (r *Runner) IsRunning() bool {
	if !r.started.Load() {
		return false
	}
	select {
	case <-r.stopCh:
		return false
	default:
		return true
	}
}

func (r *Runner) run() {
	for {
		select {
		case <-r.stopCh:
			return
		case fn := <-r.cmdCh:
			r.safeExec(fn)
		}
	}
}

// safeExec 执行单条命令并兜底 panic，使一条坏命令不崩掉整条 goroutine。
func (r *Runner) safeExec(fn func()) {
	defer func() {
		if rec := recover(); rec != nil {
			r.logger.Error("runner command panic recovered",
				zap.Int64("runner_id", r.id), zap.Any("panic", rec))
		}
	}()
	fn()
}

// Do 把 fn 投递到 Runner 的 goroutine 上「同步执行并等待完成」（请求-响应语义，供调用方拿返回值）。
// 若尚未 Start 或已 Stop，则就地执行以保证调用方不永久阻塞、语义不变。
// ⚠ 绝不能在 Runner goroutine 内部调用 Do（自投递死锁）。
func (r *Runner) Do(fn func()) {
	if !r.started.Load() {
		r.safeExec(fn)
		return
	}
	done := make(chan struct{})
	wrapped := func() {
		defer close(done)
		fn()
	}
	select {
	case r.cmdCh <- wrapped:
		<-done
	case <-r.stopCh:
		// 正在停止：尽力就地执行，保证调用方不永久阻塞。
		r.safeExec(fn)
	}
}

// Post 异步投递命令：入队后立即返回，不等待执行。队列满时阻塞直到有空位或已停止。
// 返回是否成功入队（停止时返回 false）。
func (r *Runner) Post(fn func()) bool {
	select {
	case r.cmdCh <- fn:
		return true
	case <-r.stopCh:
		return false
	}
}

// PostTick 合帧投递：队列满则丢弃本次（latest-wins），永不阻塞。用于固定节拍的帧更新——
// 上一帧尚未处理完时丢弃当前帧，避免慢帧堆积。
func (r *Runner) PostTick(fn func()) {
	select {
	case r.cmdCh <- fn:
	default:
		// 队列满：丢弃本帧，下一拍再来。
	}
}

// Stop 停止 Runner（幂等）。只关闭 stopCh 作为退出信号，不关闭 cmdCh——关闭 cmdCh 会与并发的
// Post/Do 形成 send-on-closed panic。未消费的命令随停止一并丢弃并由 GC 回收。
func (r *Runner) Stop() {
	r.stopOnce.Do(func() {
		close(r.stopCh)
	})
}
