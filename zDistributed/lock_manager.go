package zDistributed

import (
	"context"
	"sync"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// LockManager 分布式锁管理器
type LockManager struct {
	client    *clientv3.Client
	locks     map[string]DistributedLock
	lockMutex sync.RWMutex
	options   LockOptions
}

// NewLockManager 创建分布式锁管理器
func NewLockManager(client *clientv3.Client, options LockOptions) *LockManager {
	return &LockManager{
		client:  client,
		locks:   make(map[string]DistributedLock),
		options: options,
	}
}

// GetLock 获取或创建分布式锁
func (m *LockManager) GetLock(key string) DistributedLock {
	m.lockMutex.RLock()
	lock, exists := m.locks[key]
	m.lockMutex.RUnlock()

	if exists {
		return lock
	}

	m.lockMutex.Lock()
	defer m.lockMutex.Unlock()

	// 再次检查，避免并发创建
	lock, exists = m.locks[key]
	if exists {
		return lock
	}

	lock = NewEtcdLock(m.client, key, m.options)
	m.locks[key] = lock
	return lock
}

// ReleaseLock 释放并移除分布式锁
func (m *LockManager) ReleaseLock(key string) error {
	m.lockMutex.Lock()
	defer m.lockMutex.Unlock()

	lock, exists := m.locks[key]
	if !exists {
		return nil
	}

	err := lock.Unlock(context.Background())
	if err != nil {
		return err
	}

	delete(m.locks, key)
	return nil
}

// ReleaseAll 释放所有分布式锁
func (m *LockManager) ReleaseAll() error {
	m.lockMutex.Lock()
	defer m.lockMutex.Unlock()

	for key, lock := range m.locks {
		if err := lock.Unlock(context.Background()); err != nil {
			return err
		}
		delete(m.locks, key)
	}

	return nil
}

// GetLockCount 获取当前锁数量
func (m *LockManager) GetLockCount() int {
	m.lockMutex.RLock()
	defer m.lockMutex.RUnlock()

	return len(m.locks)
}
