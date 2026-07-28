package zDistributed

import (
	"context"
	"os"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// 需要真 etcd 的用例此前把端点**写死**成一台早已不存在的机器（192.168.91.128:2379），
// 且用 context.Background() 无超时地 Lock ——clientv3.New 不预拨号、不会失败，于是
// Lock 里的 Grant 永远等下去：`go test ./...` 整包挂到超时（600s）才算完。
//
// 现在：端点可配（ZMMO_TEST_ETCD，默认取本环境常用的那台），跑之前先短超时探活，
// **连不上就 Skip 而不是挂死**；所有 etcd 调用都带超时，坏掉时是干净失败而非无限等待。

// testEtcdEndpoint 返回测试用 etcd 端点。
func testEtcdEndpoint() string {
	if v := os.Getenv("ZMMO_TEST_ETCD"); v != "" {
		return v
	}
	return "192.168.251.134:2379"
}

// dialTestEtcd 连 etcd 并做一次带超时的探活。不可用则 Skip（CI/离线开发机上没有 etcd 是常态，
// 不该因此把整个引擎的测试判红，更不该挂死）。
func dialTestEtcd(t *testing.T) *clientv3.Client {
	t.Helper()

	endpoint := testEtcdEndpoint()
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{endpoint},
		DialTimeout: 3 * time.Second,
	})
	if err != nil {
		t.Skipf("etcd 不可用（创建客户端失败 %s: %v），跳过", endpoint, err)
	}

	// clientv3.New 是惰性的，必须真发一次请求才知道通不通。
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := client.Get(ctx, "zdistributed-probe"); err != nil {
		_ = client.Close()
		t.Skipf("etcd 不可达（%s: %v），跳过需要真 etcd 的用例", endpoint, err)
	}

	t.Cleanup(func() { _ = client.Close() })
	return client
}

// testCtx 带超时的上下文：坏掉时干净失败，不无限等待。
func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// TestEtcdLock_WithServer 真 etcd 上的加锁/解锁往返。
func TestEtcdLock_WithServer(t *testing.T) {
	client := dialTestEtcd(t)

	lock := NewEtcdLock(client, "zdistributed-test-lock", DefaultLockOptions)
	ctx := testCtx(t)

	if err := lock.Lock(ctx); err != nil {
		t.Fatalf("Lock 失败: %v", err)
	}
	if !lock.IsLocked() {
		t.Fatalf("Lock() 之后应处于已加锁状态")
	}

	if err := lock.Unlock(ctx); err != nil {
		t.Fatalf("Unlock 失败: %v", err)
	}
	if lock.IsLocked() {
		t.Fatalf("Unlock() 之后不应仍处于已加锁状态")
	}
}

// TestEtcdLock_SecondHolderBlocked 互斥性：锁被占住时，另一个持有者拿不到
// —— 这才是分布式锁存在的意义，原用例完全没测。
func TestEtcdLock_SecondHolderBlocked(t *testing.T) {
	client := dialTestEtcd(t)

	const key = "zdistributed-test-lock-mutex"
	first := NewEtcdLock(client, key, DefaultLockOptions)
	ctx := testCtx(t)

	if err := first.Lock(ctx); err != nil {
		t.Fatalf("首个持有者加锁失败: %v", err)
	}
	defer func() { _ = first.Unlock(context.Background()) }()

	second := NewEtcdLock(client, key, LockOptions{
		Type:          LockTypeExclusive,
		LeaseTTL:      DefaultLockOptions.LeaseTTL,
		RetryCount:    1,
		RetryInterval: 50,
	})
	if err := second.Lock(ctx); err == nil {
		_ = second.Unlock(ctx)
		t.Fatalf("锁已被占用时，第二个持有者不应加锁成功")
	}
	if second.IsLocked() {
		t.Fatalf("加锁失败的持有者不应标记为已加锁")
	}

	// 首个持有者释放后，第二个应能拿到。
	if err := first.Unlock(ctx); err != nil {
		t.Fatalf("首个持有者释放失败: %v", err)
	}
	if err := second.Lock(ctx); err != nil {
		t.Fatalf("锁释放后第二个持有者应能加锁: %v", err)
	}
	_ = second.Unlock(ctx)
}

// TestLockManager_WithServer 锁管理器的获取/计数/释放。
func TestLockManager_WithServer(t *testing.T) {
	client := dialTestEtcd(t)

	manager := NewLockManager(client, DefaultLockOptions)
	lock := manager.GetLock("zdistributed-test-lock-2")
	ctx := testCtx(t)

	if err := lock.Lock(ctx); err != nil {
		t.Fatalf("Lock 失败: %v", err)
	}
	if !lock.IsLocked() {
		t.Fatalf("Lock() 之后应处于已加锁状态")
	}
	if got := manager.GetLockCount(); got != 1 {
		t.Fatalf("锁数量应为 1, got %d", got)
	}

	if err := manager.ReleaseLock("zdistributed-test-lock-2"); err != nil {
		t.Fatalf("经管理器释放失败: %v", err)
	}
	if lock.IsLocked() {
		t.Fatalf("释放后不应仍处于已加锁状态")
	}
	if got := manager.GetLockCount(); got != 0 {
		t.Fatalf("释放后锁数量应为 0, got %d", got)
	}
}

// TestLockOptions 默认锁选项（纯本地，不需要 etcd）。
func TestLockOptions(t *testing.T) {
	options := DefaultLockOptions
	if options.Type != LockTypeExclusive {
		t.Errorf("默认锁类型应为独占, got %s", options.Type)
	}
	if options.LeaseTTL != 10 {
		t.Errorf("默认租约 TTL 应为 10, got %d", options.LeaseTTL)
	}
	if options.RetryCount != 3 {
		t.Errorf("默认重试次数应为 3, got %d", options.RetryCount)
	}
	if options.RetryInterval != 100 {
		t.Errorf("默认重试间隔应为 100, got %d", options.RetryInterval)
	}
}
