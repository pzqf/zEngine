package zDistributed

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

func TestEtcdLockRejectsUnsupportedSharedMode(t *testing.T) {
	lock := NewEtcdLock(nil, contractKey(t, "shared"), LockOptions{
		Type:     LockTypeShared,
		LeaseTTL: 5,
	})
	if _, err := lock.Acquire(context.Background()); !errors.Is(err, ErrUnsupportedLockType) {
		t.Fatalf("Acquire error = %v, want ErrUnsupportedLockType", err)
	}
}

func TestEtcdLockLeaseLossFencesStaleOwner(t *testing.T) {
	client := dialTestEtcd(t)
	ctx := testCtx(t)
	key := contractKey(t, "lease-loss")
	protectedKey := key + "/protected"

	oldLock := NewEtcdLock(client, key, LockOptions{Type: LockTypeExclusive, LeaseTTL: 2})
	oldHandle, err := oldLock.Acquire(ctx)
	if err != nil {
		t.Fatalf("old owner Acquire: %v", err)
	}
	if oldHandle.ResourceKey() != key || oldHandle.OwnerToken() == "" || oldHandle.LeaseID() == 0 || oldHandle.FencingToken() <= 0 {
		t.Fatalf("invalid old handle: resource=%q owner=%q lease=%d fence=%d", oldHandle.ResourceKey(), oldHandle.OwnerToken(), oldHandle.LeaseID(), oldHandle.FencingToken())
	}

	if _, err := client.Revoke(ctx, oldHandle.LeaseID()); err != nil {
		t.Fatalf("revoke old lease: %v", err)
	}
	waitHandleDone(t, oldHandle)
	if oldHandle.Valid() || oldLock.IsLocked() {
		t.Fatal("old owner remained valid after lease loss")
	}

	newLock := NewEtcdLock(client, key, LockOptions{
		Type:          LockTypeExclusive,
		LeaseTTL:      5,
		RetryCount:    10,
		RetryInterval: 20,
	})
	newHandle, err := newLock.Acquire(ctx)
	if err != nil {
		t.Fatalf("new owner Acquire: %v", err)
	}
	t.Cleanup(func() { _ = newLock.Release(context.Background(), newHandle) })
	if newHandle.FencingToken() <= oldHandle.FencingToken() {
		t.Fatalf("fencing token did not increase: old=%d new=%d", oldHandle.FencingToken(), newHandle.FencingToken())
	}

	if err := oldLock.Release(ctx, oldHandle); !errors.Is(err, ErrLockOwnershipLost) {
		t.Fatalf("stale Release error = %v, want ErrLockOwnershipLost", err)
	}
	if err := newLock.Validate(ctx, newHandle); err != nil {
		t.Fatalf("stale Release affected new owner: %v", err)
	}

	if _, err := oldLock.GuardedTxn(ctx, oldHandle, clientv3.OpPut(protectedKey, "stale")); !errors.Is(err, ErrLockOwnershipLost) {
		t.Fatalf("stale GuardedTxn error = %v, want ErrLockOwnershipLost", err)
	}
	if response, err := client.Get(ctx, protectedKey); err != nil {
		t.Fatalf("read protected key after stale write: %v", err)
	} else if len(response.Kvs) != 0 {
		t.Fatalf("stale owner wrote protected value %q", response.Kvs[0].Value)
	}

	if _, err := newLock.GuardedTxn(ctx, newHandle, clientv3.OpPut(protectedKey, "current")); err != nil {
		t.Fatalf("current owner GuardedTxn: %v", err)
	}
	if response, err := client.Get(ctx, protectedKey); err != nil {
		t.Fatalf("read protected key: %v", err)
	} else if len(response.Kvs) != 1 || string(response.Kvs[0].Value) != "current" {
		t.Fatalf("protected value = %q, want current", response.Kvs[0].Value)
	}
}

func TestEtcdLockRetryAndConcurrentHandleAreDeterministic(t *testing.T) {
	client := dialTestEtcd(t)
	ctx := testCtx(t)
	key := contractKey(t, "retry")

	first := NewEtcdLock(client, key, LockOptions{Type: LockTypeExclusive, LeaseTTL: 5})
	firstHandle, err := first.Acquire(ctx)
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}

	second := NewEtcdLock(client, key, LockOptions{
		Type:          LockTypeExclusive,
		LeaseTTL:      5,
		RetryCount:    20,
		RetryInterval: 20,
	})
	releaseDone := make(chan error, 1)
	go func() {
		time.Sleep(60 * time.Millisecond)
		releaseDone <- first.Release(context.Background(), firstHandle)
	}()
	secondHandle, err := second.Acquire(ctx)
	if err != nil {
		t.Fatalf("retrying Acquire: %v", err)
	}
	if err := <-releaseDone; err != nil {
		t.Fatalf("release first owner: %v", err)
	}
	const callers = 32
	handles := make(chan *LockHandle, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			handle, err := second.Acquire(ctx)
			handles <- handle
			errs <- err
		}()
	}
	wg.Wait()
	close(handles)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Acquire: %v", err)
		}
	}
	for handle := range handles {
		if handle != secondHandle {
			t.Fatal("concurrent Acquire returned a different ownership handle")
		}
	}
	copiedHandle := *secondHandle
	if err := second.Release(context.Background(), &copiedHandle); err != nil {
		t.Fatalf("Release copied handle: %v", err)
	}
	if secondHandle.Valid() || second.IsLocked() {
		t.Fatal("copied handle release did not invalidate the shared ownership state")
	}

	releaseErrors := make(chan error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			releaseErrors <- second.Release(context.Background(), secondHandle)
		}()
	}
	wg.Wait()
	close(releaseErrors)
	for err := range releaseErrors {
		if err != nil {
			t.Fatalf("concurrent Release: %v", err)
		}
	}
}

func TestEtcdLockAcquisitionContextDoesNotOwnLeaseLifetime(t *testing.T) {
	client := dialTestEtcd(t)
	lock := NewEtcdLock(client, contractKey(t, "context-lifetime"), LockOptions{Type: LockTypeExclusive, LeaseTTL: 2})
	ctx, cancel := context.WithCancel(context.Background())
	handle, err := lock.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	cancel()
	time.Sleep(50 * time.Millisecond)
	validationCtx, validationCancel := context.WithTimeout(context.Background(), time.Second)
	defer validationCancel()
	if err := lock.Validate(validationCtx, handle); err != nil {
		t.Fatalf("acquisition context canceled the lease: %v", err)
	}
	if err := lock.Release(validationCtx, handle); err != nil {
		t.Fatalf("Release: %v", err)
	}
}

func TestEtcdLockClientReconnectAfterLeaseExpiry(t *testing.T) {
	oldClient := dialTestEtcd(t)
	key := contractKey(t, "client-reconnect")
	oldLock := NewEtcdLock(oldClient, key, LockOptions{Type: LockTypeExclusive, LeaseTTL: 1})
	oldHandle, err := oldLock.Acquire(testCtx(t))
	if err != nil {
		t.Fatalf("old owner Acquire: %v", err)
	}
	if err := oldClient.Close(); err != nil {
		t.Fatalf("close old client: %v", err)
	}
	waitHandleDone(t, oldHandle)

	newClient := dialTestEtcd(t)
	newLock := NewEtcdLock(newClient, key, LockOptions{
		Type:          LockTypeExclusive,
		LeaseTTL:      2,
		RetryCount:    100,
		RetryInterval: 50,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	newHandle, err := newLock.Acquire(ctx)
	if err != nil {
		t.Fatalf("new owner Acquire after reconnect: %v", err)
	}
	if newHandle.FencingToken() <= oldHandle.FencingToken() {
		t.Fatalf("reconnected owner fence did not increase: old=%d new=%d", oldHandle.FencingToken(), newHandle.FencingToken())
	}
	if err := newLock.Validate(ctx, newHandle); err != nil {
		t.Fatalf("validate reconnected owner: %v", err)
	}
	if err := newLock.Release(ctx, newHandle); err != nil {
		t.Fatalf("release reconnected owner: %v", err)
	}
}

func TestEtcdLockRetryHonorsContext(t *testing.T) {
	client := dialTestEtcd(t)
	key := contractKey(t, "retry-context")
	owner := NewEtcdLock(client, key, LockOptions{Type: LockTypeExclusive, LeaseTTL: 5})
	ownerHandle, err := owner.Acquire(testCtx(t))
	if err != nil {
		t.Fatalf("owner Acquire: %v", err)
	}
	defer func() { _ = owner.Release(context.Background(), ownerHandle) }()

	contender := NewEtcdLock(client, key, LockOptions{
		Type:          LockTypeExclusive,
		LeaseTTL:      5,
		RetryCount:    100,
		RetryInterval: 50,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if _, err := contender.Acquire(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Acquire error = %v, want context deadline", err)
	}
}

func TestEtcdLockReleaseFailureCannotMaskConcurrentOwnershipLoss(t *testing.T) {
	lock := &EtcdLock{}
	handle := newLockHandle(lock, "/resource", "/resource/owner", 1, 1, nil)
	lock.current = handle

	handle.lifecycle.releaseIntent.Store(true)
	lock.observeOwnershipLoss(handle)
	if !handle.lifecycle.lossObserved.Load() {
		t.Fatal("release window did not retain the observed ownership loss")
	}

	lock.mu.Lock()
	lost := lock.finishFailedReleaseLocked(handle)
	lock.mu.Unlock()
	if !lost || handle.Valid() || !handle.Lost() || lock.IsLocked() {
		t.Fatalf("failed release left stale ownership: lost=%v valid=%v handleLost=%v locked=%v", lost, handle.Valid(), handle.Lost(), lock.IsLocked())
	}
	select {
	case <-handle.Done():
	default:
		t.Fatal("failed release did not close Done after concurrent ownership loss")
	}
}

func TestLockWatchdogDoesNotSilentlyReacquireOwnership(t *testing.T) {
	lock := &watchdogContractLock{}
	manager := &LockManager{locks: map[string]DistributedLock{"resource": lock}}
	watchdog := NewLockWatchdog(manager, WatchdogConfig{AutoRenew: true, MaxFailures: 1})

	watchdog.checkLocks(context.Background())
	if got := lock.lockCalls.Load(); got != 0 {
		t.Fatalf("watchdog silently reacquired ownership %d times", got)
	}
	if got := manager.GetLockCount(); got != 0 {
		t.Fatalf("lost lock remained managed: count=%d", got)
	}
}

func contractKey(t *testing.T, suffix string) string {
	t.Helper()
	name := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	return fmt.Sprintf("/zmmo-tests/zdistributed/%s/%d/%s", name, time.Now().UnixNano(), suffix)
}

func waitHandleDone(t *testing.T, handle *LockHandle) {
	t.Helper()
	select {
	case <-handle.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("lock handle did not observe lease loss")
	}
}

type watchdogContractLock struct {
	lockCalls atomic.Int64
}

func (l *watchdogContractLock) Lock(context.Context) error {
	l.lockCalls.Add(1)
	return nil
}

func (*watchdogContractLock) Unlock(context.Context) error { return nil }
func (*watchdogContractLock) IsLocked() bool               { return false }
func (*watchdogContractLock) GetKey() string               { return "resource" }
