package zNet

import (
	"errors"
	"fmt"
	"sync"
)

var (
	ErrHandlerAlreadyRegistered = errors.New("message handler already registered")
	ErrMessageRouterSealed      = errors.New("message router is sealed")
	ErrNilMessageHandler        = errors.New("message handler is nil")
)

// MessageRouter 按 ProtoId 分派消息。构造阶段使用严格注册 API，装配完成后调用 Seal，运行期 Dispatch
// 只读稳定路由表。handler 与 fallback 始终在锁外执行。
type MessageRouter struct {
	mu       sync.RWMutex
	handlers map[int32]HandlerFun
	fallback HandlerFun
	sealed   bool
}

// NewMessageRouter 创建空路由表。
func NewMessageRouter() *MessageRouter {
	return &MessageRouter{handlers: make(map[int32]HandlerFun)}
}

// TryRegisterHandler 严格注册 handler。重复 ProtoId 不允许静默覆盖。
func (r *MessageRouter) TryRegisterHandler(protoID int32, handler HandlerFun) error {
	if handler == nil {
		return ErrNilMessageHandler
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sealed {
		return ErrMessageRouterSealed
	}
	if r.handlers == nil {
		r.handlers = make(map[int32]HandlerFun)
	}
	if _, exists := r.handlers[protoID]; exists {
		return fmt.Errorf("%w: %d", ErrHandlerAlreadyRegistered, protoID)
	}
	r.handlers[protoID] = handler
	return nil
}

// RegisterHandler 保留旧链式 API，在未 seal 时维持 last-wins 兼容语义。
// 新代码应使用 TryRegisterHandler 取得重复注册错误。
// Deprecated: use TryRegisterHandler.
func (r *MessageRouter) RegisterHandler(protoID int32, handler HandlerFun) *MessageRouter {
	if handler == nil {
		return r
	}
	r.mu.Lock()
	if !r.sealed {
		if r.handlers == nil {
			r.handlers = make(map[int32]HandlerFun)
		}
		r.handlers[protoID] = handler
	}
	r.mu.Unlock()
	return r
}

// TryUnregisterHandler 在构造阶段移除 handler。
func (r *MessageRouter) TryUnregisterHandler(protoID int32) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sealed {
		return ErrMessageRouterSealed
	}
	delete(r.handlers, protoID)
	return nil
}

// UnregisterHandler 保留旧 API。seal 后不再修改路由表。
// Deprecated: use TryUnregisterHandler.
func (r *MessageRouter) UnregisterHandler(protoID int32) {
	_ = r.TryUnregisterHandler(protoID)
}

// TrySetFallback 设置或清除 fallback。与 Dispatch 并发安全。
func (r *MessageRouter) TrySetFallback(handler HandlerFun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sealed {
		return ErrMessageRouterSealed
	}
	r.fallback = handler
	return nil
}

// SetFallback 保留旧链式 API。seal 后不再修改 fallback。
// Deprecated: use TrySetFallback.
func (r *MessageRouter) SetFallback(handler HandlerFun) *MessageRouter {
	_ = r.TrySetFallback(handler)
	return r
}

// Seal 冻结路由表。重复调用幂等。
func (r *MessageRouter) Seal() {
	r.mu.Lock()
	r.sealed = true
	r.mu.Unlock()
}

// IsSealed 返回路由表是否已冻结。
func (r *MessageRouter) IsSealed() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sealed
}

// HasHandler 判断是否已注册。
func (r *MessageRouter) HasHandler(protoID int32) bool {
	r.mu.RLock()
	_, exists := r.handlers[protoID]
	r.mu.RUnlock()
	return exists
}

// Dispatch 锁内只取 handler 快照，锁外执行上层代码。
func (r *MessageRouter) Dispatch(session Session, packet *NetPacket) error {
	if packet == nil {
		return nil
	}
	r.mu.RLock()
	handler := r.handlers[int32(packet.ProtoId)]
	if handler == nil {
		handler = r.fallback
	}
	r.mu.RUnlock()
	if handler == nil {
		return nil
	}
	return handler(session, packet)
}
