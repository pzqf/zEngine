package zDistributed

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

// EtcdLock is an exclusive, lease-backed lock. New code should use Acquire
// and retain its LockHandle; Lock/Unlock remain advisory compatibility APIs.
type EtcdLock struct {
	client  *clientv3.Client
	lockKey string
	options LockOptions

	mu      sync.Mutex
	current *LockHandle
}

var _ FencedLock = (*EtcdLock)(nil)

func NewEtcdLock(client *clientv3.Client, key string, options LockOptions) *EtcdLock {
	if options.Type == "" {
		options.Type = LockTypeExclusive
	}
	if options.LeaseTTL <= 0 {
		options.LeaseTTL = DefaultLockOptions.LeaseTTL
	}
	if options.RetryCount > 0 && options.RetryInterval <= 0 {
		options.RetryInterval = DefaultLockOptions.RetryInterval
	}
	return &EtcdLock{client: client, lockKey: key, options: options}
}

// Acquire returns the current valid handle or obtains a new exclusive owner.
// The acquisition context bounds RPCs and retry waits; the acquired lease has
// an independent lifecycle and remains valid until Release or ownership loss.
func (l *EtcdLock) Acquire(ctx context.Context) (*LockHandle, error) {
	if l == nil {
		return nil, fmt.Errorf("%w: nil lock", ErrInvalidLockConfig)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := l.validateConfig(); err != nil {
		return nil, err
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.current != nil && l.current.Valid() {
		return l.current, nil
	}
	l.current = nil

	leaseResponse, err := l.client.Grant(ctx, l.options.LeaseTTL)
	if err != nil {
		return nil, fmt.Errorf("grant lock lease: %w", err)
	}

	lifecycleCtx, cancel := context.WithCancel(context.Background())
	session, err := concurrency.NewSession(
		l.client,
		concurrency.WithLease(leaseResponse.ID),
		concurrency.WithContext(lifecycleCtx),
	)
	if err != nil {
		cancel()
		l.revokeLease(leaseResponse.ID)
		return nil, fmt.Errorf("start lock lease session: %w", err)
	}

	mutex := concurrency.NewMutex(session, l.lockKey)
	if err := l.tryAcquire(ctx, mutex); err != nil {
		cancel()
		l.revokeLease(leaseResponse.ID)
		return nil, err
	}

	ownerResponse, err := l.client.Get(ctx, mutex.Key())
	if err != nil {
		cancel()
		l.revokeLease(leaseResponse.ID)
		return nil, fmt.Errorf("read lock owner: %w", err)
	}
	if len(ownerResponse.Kvs) != 1 {
		cancel()
		l.revokeLease(leaseResponse.ID)
		return nil, ErrLockOwnershipLost
	}
	owner := ownerResponse.Kvs[0]
	if owner.Lease != int64(leaseResponse.ID) || owner.CreateRevision <= 0 {
		cancel()
		l.revokeLease(leaseResponse.ID)
		return nil, ErrLockOwnershipLost
	}

	handle := newLockHandle(l, l.lockKey, mutex.Key(), leaseResponse.ID, owner.CreateRevision, cancel)
	l.current = handle
	go l.monitorOwnership(session, handle)

	select {
	case <-handle.Done():
		l.current = nil
		return nil, ErrLockOwnershipLost
	default:
		return handle, nil
	}
}

func (l *EtcdLock) tryAcquire(ctx context.Context, mutex *concurrency.Mutex) error {
	for attempt := 0; ; attempt++ {
		err := mutex.TryLock(ctx)
		if err == nil {
			return nil
		}
		if !errors.Is(err, concurrency.ErrLocked) {
			return fmt.Errorf("acquire lock %q: %w", l.lockKey, err)
		}
		if attempt >= l.options.RetryCount {
			return fmt.Errorf("%w: key %q after %d attempts", ErrLockAlreadyHeld, l.lockKey, attempt+1)
		}
		if err := waitRetry(ctx, time.Duration(l.options.RetryInterval)*time.Millisecond); err != nil {
			return err
		}
	}
}

func waitRetry(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (l *EtcdLock) validateConfig() error {
	if l.options.Type != LockTypeExclusive {
		return fmt.Errorf("%w: %q", ErrUnsupportedLockType, l.options.Type)
	}
	if l.client == nil {
		return fmt.Errorf("%w: nil etcd client", ErrInvalidLockConfig)
	}
	if l.lockKey == "" {
		return fmt.Errorf("%w: empty lock key", ErrInvalidLockConfig)
	}
	if l.options.RetryCount < 0 || l.options.RetryInterval < 0 {
		return fmt.Errorf("%w: negative retry option", ErrInvalidLockConfig)
	}
	return nil
}

func (l *EtcdLock) monitorOwnership(session *concurrency.Session, handle *LockHandle) {
	watch := l.client.Watch(session.Ctx(), handle.ownerToken, clientv3.WithRev(handle.fencingToken+1))
	for {
		select {
		case <-session.Done():
			l.observeOwnershipLoss(handle)
			return
		case response, ok := <-watch:
			if !ok || response.Canceled || response.Err() != nil {
				l.observeOwnershipLoss(handle)
				return
			}
			for _, event := range response.Events {
				if event.Type == clientv3.EventTypeDelete ||
					event.Kv.CreateRevision != handle.fencingToken ||
					event.Kv.Lease != int64(handle.leaseID) {
					l.observeOwnershipLoss(handle)
					return
				}
			}
		}
	}
}

func (l *EtcdLock) observeOwnershipLoss(handle *LockHandle) {
	handle.lifecycle.lossObserved.Store(true)
	if !handle.lifecycle.releaseIntent.Load() {
		l.markLost(handle)
	}
}

func (l *EtcdLock) markLost(handle *LockHandle) {
	handle.finish(lockHandleLost)
	l.mu.Lock()
	if sameLockHandle(l.current, handle) {
		l.current = nil
	}
	l.mu.Unlock()
}

// Validate checks the owner key, lease, and fencing revision in etcd.
func (l *EtcdLock) Validate(ctx context.Context, handle *LockHandle) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := l.validateHandle(handle); err != nil {
		return err
	}
	response, err := l.client.Txn(ctx).
		If(handle.ownershipComparisons()...).
		Then(clientv3.OpGet(handle.ownerToken)).
		Commit()
	if err != nil {
		return fmt.Errorf("validate lock owner: %w", err)
	}
	if !response.Succeeded {
		l.markLost(handle)
		return ErrLockOwnershipLost
	}
	return nil
}

// GuardedTxn commits etcd operations only while the supplied handle still owns
// its unique owner key, lease, and fencing revision.
func (l *EtcdLock) GuardedTxn(ctx context.Context, handle *LockHandle, operations ...clientv3.Op) (*clientv3.TxnResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := l.validateHandle(handle); err != nil {
		return nil, err
	}
	response, err := l.client.Txn(ctx).
		If(handle.ownershipComparisons()...).
		Then(operations...).
		Commit()
	if err != nil {
		return nil, fmt.Errorf("guarded lock transaction: %w", err)
	}
	if !response.Succeeded {
		l.markLost(handle)
		return response, ErrLockOwnershipLost
	}
	return response, nil
}

func (l *EtcdLock) validateHandle(handle *LockHandle) error {
	if l == nil || handle == nil || handle.lock != l {
		return fmt.Errorf("%w: handle does not belong to lock", ErrInvalidLockConfig)
	}
	if !handle.Valid() {
		return ErrLockOwnershipLost
	}
	return nil
}

// Release uses compare-delete, so a stale handle can never delete a successor.
func (l *EtcdLock) Release(ctx context.Context, handle *LockHandle) error {
	if l == nil || handle == nil || handle.lock != l {
		return fmt.Errorf("%w: handle does not belong to lock", ErrInvalidLockConfig)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	state := lockHandleState(handle.lifecycle.state.Load())
	if state == lockHandleReleased {
		return nil
	}
	if state != lockHandleActive {
		return ErrLockOwnershipLost
	}

	handle.lifecycle.releaseIntent.Store(true)
	response, err := l.client.Txn(ctx).
		If(handle.ownershipComparisons()...).
		Then(clientv3.OpDelete(handle.ownerToken)).
		Commit()
	if err != nil {
		releaseErr := fmt.Errorf("release lock owner: %w", err)
		if l.finishFailedReleaseLocked(handle) {
			return errors.Join(releaseErr, ErrLockOwnershipLost)
		}
		return releaseErr
	}
	if !response.Succeeded {
		handle.finish(lockHandleLost)
		if sameLockHandle(l.current, handle) {
			l.current = nil
		}
		return ErrLockOwnershipLost
	}

	handle.finish(lockHandleReleased)
	if sameLockHandle(l.current, handle) {
		l.current = nil
	}
	l.revokeLease(handle.leaseID)
	return nil
}

// finishFailedReleaseLocked reopens local ownership only when the monitor did
// not observe session or owner-key loss while the release RPC was in flight.
// The caller must hold l.mu.
func (l *EtcdLock) finishFailedReleaseLocked(handle *LockHandle) bool {
	handle.lifecycle.releaseIntent.Store(false)
	if !handle.lifecycle.lossObserved.Load() {
		return false
	}
	handle.finish(lockHandleLost)
	if sameLockHandle(l.current, handle) {
		l.current = nil
	}
	return true
}

// Lock is the compatibility acquisition API. New code should retain the
// LockHandle returned by Acquire and fence every protected write.
func (l *EtcdLock) Lock(ctx context.Context) error {
	_, err := l.Acquire(ctx)
	return err
}

// LockWithAutoRenew is retained for compatibility; Acquire already owns an
// automatically renewed etcd session.
// Deprecated: use Acquire.
func (l *EtcdLock) LockWithAutoRenew(ctx context.Context) error {
	return l.Lock(ctx)
}

// Unlock releases the current compatibility handle with compare-delete.
func (l *EtcdLock) Unlock(ctx context.Context) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	handle := l.current
	l.mu.Unlock()
	if handle == nil {
		return nil
	}
	return l.Release(ctx, handle)
}

func (l *EtcdLock) IsLocked() bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.current != nil && l.current.Valid()
}

func (l *EtcdLock) GetKey() string {
	if l == nil {
		return ""
	}
	return l.lockKey
}

func (l *EtcdLock) revokeLease(leaseID clientv3.LeaseID) {
	if l.client == nil || leaseID == clientv3.NoLease {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = l.client.Revoke(ctx, leaseID)
}

func sameLockHandle(left, right *LockHandle) bool {
	return left != nil && right != nil && left.lifecycle != nil && left.lifecycle == right.lifecycle
}
