package zDistributed

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	clientv3 "go.etcd.io/etcd/client/v3"
)

var (
	ErrOwnershipConflict      = errors.New("ownership conflict")
	ErrOwnershipLost          = errors.New("ownership lost")
	ErrInvalidOwnershipConfig = errors.New("invalid ownership config")
)

// OwnershipStore manages one exclusive owner per opaque resource key. The
// application chooses resource keys and candidate tokens; the store only
// provides lease, revision, fencing, CAS, and watch mechanics.
type OwnershipStore interface {
	Acquire(ctx context.Context, resourceKey, candidateToken string, expectedRevision int64) (*OwnershipHandle, error)
	Renew(ctx context.Context, handle *OwnershipHandle) (*OwnershipHandle, error)
	Release(ctx context.Context, handle *OwnershipHandle) error
	Get(ctx context.Context, resourceKey string) (OwnershipRecord, bool, error)
	Watch(ctx context.Context, resourceKey string) (<-chan OwnershipEvent, error)
}

// FencedOwnershipStore protects etcd writes with the same owner, revision, and
// lease comparisons used by Acquire and Release.
type FencedOwnershipStore interface {
	OwnershipStore
	Validate(ctx context.Context, handle *OwnershipHandle) error
	GuardedTxn(ctx context.Context, handle *OwnershipHandle, operations ...clientv3.Op) (*clientv3.TxnResponse, error)
}

// OwnershipRecord is an immutable snapshot returned by Get and Watch. Epoch
// and Revision intentionally share the owner key ModRevision: every successful
// acquisition or explicit CAS handoff advances both monotonically.
type OwnershipRecord struct {
	ResourceKey string
	OwnerToken  string
	Epoch       int64
	LeaseID     clientv3.LeaseID
	Revision    int64
}

type OwnershipEventType string

const (
	OwnershipEventAcquired OwnershipEventType = "acquired"
	OwnershipEventChanged  OwnershipEventType = "changed"
	OwnershipEventReleased OwnershipEventType = "released"
)

// OwnershipEvent contains a value snapshot. Released events retain the last
// owner record while Revision identifies the deletion or reconciled revision.
type OwnershipEvent struct {
	Type     OwnershipEventType
	Record   OwnershipRecord
	Revision int64
}

type ownershipHandleState uint32

const (
	ownershipHandleActive ownershipHandleState = iota + 1
	ownershipHandleReleased
	ownershipHandleLost
)

// OwnershipHandle is the immutable fencing capability returned by Acquire.
// Copies share one lifecycle and become invalid together.
type OwnershipHandle struct {
	store     *EtcdOwnershipStore
	record    OwnershipRecord
	ownerKey  string
	lifecycle *ownershipHandleLifecycle
}

type ownershipHandleLifecycle struct {
	state         atomic.Uint32
	done          chan struct{}
	finishOnce    sync.Once
	releaseMu     sync.Mutex
	releaseIntent atomic.Bool
	lossObserved  atomic.Bool
	cancel        context.CancelFunc
}

func newOwnershipHandle(store *EtcdOwnershipStore, record OwnershipRecord, ownerKey string, cancel context.CancelFunc) *OwnershipHandle {
	handle := &OwnershipHandle{
		store:    store,
		record:   record,
		ownerKey: ownerKey,
		lifecycle: &ownershipHandleLifecycle{
			done:   make(chan struct{}),
			cancel: cancel,
		},
	}
	handle.lifecycle.state.Store(uint32(ownershipHandleActive))
	return handle
}

func (h *OwnershipHandle) ResourceKey() string {
	if h == nil {
		return ""
	}
	return h.record.ResourceKey
}

func (h *OwnershipHandle) OwnerToken() string {
	if h == nil {
		return ""
	}
	return h.record.OwnerToken
}

func (h *OwnershipHandle) Epoch() int64 {
	if h == nil {
		return 0
	}
	return h.record.Epoch
}

func (h *OwnershipHandle) LeaseID() clientv3.LeaseID {
	if h == nil {
		return clientv3.NoLease
	}
	return h.record.LeaseID
}

func (h *OwnershipHandle) Revision() int64 {
	if h == nil {
		return 0
	}
	return h.record.Revision
}

func (h *OwnershipHandle) Record() OwnershipRecord {
	if h == nil {
		return OwnershipRecord{}
	}
	return h.record
}

// Valid reports local ownership knowledge. Protected writes must still use
// GuardedTxn or equivalent comparisons to fence an unobserved remote loss.
func (h *OwnershipHandle) Valid() bool {
	return h != nil && h.lifecycle != nil && ownershipHandleState(h.lifecycle.state.Load()) == ownershipHandleActive
}

func (h *OwnershipHandle) Lost() bool {
	return h != nil && h.lifecycle != nil && ownershipHandleState(h.lifecycle.state.Load()) == ownershipHandleLost
}

func (h *OwnershipHandle) Done() <-chan struct{} {
	if h == nil || h.lifecycle == nil {
		return closedOwnershipHandleDone
	}
	return h.lifecycle.done
}

// OwnershipComparisons can be composed into an etcd transaction. The value,
// ModRevision, and lease must all still identify this exact owner epoch.
func (h *OwnershipHandle) OwnershipComparisons() ([]clientv3.Cmp, error) {
	if h == nil || !h.Valid() {
		return nil, ErrOwnershipLost
	}
	return h.ownershipComparisons(), nil
}

func (h *OwnershipHandle) ownershipComparisons() []clientv3.Cmp {
	return []clientv3.Cmp{
		clientv3.Compare(clientv3.Value(h.ownerKey), "=", h.record.OwnerToken),
		clientv3.Compare(clientv3.ModRevision(h.ownerKey), "=", h.record.Revision),
		clientv3.Compare(clientv3.LeaseValue(h.ownerKey), "=", int64(h.record.LeaseID)),
	}
}

func (h *OwnershipHandle) finish(state ownershipHandleState) {
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

var closedOwnershipHandleDone = func() <-chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}()
