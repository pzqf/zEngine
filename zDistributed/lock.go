package zDistributed

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	clientv3 "go.etcd.io/etcd/client/v3"
)

var (
	ErrLockAlreadyHeld     = errors.New("lock already held")
	ErrLockOwnershipLost   = errors.New("lock ownership lost")
	ErrUnsupportedLockType = errors.New("unsupported lock type")
	ErrInvalidLockConfig   = errors.New("invalid lock config")
)

// DistributedLock is the advisory compatibility interface. It cannot carry a
// fencing token to protected writes; new ownership code should use FencedLock.
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

// FencedLock exposes the ownership handle required to fence protected writes.
type FencedLock interface {
	DistributedLock
	Acquire(ctx context.Context) (*LockHandle, error)
	Release(ctx context.Context, handle *LockHandle) error
	Validate(ctx context.Context, handle *LockHandle) error
	GuardedTxn(ctx context.Context, handle *LockHandle, operations ...clientv3.Op) (*clientv3.TxnResponse, error)
}

// LockType 锁类型
type LockType string

const (
	// LockTypeExclusive 排他锁
	LockTypeExclusive LockType = "exclusive"

	// LockTypeShared is retained for source compatibility and rejected by
	// EtcdLock because shared ownership cannot provide the exclusive fencing
	// contract promised by FencedLock.
	// Deprecated: only LockTypeExclusive is supported.
	LockTypeShared LockType = "shared"
)

// LockOptions 锁选项
type LockOptions struct {
	// Type currently supports only LockTypeExclusive. LockTypeShared is kept
	// as a compatibility constant and is rejected explicitly.
	Type LockType

	// LeaseTTL 租约过期时间（秒）
	LeaseTTL int64

	// RetryCount is the number of retries after the initial acquisition attempt.
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

type lockHandleState uint32

const (
	lockHandleActive lockHandleState = iota + 1
	lockHandleReleased
	lockHandleLost
)

// LockHandle is the immutable ownership capability returned by Acquire.
// FencingToken is the create revision of OwnerToken and is monotonically
// increasing for successive owners of the same resource key.
type LockHandle struct {
	lock         *EtcdLock
	resourceKey  string
	ownerToken   string
	leaseID      clientv3.LeaseID
	fencingToken int64
	lifecycle    *lockHandleLifecycle
}

type lockHandleLifecycle struct {
	state         atomic.Uint32
	done          chan struct{}
	finishOnce    sync.Once
	releaseIntent atomic.Bool
	lossObserved  atomic.Bool
	cancel        context.CancelFunc
}

func newLockHandle(lock *EtcdLock, resourceKey, ownerToken string, leaseID clientv3.LeaseID, fencingToken int64, cancel context.CancelFunc) *LockHandle {
	handle := &LockHandle{
		lock:         lock,
		resourceKey:  resourceKey,
		ownerToken:   ownerToken,
		leaseID:      leaseID,
		fencingToken: fencingToken,
		lifecycle: &lockHandleLifecycle{
			done:   make(chan struct{}),
			cancel: cancel,
		},
	}
	handle.lifecycle.state.Store(uint32(lockHandleActive))
	return handle
}

// Valid reports local ownership knowledge. Distributed writes must still use
// OwnershipComparisons or GuardedTxn to fence an owner that has not observed a
// remote lease loss yet.
func (h *LockHandle) Valid() bool {
	return h != nil && h.lifecycle != nil && lockHandleState(h.lifecycle.state.Load()) == lockHandleActive
}

// Done closes when this handle is released or loses ownership.
func (h *LockHandle) Done() <-chan struct{} {
	if h == nil || h.lifecycle == nil {
		return closedLockHandleDone
	}
	return h.lifecycle.done
}

// Lost reports whether ownership ended because its lease or owner key was lost.
func (h *LockHandle) Lost() bool {
	return h != nil && h.lifecycle != nil && lockHandleState(h.lifecycle.state.Load()) == lockHandleLost
}

func (h *LockHandle) ResourceKey() string {
	if h == nil {
		return ""
	}
	return h.resourceKey
}

func (h *LockHandle) OwnerToken() string {
	if h == nil {
		return ""
	}
	return h.ownerToken
}

func (h *LockHandle) LeaseID() clientv3.LeaseID {
	if h == nil {
		return clientv3.NoLease
	}
	return h.leaseID
}

func (h *LockHandle) FencingToken() int64 {
	if h == nil {
		return 0
	}
	return h.fencingToken
}

// OwnershipComparisons can be composed into an etcd transaction that must be
// rejected after this owner loses its unique key, lease, or fencing revision.
func (h *LockHandle) OwnershipComparisons() ([]clientv3.Cmp, error) {
	if h == nil || !h.Valid() {
		return nil, ErrLockOwnershipLost
	}
	return h.ownershipComparisons(), nil
}

func (h *LockHandle) ownershipComparisons() []clientv3.Cmp {
	return []clientv3.Cmp{
		clientv3.Compare(clientv3.CreateRevision(h.ownerToken), "=", h.fencingToken),
		clientv3.Compare(clientv3.LeaseValue(h.ownerToken), "=", int64(h.leaseID)),
	}
}

func (h *LockHandle) finish(state lockHandleState) {
	if h == nil || h.lifecycle == nil {
		return
	}
	h.lifecycle.finishOnce.Do(func() {
		h.lifecycle.state.Store(uint32(state))
		if h.lifecycle.cancel != nil {
			h.lifecycle.cancel()
		}
		close(h.lifecycle.done)
	})
}

var closedLockHandleDone = func() <-chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}()
