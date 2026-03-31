package zDistributed

import (
	"context"
)

// DistributedLock 分布式锁接口
type DistributedLock interface {
	// Lock 获取锁
	Lock(ctx context.Context) error

	// Unlock 释放锁
	Unlock(ctx context.Context) error

	// IsLocked 检查是否持有锁
	IsLocked() bool

	// GetKey 获取锁的键名
	GetKey() string
}

// LockType 锁类型
type LockType string

const (
	// LockTypeExclusive 排他锁
	LockTypeExclusive LockType = "exclusive"

	// LockTypeShared 共享锁
	LockTypeShared LockType = "shared"
)

// LockOptions 锁选项
type LockOptions struct {
	// Type 锁类型
	Type LockType

	// LeaseTTL 租约过期时间（秒）
	LeaseTTL int64

	// RetryCount 重试次数
	RetryCount int

	// RetryInterval 重试间隔（毫秒）
	RetryInterval int
}

// DefaultLockOptions 默认锁选项
var DefaultLockOptions = LockOptions{
	Type:          LockTypeExclusive,
	LeaseTTL:      10,
	RetryCount:    3,
	RetryInterval: 100,
}
