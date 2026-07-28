package zNet

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pzqf/zEngine/zLog"
	"github.com/pzqf/zUtil/zMap"
	"go.uber.org/zap"
)

// RequestRouter 是一个通用的「异步请求-响应关联器」：把「无状态发送 + 异步到达的响应」缝合成
// 一次同步的请求-响应调用（Call）。零业务语义——任何「发出去一个带 requestID 的请求、稍后由另一
// 条路径把响应按 requestID 送回来」的场景都能用（跨进程 RPC / 跨服消息 / 设备回执 等）。
//
// 用法：
//
//	reqID := rr.NextRequestID()
//	data, err := rr.SendRequest(ctx, reqID, func() error { return conn.Send(encode(reqID, req)) })
//	// 另一条 goroutine 收到响应时：rr.CompleteRequest(reqID, respData, nil)
//
// 需由外部周期调用 Cleanup() 清理超时未回的挂起请求（否则超时的 pending 项会积累）。
//
// 从 zMmoServer/crossserver 下沉——此前每个上层各自实现这套关联逻辑，现统一为引擎能力。
type RequestRouter struct {
	pending   *zMap.TypedMap[uint64, *PendingResponse]
	nextReqID atomic.Uint64
	timeout   time.Duration
	cleanupMu sync.Mutex
}

// PendingResponse 一个挂起中的请求：响应到达时经 ch 送回，deadline 供 Cleanup 判超时。
type PendingResponse struct {
	ch       chan *ResponseResult
	deadline time.Time
}

// ResponseResult 一次请求的结果（成功携带 Data，失败携带 Error）。
type ResponseResult struct {
	Data  []byte
	Error error
}

// NewRequestRouter 创建关联器。timeout<=0 时默认 10s。
func NewRequestRouter(timeout time.Duration) *RequestRouter {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &RequestRouter{
		pending: zMap.NewTypedMap[uint64, *PendingResponse](),
		timeout: timeout,
	}
}

// NextRequestID 返回一个进程内单调递增的请求 ID。
func (rr *RequestRouter) NextRequestID() uint64 {
	return rr.nextReqID.Add(1)
}

// RegisterPending 登记一个挂起请求并返回接收响应的 channel（缓冲 1，单发一收）。
func (rr *RequestRouter) RegisterPending(requestID uint64) <-chan *ResponseResult {
	ch := make(chan *ResponseResult, 1)
	rr.pending.Store(requestID, &PendingResponse{
		ch:       ch,
		deadline: time.Now().Add(rr.timeout),
	})
	return ch
}

// CompleteRequest 把响应按 requestID 送回对应的挂起请求。返回是否命中一个挂起项（未命中=已超时/未知）。
func (rr *RequestRouter) CompleteRequest(requestID uint64, data []byte, err error) bool {
	pending, exists := rr.pending.LoadAndDelete(requestID)
	if !exists {
		return false
	}

	result := &ResponseResult{Data: data, Error: err}
	select {
	case pending.ch <- result:
	default:
	}
	return true
}

// SendRequest 注册挂起 → 调用 sendFn 发送 → 等待响应 / ctx 取消 / 超时。返回响应数据或错误。
func (rr *RequestRouter) SendRequest(ctx context.Context, requestID uint64, sendFn func() error) ([]byte, error) {
	respCh := rr.RegisterPending(requestID)

	if err := sendFn(); err != nil {
		rr.pending.Delete(requestID)
		return nil, fmt.Errorf("send request failed: %w", err)
	}

	select {
	case result := <-respCh:
		if result.Error != nil {
			return nil, result.Error
		}
		return result.Data, nil
	case <-ctx.Done():
		rr.pending.Delete(requestID)
		return nil, ctx.Err()
	case <-time.After(rr.timeout):
		rr.pending.Delete(requestID)
		return nil, fmt.Errorf("request %d timed out after %v", requestID, rr.timeout)
	}
}

// Cleanup 清理已超过 deadline 仍未回的挂起请求（给它们的等待方发一个 expired 错误）。须周期调用。
func (rr *RequestRouter) Cleanup() {
	rr.cleanupMu.Lock()
	defer rr.cleanupMu.Unlock()

	now := time.Now()
	var expired []uint64
	rr.pending.Range(func(reqID uint64, p *PendingResponse) bool {
		if now.After(p.deadline) {
			expired = append(expired, reqID)
		}
		return true
	})

	for _, reqID := range expired {
		if p, exists := rr.pending.LoadAndDelete(reqID); exists {
			select {
			case p.ch <- &ResponseResult{Error: fmt.Errorf("request expired")}:
			default:
			}
		}
	}

	if len(expired) > 0 {
		zLog.Debug("Cleaned up expired pending requests",
			zap.Int("count", len(expired)))
	}
}

// PendingCount 当前挂起请求数。
func (rr *RequestRouter) PendingCount() int {
	return int(rr.pending.Len())
}
