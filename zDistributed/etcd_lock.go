package zDistributed

import (
	"context"
	"errors"
	"sync/atomic"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// EtcdLock 基于etcd的分布式锁实现
type EtcdLock struct {
	client     *clientv3.Client
	lease      clientv3.LeaseID
	lockKey    string
	isLocked   atomic.Bool
	cancelFunc context.CancelFunc
	options    LockOptions
}

// NewEtcdLock 创建基于etcd的分布式锁
func NewEtcdLock(client *clientv3.Client, key string, options LockOptions) *EtcdLock {
	if options.LeaseTTL <= 0 {
		options.LeaseTTL = DefaultLockOptions.LeaseTTL
	}

	return &EtcdLock{
		client:  client,
		lockKey: key,
		options: options,
	}
}

// Lock 获取锁
func (l *EtcdLock) Lock(ctx context.Context) error {
	if l.isLocked.Load() {
		return nil
	}

	// 创建租约
	leaseResp, err := l.client.Grant(ctx, l.options.LeaseTTL)
	if err != nil {
		return err
	}
	l.lease = leaseResp.ID

	// 创建上下文用于自动续约
	ctxWithCancel, cancel := context.WithCancel(ctx)
	l.cancelFunc = cancel

	// 启动续约协程
	go l.keepAlive(ctxWithCancel)

	// 尝试获取锁
	lockResp, err := l.client.Txn(ctx).
		If(clientv3.Compare(clientv3.CreateRevision(l.lockKey), "=", 0)).
		Then(clientv3.OpPut(l.lockKey, "", clientv3.WithLease(l.lease))).
		Commit()

	if err != nil {
		cancel()
		return err
	}

	if !lockResp.Succeeded {
		cancel()
		return errors.New("lock already held")
	}

	l.isLocked.Store(true)
	return nil
}

// Unlock 释放锁
func (l *EtcdLock) Unlock(ctx context.Context) error {
	if !l.isLocked.Load() {
		return nil
	}

	// 取消续约
	if l.cancelFunc != nil {
		l.cancelFunc()
	}

	// 删除锁
	_, err := l.client.Delete(ctx, l.lockKey)
	if err != nil {
		return err
	}

	l.isLocked.Store(false)
	return nil
}

// IsLocked 检查是否持有锁
func (l *EtcdLock) IsLocked() bool {
	return l.isLocked.Load()
}

// GetKey 获取锁的键名
func (l *EtcdLock) GetKey() string {
	return l.lockKey
}

// keepAlive 保持租约活跃
func (l *EtcdLock) keepAlive(ctx context.Context) {
	ch, err := l.client.KeepAlive(ctx, l.lease)
	if err != nil {
		return
	}

	for range ch {
		// 续约成功，继续监听
	}
}
