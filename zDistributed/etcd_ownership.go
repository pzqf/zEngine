package zDistributed

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const defaultOwnershipPrefix = "/ownership/"

type OwnershipOptions struct {
	Prefix   string
	LeaseTTL int64
}

var DefaultOwnershipOptions = OwnershipOptions{
	Prefix:   defaultOwnershipPrefix,
	LeaseTTL: 30,
}

// EtcdOwnershipStore borrows its etcd client. Each acquired handle owns its
// lease and keepalive lifecycle until Release or ownership loss.
type EtcdOwnershipStore struct {
	client   *clientv3.Client
	prefix   string
	leaseTTL int64
}

var _ FencedOwnershipStore = (*EtcdOwnershipStore)(nil)

func NewEtcdOwnershipStore(client *clientv3.Client, options OwnershipOptions) *EtcdOwnershipStore {
	prefix := strings.TrimSpace(options.Prefix)
	if prefix == "" {
		prefix = DefaultOwnershipOptions.Prefix
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	leaseTTL := options.LeaseTTL
	if leaseTTL <= 0 {
		leaseTTL = DefaultOwnershipOptions.LeaseTTL
	}
	return &EtcdOwnershipStore{client: client, prefix: prefix, leaseTTL: leaseTTL}
}

// Acquire requires an absent resource when expectedRevision is zero. A
// positive expectedRevision performs an explicit CAS handoff from precisely
// that owner revision. It never silently reacquires after handle loss.
func (s *EtcdOwnershipStore) Acquire(ctx context.Context, resourceKey, candidateToken string, expectedRevision int64) (*OwnershipHandle, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.validateAcquire(resourceKey, candidateToken, expectedRevision); err != nil {
		return nil, err
	}

	leaseResponse, err := s.client.Grant(ctx, s.leaseTTL)
	if err != nil {
		return nil, fmt.Errorf("grant ownership lease: %w", err)
	}
	lifecycleCtx, cancel := context.WithCancel(context.Background())
	keepAlive, err := s.client.KeepAlive(lifecycleCtx, leaseResponse.ID)
	if err != nil {
		cancel()
		s.revokeLease(leaseResponse.ID)
		return nil, fmt.Errorf("start ownership keepalive: %w", err)
	}

	ownerKey := s.ownerKey(resourceKey)
	comparison := clientv3.Compare(clientv3.Version(ownerKey), "=", 0)
	if expectedRevision > 0 {
		comparison = clientv3.Compare(clientv3.ModRevision(ownerKey), "=", expectedRevision)
	}
	response, err := s.client.Txn(ctx).
		If(comparison).
		Then(clientv3.OpPut(ownerKey, candidateToken, clientv3.WithLease(leaseResponse.ID))).
		Commit()
	if err != nil {
		cancel()
		s.revokeLease(leaseResponse.ID)
		return nil, fmt.Errorf("acquire ownership: %w", err)
	}
	if !response.Succeeded {
		cancel()
		s.revokeLease(leaseResponse.ID)
		return nil, fmt.Errorf("%w: resource %q expected revision %d", ErrOwnershipConflict, resourceKey, expectedRevision)
	}

	getResponse, err := s.client.Get(ctx, ownerKey)
	if err != nil {
		cancel()
		s.revokeLease(leaseResponse.ID)
		return nil, fmt.Errorf("read acquired ownership: %w", err)
	}
	if len(getResponse.Kvs) != 1 {
		cancel()
		s.revokeLease(leaseResponse.ID)
		return nil, ErrOwnershipLost
	}
	kv := getResponse.Kvs[0]
	if string(kv.Value) != candidateToken || kv.Lease != int64(leaseResponse.ID) || kv.ModRevision <= 0 {
		cancel()
		s.revokeLease(leaseResponse.ID)
		return nil, ErrOwnershipLost
	}
	record := ownershipRecordFromKV(resourceKey, kv)
	handle := newOwnershipHandle(s, record, ownerKey, cancel)
	go s.monitorOwnership(lifecycleCtx, handle, keepAlive)

	select {
	case <-handle.Done():
		return nil, ErrOwnershipLost
	default:
		return handle, nil
	}
}

// Renew explicitly confirms the lease and owner record. The background
// keepalive remains owned by the handle; callers use Renew as a fail-closed
// freshness boundary before protected periodic work.
func (s *EtcdOwnershipStore) Renew(ctx context.Context, handle *OwnershipHandle) (*OwnershipHandle, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.validateHandle(handle); err != nil {
		return nil, err
	}
	handle.lifecycle.releaseMu.Lock()
	defer handle.lifecycle.releaseMu.Unlock()
	if !handle.Valid() {
		return nil, ErrOwnershipLost
	}
	response, err := s.client.KeepAliveOnce(ctx, handle.record.LeaseID)
	if err != nil {
		return nil, fmt.Errorf("renew ownership lease: %w", err)
	}
	if response == nil || response.TTL <= 0 {
		s.markLost(handle)
		return nil, ErrOwnershipLost
	}
	if err := s.validateRemote(ctx, handle); err != nil {
		return nil, err
	}
	return handle, nil
}

func (s *EtcdOwnershipStore) Validate(ctx context.Context, handle *OwnershipHandle) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.validateHandle(handle); err != nil {
		return err
	}
	return s.validateRemote(ctx, handle)
}

func (s *EtcdOwnershipStore) validateRemote(ctx context.Context, handle *OwnershipHandle) error {
	response, err := s.client.Txn(ctx).
		If(handle.ownershipComparisons()...).
		Then(clientv3.OpGet(handle.ownerKey)).
		Commit()
	if err != nil {
		return fmt.Errorf("validate ownership: %w", err)
	}
	if !response.Succeeded {
		s.markLost(handle)
		return ErrOwnershipLost
	}
	return nil
}

func (s *EtcdOwnershipStore) GuardedTxn(ctx context.Context, handle *OwnershipHandle, operations ...clientv3.Op) (*clientv3.TxnResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.validateHandle(handle); err != nil {
		return nil, err
	}
	response, err := s.client.Txn(ctx).
		If(handle.ownershipComparisons()...).
		Then(operations...).
		Commit()
	if err != nil {
		return nil, fmt.Errorf("guarded ownership transaction: %w", err)
	}
	if !response.Succeeded {
		s.markLost(handle)
		return response, ErrOwnershipLost
	}
	return response, nil
}

func (s *EtcdOwnershipStore) Release(ctx context.Context, handle *OwnershipHandle) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if handle == nil || handle.store != s || handle.lifecycle == nil {
		return fmt.Errorf("%w: handle does not belong to store", ErrInvalidOwnershipConfig)
	}
	handle.lifecycle.releaseMu.Lock()
	defer handle.lifecycle.releaseMu.Unlock()

	state := ownershipHandleState(handle.lifecycle.state.Load())
	if state == ownershipHandleReleased {
		return nil
	}
	if state != ownershipHandleActive {
		return ErrOwnershipLost
	}

	handle.lifecycle.releaseIntent.Store(true)
	response, err := s.client.Txn(ctx).
		If(handle.ownershipComparisons()...).
		Then(clientv3.OpDelete(handle.ownerKey)).
		Commit()
	if err != nil {
		releaseErr := fmt.Errorf("release ownership: %w", err)
		handle.lifecycle.releaseIntent.Store(false)
		if handle.lifecycle.lossObserved.Load() {
			s.markLost(handle)
			return errors.Join(releaseErr, ErrOwnershipLost)
		}
		return releaseErr
	}
	if !response.Succeeded {
		s.markLost(handle)
		return ErrOwnershipLost
	}

	handle.finish(ownershipHandleReleased)
	s.revokeLease(handle.record.LeaseID)
	return nil
}

func (s *EtcdOwnershipStore) Get(ctx context.Context, resourceKey string) (OwnershipRecord, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	record, found, _, err := s.getSnapshot(ctx, resourceKey)
	return record, found, err
}

// Watch first emits the current owner, if any, and then every owner change.
// On a canceled or closed etcd watch it reconciles with Get and resumes from a
// later revision. Events are never dropped; caller cancellation provides the
// backpressure escape hatch.
func (s *EtcdOwnershipStore) Watch(ctx context.Context, resourceKey string) (<-chan OwnershipEvent, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.validateResource(resourceKey); err != nil {
		return nil, err
	}
	record, found, headerRevision, err := s.getSnapshot(ctx, resourceKey)
	if err != nil {
		return nil, err
	}
	events := make(chan OwnershipEvent, 16)
	go s.watchLoop(ctx, resourceKey, record, found, headerRevision, events)
	return events, nil
}

func (s *EtcdOwnershipStore) watchLoop(ctx context.Context, resourceKey string, record OwnershipRecord, found bool, headerRevision int64, output chan<- OwnershipEvent) {
	defer close(output)
	lastRecord := record
	present := found
	lastEventRevision := int64(0)
	if found {
		initial := OwnershipEvent{Type: OwnershipEventAcquired, Record: record, Revision: record.Revision}
		if !sendOwnershipEvent(ctx, output, initial) {
			return
		}
		lastEventRevision = record.Revision
	}
	nextRevision := headerRevision + 1
	if nextRevision <= 0 {
		nextRevision = 1
	}
	ownerKey := s.ownerKey(resourceKey)

	for {
		watch := s.client.Watch(ctx, ownerKey, clientv3.WithRev(nextRevision), clientv3.WithPrevKV())
		watchEnded := false
		for !watchEnded {
			select {
			case <-ctx.Done():
				return
			case response, ok := <-watch:
				if !ok || response.Canceled || response.Err() != nil {
					watchEnded = true
					continue
				}
				if response.Header.Revision >= nextRevision {
					nextRevision = response.Header.Revision + 1
				}
				for _, event := range response.Events {
					ownershipEvent, ok := ownershipEventFromEtcd(resourceKey, event)
					if !ok || ownershipEvent.Revision <= lastEventRevision {
						continue
					}
					if !sendOwnershipEvent(ctx, output, ownershipEvent) {
						return
					}
					lastEventRevision = ownershipEvent.Revision
					nextRevision = ownershipEvent.Revision + 1
					if ownershipEvent.Type == OwnershipEventReleased {
						present = false
					} else {
						present = true
						lastRecord = ownershipEvent.Record
					}
				}
			}
		}

		for {
			if !waitOwnershipRetry(ctx, 50*time.Millisecond) {
				return
			}
			current, currentFound, revision, err := s.getSnapshot(ctx, resourceKey)
			if err != nil {
				continue
			}
			if currentFound && current.Revision > lastEventRevision {
				eventType := OwnershipEventAcquired
				if present {
					eventType = OwnershipEventChanged
				}
				event := OwnershipEvent{Type: eventType, Record: current, Revision: current.Revision}
				if !sendOwnershipEvent(ctx, output, event) {
					return
				}
				lastEventRevision = current.Revision
				lastRecord = current
				present = true
			} else if !currentFound && present && revision > lastEventRevision {
				event := OwnershipEvent{Type: OwnershipEventReleased, Record: lastRecord, Revision: revision}
				if !sendOwnershipEvent(ctx, output, event) {
					return
				}
				lastEventRevision = revision
				present = false
			}
			if revision >= lastEventRevision {
				nextRevision = revision + 1
			} else {
				nextRevision = lastEventRevision + 1
			}
			break
		}
	}
}

func (s *EtcdOwnershipStore) getSnapshot(ctx context.Context, resourceKey string) (OwnershipRecord, bool, int64, error) {
	if err := s.validateResource(resourceKey); err != nil {
		return OwnershipRecord{}, false, 0, err
	}
	response, err := s.client.Get(ctx, s.ownerKey(resourceKey))
	if err != nil {
		return OwnershipRecord{}, false, 0, fmt.Errorf("get ownership: %w", err)
	}
	if len(response.Kvs) == 0 {
		return OwnershipRecord{}, false, response.Header.Revision, nil
	}
	return ownershipRecordFromKV(resourceKey, response.Kvs[0]), true, response.Header.Revision, nil
}

func (s *EtcdOwnershipStore) monitorOwnership(ctx context.Context, handle *OwnershipHandle, keepAlive <-chan *clientv3.LeaseKeepAliveResponse) {
	watch := s.client.Watch(ctx, handle.ownerKey, clientv3.WithRev(handle.record.Revision+1))
	for {
		select {
		case <-handle.Done():
			return
		case _, ok := <-keepAlive:
			if !ok {
				s.observeOwnershipLoss(handle)
				return
			}
		case response, ok := <-watch:
			if !ok || response.Canceled || response.Err() != nil {
				s.observeOwnershipLoss(handle)
				return
			}
			for _, event := range response.Events {
				if event.Type == clientv3.EventTypeDelete ||
					event.Kv.ModRevision != handle.record.Revision ||
					string(event.Kv.Value) != handle.record.OwnerToken ||
					event.Kv.Lease != int64(handle.record.LeaseID) {
					s.observeOwnershipLoss(handle)
					return
				}
			}
		}
	}
}

func (s *EtcdOwnershipStore) observeOwnershipLoss(handle *OwnershipHandle) {
	handle.lifecycle.lossObserved.Store(true)
	if !handle.lifecycle.releaseIntent.Load() {
		s.markLost(handle)
	}
}

func (s *EtcdOwnershipStore) markLost(handle *OwnershipHandle) {
	if handle != nil {
		handle.finish(ownershipHandleLost)
	}
}

func (s *EtcdOwnershipStore) validateAcquire(resourceKey, candidateToken string, expectedRevision int64) error {
	if err := s.validateResource(resourceKey); err != nil {
		return err
	}
	if strings.TrimSpace(candidateToken) == "" {
		return fmt.Errorf("%w: empty candidate token", ErrInvalidOwnershipConfig)
	}
	if expectedRevision < 0 {
		return fmt.Errorf("%w: negative expected revision", ErrInvalidOwnershipConfig)
	}
	return nil
}

func (s *EtcdOwnershipStore) validateResource(resourceKey string) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("%w: nil etcd client", ErrInvalidOwnershipConfig)
	}
	if strings.TrimSpace(s.prefix) == "" || s.leaseTTL <= 0 {
		return fmt.Errorf("%w: invalid store options", ErrInvalidOwnershipConfig)
	}
	if strings.TrimSpace(resourceKey) == "" {
		return fmt.Errorf("%w: empty resource key", ErrInvalidOwnershipConfig)
	}
	return nil
}

func (s *EtcdOwnershipStore) validateHandle(handle *OwnershipHandle) error {
	if s == nil || handle == nil || handle.store != s || handle.lifecycle == nil {
		return fmt.Errorf("%w: handle does not belong to store", ErrInvalidOwnershipConfig)
	}
	if !handle.Valid() {
		return ErrOwnershipLost
	}
	return nil
}

func (s *EtcdOwnershipStore) ownerKey(resourceKey string) string {
	encoded := base64.RawURLEncoding.EncodeToString([]byte(resourceKey))
	return s.prefix + encoded
}

func (s *EtcdOwnershipStore) revokeLease(leaseID clientv3.LeaseID) {
	if s == nil || s.client == nil || leaseID == clientv3.NoLease {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = s.client.Revoke(ctx, leaseID)
}

func ownershipRecordFromKV(resourceKey string, kv *mvccpb.KeyValue) OwnershipRecord {
	if kv == nil {
		return OwnershipRecord{ResourceKey: resourceKey}
	}
	return OwnershipRecord{
		ResourceKey: resourceKey,
		OwnerToken:  string(kv.Value),
		Epoch:       kv.ModRevision,
		LeaseID:     clientv3.LeaseID(kv.Lease),
		Revision:    kv.ModRevision,
	}
}

func ownershipEventFromEtcd(resourceKey string, event *clientv3.Event) (OwnershipEvent, bool) {
	if event == nil || event.Kv == nil {
		return OwnershipEvent{}, false
	}
	if event.Type == clientv3.EventTypeDelete {
		record := ownershipRecordFromKV(resourceKey, event.PrevKv)
		return OwnershipEvent{Type: OwnershipEventReleased, Record: record, Revision: event.Kv.ModRevision}, true
	}
	record := ownershipRecordFromKV(resourceKey, event.Kv)
	eventType := OwnershipEventChanged
	if event.PrevKv == nil || event.PrevKv.Version == 0 {
		eventType = OwnershipEventAcquired
	}
	return OwnershipEvent{Type: eventType, Record: record, Revision: record.Revision}, true
}

func sendOwnershipEvent(ctx context.Context, output chan<- OwnershipEvent, event OwnershipEvent) bool {
	select {
	case <-ctx.Done():
		return false
	case output <- event:
		return true
	}
}

func waitOwnershipRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
