package zNet

import (
	"errors"
	"sync"
	"testing"
)

func TestMessageRouter_Dispatch(t *testing.T) {
	r := NewMessageRouter()

	var got ProtoIdType = -1
	r.RegisterHandler(1001, func(_ Session, p *NetPacket) error {
		got = p.ProtoId
		return nil
	})

	// 命中已注册协议
	if err := r.Dispatch(nil, &NetPacket{ProtoId: 1001}); err != nil {
		t.Fatalf("dispatch returned err: %v", err)
	}
	if got != 1001 {
		t.Fatalf("handler not invoked, got=%d", got)
	}

	// 未注册协议且无 fallback：丢弃，返回 nil
	got = -1
	if err := r.Dispatch(nil, &NetPacket{ProtoId: 9999}); err != nil {
		t.Fatalf("unknown proto should return nil, got: %v", err)
	}
	if got != -1 {
		t.Fatalf("handler should not have run for unknown proto")
	}
}

func TestMessageRouterStrictRegistrationAndSeal(t *testing.T) {
	r := NewMessageRouter()
	h := func(_ Session, _ *NetPacket) error { return nil }
	if err := r.TryRegisterHandler(1, h); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := r.TryRegisterHandler(1, h); !errors.Is(err, ErrHandlerAlreadyRegistered) {
		t.Fatalf("duplicate register error = %v", err)
	}
	if err := r.TrySetFallback(h); err != nil {
		t.Fatalf("set fallback: %v", err)
	}
	r.Seal()
	if !r.IsSealed() {
		t.Fatal("router should be sealed")
	}
	if err := r.TryRegisterHandler(2, h); !errors.Is(err, ErrMessageRouterSealed) {
		t.Fatalf("register after seal error = %v", err)
	}
	if err := r.TryUnregisterHandler(1); !errors.Is(err, ErrMessageRouterSealed) {
		t.Fatalf("unregister after seal error = %v", err)
	}
	if err := r.TrySetFallback(nil); !errors.Is(err, ErrMessageRouterSealed) {
		t.Fatalf("fallback after seal error = %v", err)
	}
}

func TestMessageRouterConcurrentFallbackAndDispatch(t *testing.T) {
	r := NewMessageRouter()
	h := func(_ Session, _ *NetPacket) error { return nil }
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = r.TrySetFallback(h)
		}()
		go func() {
			defer wg.Done()
			_ = r.Dispatch(nil, &NetPacket{ProtoId: 999})
		}()
	}
	wg.Wait()
}

func TestMessageRouterZeroValueSupportsStrictRegistration(t *testing.T) {
	var router MessageRouter
	called := false
	if err := router.TryRegisterHandler(77, func(Session, *NetPacket) error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("zero-value registration: %v", err)
	}
	if err := router.Dispatch(nil, &NetPacket{ProtoId: 77}); err != nil {
		t.Fatalf("zero-value dispatch: %v", err)
	}
	if !called {
		t.Fatal("zero-value router did not dispatch registered handler")
	}
}

func TestMessageRouter_Fallback(t *testing.T) {
	r := NewMessageRouter()
	var fallbackProto ProtoIdType = -1
	r.SetFallback(func(_ Session, p *NetPacket) error {
		fallbackProto = p.ProtoId
		return nil
	})

	if err := r.Dispatch(nil, &NetPacket{ProtoId: 7777}); err != nil {
		t.Fatalf("dispatch err: %v", err)
	}
	if fallbackProto != 7777 {
		t.Fatalf("fallback not invoked for unknown proto, got=%d", fallbackProto)
	}
}

func TestMessageRouter_HandlerErrorPropagates(t *testing.T) {
	r := NewMessageRouter()
	sentinel := errors.New("boom")
	r.RegisterHandler(1, func(_ Session, _ *NetPacket) error { return sentinel })
	if err := r.Dispatch(nil, &NetPacket{ProtoId: 1}); !errors.Is(err, sentinel) {
		t.Fatalf("expected handler error to propagate, got: %v", err)
	}
}

func TestMessageRouter_RegisterUnregisterHasHandler(t *testing.T) {
	r := NewMessageRouter()
	if r.HasHandler(1) {
		t.Fatal("should not have handler before register")
	}
	r.RegisterHandler(1, func(_ Session, _ *NetPacket) error { return nil })
	if !r.HasHandler(1) {
		t.Fatal("should have handler after register")
	}
	r.UnregisterHandler(1)
	if r.HasHandler(1) {
		t.Fatal("should not have handler after unregister")
	}

	// nil handler 不注册
	r.RegisterHandler(2, nil)
	if r.HasHandler(2) {
		t.Fatal("nil handler must not be registered")
	}

	// nil packet 不 panic、返回 nil
	if err := r.Dispatch(nil, nil); err != nil {
		t.Fatalf("nil packet should return nil, got: %v", err)
	}
}
