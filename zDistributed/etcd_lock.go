package zDistributed

import (
	"context"
	"errors"
	"sync/atomic"

	clientv3 "go.etcd.io/etcd/client/v3"
	"github.com/pzqf/zEngine/zLog"
	"go.uber.org/zap"
)

type EtcdLock struct {
	client     *clientv3.Client
	lease      clientv3.LeaseID
	lockKey    string
	isLocked   atomic.Bool
	cancelFunc context.CancelFunc
	options    LockOptions
}

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

func (l *EtcdLock) Lock(ctx context.Context) error {
	if l.isLocked.Load() {
		return nil
	}

	leaseResp, err := l.client.Grant(ctx, l.options.LeaseTTL)
	if err != nil {
		return err
	}
	l.lease = leaseResp.ID

	ctxWithCancel, cancel := context.WithCancel(ctx)
	l.cancelFunc = cancel

	go l.keepAlive(ctxWithCancel)

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

func (l *EtcdLock) LockWithAutoRenew(ctx context.Context) error {
	if l.isLocked.Load() {
		return nil
	}

	leaseResp, err := l.client.Grant(ctx, l.options.LeaseTTL)
	if err != nil {
		return err
	}
	l.lease = leaseResp.ID

	ctxWithCancel, cancel := context.WithCancel(ctx)
	l.cancelFunc = cancel

	keepAliveCh, err := l.client.KeepAlive(ctxWithCancel, l.lease)
	if err != nil {
		cancel()
		return err
	}

	go func() {
		for {
			select {
			case <-ctxWithCancel.Done():
				return
			case resp, ok := <-keepAliveCh:
				if !ok {
					zLog.Warn("Lock keep-alive channel closed, lease may have expired",
						zap.String("key", l.lockKey))
					l.isLocked.Store(false)
					return
				}
				if resp == nil {
					zLog.Warn("Lock keep-alive received nil response",
						zap.String("key", l.lockKey))
					l.isLocked.Store(false)
					return
				}
			}
		}
	}()

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

func (l *EtcdLock) Unlock(ctx context.Context) error {
	if !l.isLocked.Load() {
		return nil
	}

	if l.cancelFunc != nil {
		l.cancelFunc()
	}

	_, err := l.client.Delete(ctx, l.lockKey)
	if err != nil {
		return err
	}

	l.isLocked.Store(false)
	return nil
}

func (l *EtcdLock) IsLocked() bool {
	return l.isLocked.Load()
}

func (l *EtcdLock) GetKey() string {
	return l.lockKey
}

func (l *EtcdLock) keepAlive(ctx context.Context) {
	ch, err := l.client.KeepAlive(ctx, l.lease)
	if err != nil {
		zLog.Error("Lock keep-alive failed",
			zap.String("key", l.lockKey),
			zap.Error(err))
		l.isLocked.Store(false)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case resp, ok := <-ch:
			if !ok {
				zLog.Warn("Lock keep-alive channel closed",
					zap.String("key", l.lockKey))
				l.isLocked.Store(false)
				return
			}
			if resp == nil {
				zLog.Warn("Lock keep-alive received nil response",
					zap.String("key", l.lockKey))
				l.isLocked.Store(false)
				return
			}
		}
	}
}
