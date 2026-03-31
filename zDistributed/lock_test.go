package zDistributed

import (
	"context"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// TestEtcdLock_WithServer 测试使用实际etcd服务器的分布式锁
func TestEtcdLock_WithServer(t *testing.T) {
	// 连接到etcd服务器
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"192.168.91.128:2379"},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Failed to create etcd client: %v", err)
	}
	defer client.Close()

	// 创建锁
	lock := NewEtcdLock(client, "test-lock", DefaultLockOptions)
	ctx := context.Background()

	// 测试获取锁
	err = lock.Lock(ctx)
	if err != nil {
		t.Fatalf("Failed to lock: %v", err)
	}
	if !lock.IsLocked() {
		t.Error("Lock should be locked after Lock()")
	}

	// 模拟业务操作
	time.Sleep(2 * time.Second)

	// 测试释放锁
	err = lock.Unlock(ctx)
	if err != nil {
		t.Fatalf("Failed to unlock: %v", err)
	}
	if lock.IsLocked() {
		t.Error("Lock should not be locked after Unlock()")
	}
}

// TestLockManager_WithServer 测试使用实际etcd服务器的锁管理器
func TestLockManager_WithServer(t *testing.T) {
	// 连接到etcd服务器
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"192.168.91.128:2379"},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Failed to create etcd client: %v", err)
	}
	defer client.Close()

	// 创建锁管理器
	manager := NewLockManager(client, DefaultLockOptions)

	// 获取锁
	lock := manager.GetLock("test-lock-2")
	ctx := context.Background()

	// 测试获取锁
	err = lock.Lock(ctx)
	if err != nil {
		t.Fatalf("Failed to lock: %v", err)
	}
	if !lock.IsLocked() {
		t.Error("Lock should be locked after Lock()")
	}

	// 测试锁数量
	if manager.GetLockCount() != 1 {
		t.Errorf("Expected lock count 1, got %d", manager.GetLockCount())
	}

	// 模拟业务操作
	time.Sleep(2 * time.Second)

	// 测试释放锁
	err = manager.ReleaseLock("test-lock-2")
	if err != nil {
		t.Fatalf("Failed to unlock via manager: %v", err)
	}
	if lock.IsLocked() {
		t.Error("Lock should not be locked after Unlock()")
	}

	// 测试锁数量
	if manager.GetLockCount() != 0 {
		t.Errorf("Expected lock count 0, got %d", manager.GetLockCount())
	}
}

// TestLockOptions 测试锁选项
func TestLockOptions(t *testing.T) {
	// 测试默认锁选项
	options := DefaultLockOptions
	if options.Type != LockTypeExclusive {
		t.Errorf("Expected default lock type to be exclusive, got %s", options.Type)
	}
	if options.LeaseTTL != 10 {
		t.Errorf("Expected default lease TTL to be 10, got %d", options.LeaseTTL)
	}
	if options.RetryCount != 3 {
		t.Errorf("Expected default retry count to be 3, got %d", options.RetryCount)
	}
	if options.RetryInterval != 100 {
		t.Errorf("Expected default retry interval to be 100, got %d", options.RetryInterval)
	}
}
