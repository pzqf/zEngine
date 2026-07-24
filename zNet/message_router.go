package zNet

import (
	"github.com/pzqf/zUtil/zMap"
)

// MessageRouter 按协议 ID（ProtoId）分派消息的路由表。
//
// 背景（成熟化改造 Phase 1.1.1）：zNet 的 Server/Client 只提供单个
// RegisterDispatcher(HandlerFun)，协议分派需业务自行 switch(protoId)。
// MessageRouter 提供开箱即用的 protoId→handler 注册表，其 Dispatch 方法本身即
// 一个 HandlerFun，可直接注册到任意 Server/Client：
//
//	router := zNet.NewMessageRouter()
//	router.RegisterHandler(1001, onLogin).
//	       RegisterHandler(1002, onMove).
//	       SetFallback(onUnknown)
//	server.RegisterDispatcher(router.Dispatch)
//
// 纯附加能力，不改变现有 dispatcher 行为。并发安全（基于 zMap.TypedMap）。
type MessageRouter struct {
	handlers *zMap.TypedMap[int32, HandlerFun]
	fallback HandlerFun
}

// NewMessageRouter 创建空的消息路由表。
func NewMessageRouter() *MessageRouter {
	return &MessageRouter{
		handlers: zMap.NewTypedMap[int32, HandlerFun](),
	}
}

// RegisterHandler 注册某协议 ID 的处理函数（重复注册以最后一次为准）。返回自身以支持链式调用。
func (r *MessageRouter) RegisterHandler(protoId int32, h HandlerFun) *MessageRouter {
	if h != nil {
		r.handlers.Store(protoId, h)
	}
	return r
}

// UnregisterHandler 移除某协议 ID 的处理函数。
func (r *MessageRouter) UnregisterHandler(protoId int32) {
	r.handlers.Delete(protoId)
}

// SetFallback 设置兜底处理函数：收到未注册协议 ID 时调用。返回自身以支持链式调用。
func (r *MessageRouter) SetFallback(h HandlerFun) *MessageRouter {
	r.fallback = h
	return r
}

// HasHandler 判断某协议 ID 是否已注册处理函数。
func (r *MessageRouter) HasHandler(protoId int32) bool {
	_, ok := r.handlers.Load(protoId)
	return ok
}

// Dispatch 是一个 HandlerFun：按 packet.ProtoId 查表分派；未命中则走 fallback（若已设），
// 否则返回 nil（丢弃未知消息，交由使用方决定是否设 fallback 记录/告警）。
func (r *MessageRouter) Dispatch(session Session, packet *NetPacket) error {
	if packet == nil {
		return nil
	}
	if h, ok := r.handlers.Load(int32(packet.ProtoId)); ok {
		return h(session, packet)
	}
	if r.fallback != nil {
		return r.fallback(session, packet)
	}
	return nil
}
