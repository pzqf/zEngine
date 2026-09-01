package zNet

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pzqf/zEngine/zLog"
	"go.uber.org/zap"
)

var (
	ErrDuplicateRequestID  = errors.New("request ID is already pending")
	ErrRequestRouterClosed = errors.New("request router is closed")
	ErrRequestExpired      = errors.New("request expired")
)

// RequestRouter 把异步响应按 requestID 关联回等待中的调用。它只提供通用关联和生命周期机制，
// 不定义 requestID 的业务生成规则或远端成功语义。
type RequestRouter struct {
	mu         sync.Mutex
	pending    map[uint64]*PendingResponse
	nextReqID  atomic.Uint64
	timeout    time.Duration
	closed     bool
	closeCause error
}

// PendingResponse 一个挂起中的请求。
type PendingResponse struct {
	ch       chan *ResponseResult
	deadline time.Time
}

// ResponseResult 一次请求的结果。
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
		pending: make(map[uint64]*PendingResponse),
		timeout: timeout,
	}
}

// NextRequestID 返回一个进程内单调递增的请求 ID。
func (rr *RequestRouter) NextRequestID() uint64 {
	return rr.nextReqID.Add(1)
}

// TryRegisterPending 严格登记挂起请求。重复 ID 不覆盖旧等待者；关闭后拒绝新请求。
func (rr *RequestRouter) TryRegisterPending(requestID uint64) (<-chan *ResponseResult, error) {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	if rr.closed {
		return nil, rr.closedError()
	}
	if _, exists := rr.pending[requestID]; exists {
		return nil, fmt.Errorf("%w: %d", ErrDuplicateRequestID, requestID)
	}
	pending := &PendingResponse{
		ch:       make(chan *ResponseResult, 1),
		deadline: time.Now().Add(rr.timeout),
	}
	rr.pending[requestID] = pending
	return pending.ch, nil
}

// RegisterPending 保留旧 API。新代码应使用 TryRegisterPending 取得明确注册错误。
// 发生错误时返回一个已装入错误结果的 channel，避免旧调用者永久等待。
// Deprecated: use TryRegisterPending.
func (rr *RequestRouter) RegisterPending(requestID uint64) <-chan *ResponseResult {
	ch, err := rr.TryRegisterPending(requestID)
	if err == nil {
		return ch
	}
	failed := make(chan *ResponseResult, 1)
	failed <- &ResponseResult{Error: err}
	return failed
}

// CompleteRequest 送达响应。返回 false 表示请求未知、已超时或 router 已关闭。
func (rr *RequestRouter) CompleteRequest(requestID uint64, data []byte, err error) bool {
	rr.mu.Lock()
	pending, exists := rr.pending[requestID]
	if exists {
		delete(rr.pending, requestID)
	}
	rr.mu.Unlock()
	if !exists {
		return false
	}
	pending.ch <- &ResponseResult{Data: data, Error: err}
	return true
}

// SendRequest 注册挂起、执行发送，再等待响应、context、router 关闭或内部超时。
func (rr *RequestRouter) SendRequest(ctx context.Context, requestID uint64, sendFn func() error) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	respCh, err := rr.TryRegisterPending(requestID)
	if err != nil {
		return nil, err
	}

	rr.mu.Lock()
	pending := rr.pending[requestID]
	rr.mu.Unlock()
	if sendFn == nil {
		rr.removePending(requestID, pending)
		return nil, errors.New("send request failed: nil send function")
	}
	if err := sendFn(); err != nil {
		rr.removePending(requestID, pending)
		return nil, fmt.Errorf("send request failed: %w", err)
	}

	timer := time.NewTimer(rr.timeout)
	defer timer.Stop()
	select {
	case result := <-respCh:
		if result.Error != nil {
			return nil, result.Error
		}
		return result.Data, nil
	case <-ctx.Done():
		rr.removePending(requestID, pending)
		return nil, ctx.Err()
	case <-timer.C:
		rr.removePending(requestID, pending)
		return nil, fmt.Errorf("request %d timed out after %v: %w", requestID, rr.timeout, ErrRequestExpired)
	}
}

func (rr *RequestRouter) removePending(requestID uint64, pending *PendingResponse) bool {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	current, exists := rr.pending[requestID]
	if !exists || current != pending {
		return false
	}
	delete(rr.pending, requestID)
	return true
}

// Cleanup 清理超过 deadline 的挂起请求。
func (rr *RequestRouter) Cleanup() {
	now := time.Now()
	var expired []*PendingResponse
	rr.mu.Lock()
	for requestID, pending := range rr.pending {
		if now.After(pending.deadline) {
			delete(rr.pending, requestID)
			expired = append(expired, pending)
		}
	}
	rr.mu.Unlock()

	for _, pending := range expired {
		pending.ch <- &ResponseResult{Error: ErrRequestExpired}
	}
	if len(expired) > 0 {
		zLog.Debug("Cleaned up expired pending requests", zap.Int("count", len(expired)))
	}
}

// Close 原子拒绝新请求，并用 cause 失败全部 pending。首次 cause 是稳定关闭原因。
func (rr *RequestRouter) Close(cause error) {
	if cause == nil {
		cause = ErrRequestRouterClosed
	}
	rr.mu.Lock()
	if rr.closed {
		rr.mu.Unlock()
		return
	}
	rr.closed = true
	rr.closeCause = cause
	pending := rr.pending
	rr.pending = make(map[uint64]*PendingResponse)
	rr.mu.Unlock()

	for _, request := range pending {
		request.ch <- &ResponseResult{Error: cause}
	}
}

func (rr *RequestRouter) closedError() error {
	if rr.closeCause == nil || errors.Is(rr.closeCause, ErrRequestRouterClosed) {
		return ErrRequestRouterClosed
	}
	return fmt.Errorf("%w: %w", ErrRequestRouterClosed, rr.closeCause)
}

// PendingCount 当前挂起请求数。
func (rr *RequestRouter) PendingCount() int {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	return len(rr.pending)
}
