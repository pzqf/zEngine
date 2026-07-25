package zLog

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap/zapcore"
)

// AsyncWriter 异步日志写入器
// 将日志写入操作异步化，提高日志记录性能
type AsyncWriter struct {
	writer        zapcore.WriteSyncer // 底层写入器
	buffer        chan []byte         // 写入缓冲区
	stopChan      chan struct{}       // 停止信号通道
	flushInterval time.Duration       // 刷新间隔
	wg            sync.WaitGroup      // 等待组
	mu            sync.Mutex          // 互斥锁（串行化对底层 writer 的写）
	closed        atomic.Bool         // 是否已关闭（OPT-14：Write 热路径无锁检查）
	dropped       atomic.Int64        // OPT-14：因缓冲满/已关闭而丢弃的日志条数
}

// NewAsyncWriter 创建一个新的异步写入器
// 参数:
//   - writer: 底层写入器
//   - bufferSize: 缓冲区大小
//   - flushInterval: 刷新间隔
//
// 返回:
//   - *AsyncWriter: 异步写入器实例
func NewAsyncWriter(writer zapcore.WriteSyncer, bufferSize int, flushInterval time.Duration) *AsyncWriter {
	aw := &AsyncWriter{
		writer:        writer,
		buffer:        make(chan []byte, bufferSize),
		stopChan:      make(chan struct{}),
		flushInterval: flushInterval,
	}
	aw.wg.Add(1)
	go aw.run()
	return aw
}

// run 异步写入循环
// 在独立goroutine中运行，处理写入、定时刷新和停止信号
func (aw *AsyncWriter) run() {
	defer aw.wg.Done()
	ticker := time.NewTicker(aw.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case data := <-aw.buffer:
			aw.write(data)
		case <-ticker.C:
			aw.flushBuffer()
		case <-aw.stopChan:
			aw.flushBuffer()
			return
		}
	}
}

// write 写入数据到底层写入器
// 参数:
//   - data: 要写入的数据
func (aw *AsyncWriter) write(data []byte) {
	aw.mu.Lock()
	defer aw.mu.Unlock()
	_, _ = aw.writer.Write(data)
}

// flushBuffer 刷新缓冲区中所有数据
// 将缓冲区中所有待写入数据写入到底层写入器
func (aw *AsyncWriter) flushBuffer() {
	for {
		select {
		case data := <-aw.buffer:
			aw.write(data)
		default:
			return
		}
	}
}

// Write 实现 io.Writer 接口
// 将数据写入缓冲区，如果缓冲区满则直接写入
//
// 参数:
//   - p: 要写入的数据
//
// 返回:
//   - n: 写入的字节数
//   - error: 写入失败时返回错误
func (aw *AsyncWriter) Write(p []byte) (n int, err error) {
	// OPT-14: 关闭后拒收——run() 已退出，再往 buffer 发只会无人消费/静默丢失。计数并返回。
	if aw.closed.Load() {
		aw.dropped.Add(1)
		return len(p), nil
	}

	data := make([]byte, len(p))
	copy(data, p)

	select {
	case aw.buffer <- data:
		return len(p), nil
	default:
		// OPT-14: 缓冲满时不再同步直写——那样会插到仍排队在 buffer 里、尚未刷出的日志之前，
		// 造成日志乱序。改为丢弃并计数（异步日志过载下丢弃优于乱序或阻塞业务），Close 时汇报。
		aw.dropped.Add(1)
		return len(p), nil
	}
}

// Sync 实现 zapcore.WriteSyncer 接口
// 刷新所有缓冲区数据
//
// 返回:
//   - error: 同步失败时返回错误
func (aw *AsyncWriter) Sync() error {
	aw.Flush()
	return nil
}

// Flush 刷新缓冲区
// 将缓冲区中所有待写入数据写入到底层写入器
func (aw *AsyncWriter) Flush() {
	aw.flushBuffer()
}

// Close 关闭异步写入器
// 停止异步写入循环，刷新所有缓冲区，关闭底层写入器
//
// 返回:
//   - error: 关闭失败时返回错误
func (aw *AsyncWriter) Close() error {
	// OPT-14: 幂等关闭（CAS）。closed 置位后 Write 立即拒收，避免向无人消费的 buffer 发送。
	if !aw.closed.CompareAndSwap(false, true) {
		return nil
	}

	close(aw.stopChan)
	aw.wg.Wait()
	aw.flushBuffer()

	if d := aw.dropped.Load(); d > 0 {
		aw.write([]byte(fmt.Sprintf("[async-writer] dropped %d log messages (buffer full or closed)\n", d)))
	}
	return aw.writer.Sync()
}
