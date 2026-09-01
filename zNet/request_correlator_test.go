package zNet

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestRequestRouter_SyncCall 发送 + 另一路径 CompleteRequest → 缝合成一次同步调用拿到响应。
func TestRequestRouter_SyncCall(t *testing.T) {
	rr := NewRequestRouter(2 * time.Second)
	reqID := rr.NextRequestID()
	go func() {
		time.Sleep(10 * time.Millisecond)
		rr.CompleteRequest(reqID, []byte("pong"), nil)
	}()
	data, err := rr.SendRequest(context.Background(), reqID, func() error { return nil })
	if err != nil {
		t.Fatalf("SendRequest err: %v", err)
	}
	if string(data) != "pong" {
		t.Fatalf("期望 pong, got %q", data)
	}
	if rr.PendingCount() != 0 {
		t.Fatalf("完成后不应有挂起项, got %d", rr.PendingCount())
	}
}

// TestRequestRouter_Timeout 无响应则超时返回错误。
func TestRequestRouter_Timeout(t *testing.T) {
	rr := NewRequestRouter(30 * time.Millisecond)
	reqID := rr.NextRequestID()
	_, err := rr.SendRequest(context.Background(), reqID, func() error { return nil })
	if err == nil {
		t.Fatal("无响应应超时返回错误")
	}
	if rr.PendingCount() != 0 {
		t.Fatalf("超时后应清除挂起项, got %d", rr.PendingCount())
	}
}

// TestRequestRouter_SendFnError sendFn 失败则立即返回错误、不留挂起项。
func TestRequestRouter_SendFnError(t *testing.T) {
	rr := NewRequestRouter(time.Second)
	reqID := rr.NextRequestID()
	_, err := rr.SendRequest(context.Background(), reqID, func() error { return fmt.Errorf("boom") })
	if err == nil {
		t.Fatal("sendFn 失败应返回错误")
	}
	if rr.PendingCount() != 0 {
		t.Fatalf("发送失败应清除挂起项, got %d", rr.PendingCount())
	}
}

// TestRequestRouter_Cleanup 过期挂起项被 Cleanup 清理并收到 expired 错误。
func TestRequestRouter_Cleanup(t *testing.T) {
	rr := NewRequestRouter(1 * time.Millisecond)
	ch := rr.RegisterPending(rr.NextRequestID())
	time.Sleep(10 * time.Millisecond)
	rr.Cleanup()
	if rr.PendingCount() != 0 {
		t.Fatalf("Cleanup 后应无挂起项, got %d", rr.PendingCount())
	}
	select {
	case res := <-ch:
		if res.Error == nil {
			t.Fatal("过期项应收到错误结果")
		}
	default:
		t.Fatal("过期项应被 Cleanup 送回一个错误结果")
	}
}

// TestRequestRouter_Concurrent 并发发送/应答无死锁、无错配（-race 验证无数据竞争）。
func TestRequestRouter_Concurrent(t *testing.T) {
	rr := NewRequestRouter(2 * time.Second)
	const n = 50
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reqID := rr.NextRequestID()
			want := fmt.Sprintf("r%d", reqID)
			go func() {
				time.Sleep(time.Millisecond)
				rr.CompleteRequest(reqID, []byte(want), nil)
			}()
			data, err := rr.SendRequest(context.Background(), reqID, func() error { return nil })
			if err != nil {
				errs <- fmt.Errorf("req %d: %w", reqID, err)
				return
			}
			if string(data) != want {
				errs <- fmt.Errorf("req %d: 错配 got %q want %q", reqID, data, want)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
	if rr.PendingCount() != 0 {
		t.Fatalf("全部完成后应无挂起项, got %d", rr.PendingCount())
	}
}

func TestRequestRouterRejectsDuplicateRequestID(t *testing.T) {
	rr := NewRequestRouter(time.Second)
	reqID := rr.NextRequestID()
	if _, err := rr.TryRegisterPending(reqID); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if _, err := rr.TryRegisterPending(reqID); !errors.Is(err, ErrDuplicateRequestID) {
		t.Fatalf("duplicate register error = %v", err)
	}
}

func TestRequestRouterCloseFailsAllPendingAndRejectsNewRequests(t *testing.T) {
	rr := NewRequestRouter(time.Second)
	ch1, err := rr.TryRegisterPending(rr.NextRequestID())
	if err != nil {
		t.Fatal(err)
	}
	ch2, err := rr.TryRegisterPending(rr.NextRequestID())
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("transport stopped")
	rr.Close(cause)

	for i, ch := range []<-chan *ResponseResult{ch1, ch2} {
		select {
		case result := <-ch:
			if !errors.Is(result.Error, cause) {
				t.Fatalf("pending %d error = %v", i, result.Error)
			}
		case <-time.After(time.Second):
			t.Fatalf("pending %d was not failed by Close", i)
		}
	}
	if rr.PendingCount() != 0 {
		t.Fatalf("pending after close = %d", rr.PendingCount())
	}
	if _, err := rr.TryRegisterPending(rr.NextRequestID()); !errors.Is(err, ErrRequestRouterClosed) || !errors.Is(err, cause) {
		t.Fatalf("register after close error = %v, want router closed and original cause", err)
	}
	if _, err := rr.SendRequest(context.Background(), rr.NextRequestID(), func() error { return nil }); !errors.Is(err, ErrRequestRouterClosed) {
		t.Fatalf("SendRequest after close error = %v", err)
	}
}

func TestRequestRouterCanceledContextDoesNotSend(t *testing.T) {
	rr := NewRequestRouter(time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sent := false
	_, err := rr.SendRequest(ctx, rr.NextRequestID(), func() error {
		sent = true
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SendRequest error = %v, want context canceled", err)
	}
	if sent {
		t.Fatal("SendRequest called send function for an already canceled context")
	}
	if rr.PendingCount() != 0 {
		t.Fatalf("pending after canceled SendRequest = %d", rr.PendingCount())
	}
}

func TestRequestRouterConcurrentSendAndClose(t *testing.T) {
	rr := NewRequestRouter(time.Second)
	cause := errors.New("transport stopping")
	const callers = 64
	start := make(chan struct{})
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := rr.SendRequest(context.Background(), rr.NextRequestID(), func() error { return nil })
			errs <- err
		}()
	}
	close(start)
	rr.Close(cause)
	wg.Wait()
	close(errs)
	for err := range errs {
		if !errors.Is(err, cause) {
			t.Fatalf("concurrent SendRequest error = %v, want close cause", err)
		}
	}
	if rr.PendingCount() != 0 {
		t.Fatalf("pending after concurrent Close = %d", rr.PendingCount())
	}
}
