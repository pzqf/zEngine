package zDistributed

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

func TestEtcdOwnershipStoreRejectsInvalidInputs(t *testing.T) {
	store := NewEtcdOwnershipStore(nil, OwnershipOptions{})
	if _, err := store.Acquire(context.Background(), "resource", "candidate", 0); !errors.Is(err, ErrInvalidOwnershipConfig) {
		t.Fatalf("Acquire with nil client error = %v, want ErrInvalidOwnershipConfig", err)
	}

	client := &clientv3.Client{}
	store = NewEtcdOwnershipStore(client, OwnershipOptions{Prefix: contractKey(t, "invalid")})
	for name, tc := range map[string]struct {
		resourceKey      string
		candidateToken   string
		expectedRevision int64
	}{
		"empty resource":  {candidateToken: "candidate"},
		"empty candidate": {resourceKey: "resource"},
		"negative revision": {
			resourceKey:      "resource",
			candidateToken:   "candidate",
			expectedRevision: -1,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := store.Acquire(context.Background(), tc.resourceKey, tc.candidateToken, tc.expectedRevision); !errors.Is(err, ErrInvalidOwnershipConfig) {
				t.Fatalf("Acquire error = %v, want ErrInvalidOwnershipConfig", err)
			}
		})
	}
}

func TestEtcdOwnershipStoreCASAndFencing(t *testing.T) {
	client := dialTestEtcd(t)
	ctx := testCtx(t)
	options := OwnershipOptions{Prefix: contractKey(t, "cas"), LeaseTTL: 3}
	firstStore := NewEtcdOwnershipStore(client, options)
	secondStore := NewEtcdOwnershipStore(client, options)
	resourceKey := "service-instance/map/0001/000101"
	protectedKey := contractKey(t, "protected")

	first, err := firstStore.Acquire(ctx, resourceKey, "process-a", 0)
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	if first.ResourceKey() != resourceKey || first.OwnerToken() != "process-a" || first.LeaseID() == 0 || first.Epoch() <= 0 || first.Revision() != first.Epoch() {
		t.Fatalf("invalid first handle: resource=%q owner=%q lease=%d epoch=%d revision=%d",
			first.ResourceKey(), first.OwnerToken(), first.LeaseID(), first.Epoch(), first.Revision())
	}

	record, found, err := firstStore.Get(ctx, resourceKey)
	if err != nil || !found {
		t.Fatalf("Get first owner: found=%v err=%v", found, err)
	}
	if record.OwnerToken != first.OwnerToken() || record.Epoch != first.Epoch() || record.Revision != first.Revision() || record.LeaseID != first.LeaseID() {
		t.Fatalf("record = %+v, handle owner=%q epoch=%d revision=%d lease=%d", record, first.OwnerToken(), first.Epoch(), first.Revision(), first.LeaseID())
	}
	if renewed, err := firstStore.Renew(ctx, first); err != nil || renewed != first {
		t.Fatalf("Renew current owner: handle=%p want=%p err=%v", renewed, first, err)
	}

	if _, err := secondStore.Acquire(ctx, resourceKey, "process-b", 0); !errors.Is(err, ErrOwnershipConflict) {
		t.Fatalf("competing Acquire error = %v, want ErrOwnershipConflict", err)
	}
	if _, err := secondStore.Acquire(ctx, resourceKey, "process-b", first.Revision()+1); !errors.Is(err, ErrOwnershipConflict) {
		t.Fatalf("wrong revision Acquire error = %v, want ErrOwnershipConflict", err)
	}
	if _, err := firstStore.GuardedTxn(ctx, first, clientv3.OpPut(protectedKey, "first")); err != nil {
		t.Fatalf("first GuardedTxn: %v", err)
	}

	second, err := secondStore.Acquire(ctx, resourceKey, "process-b", first.Revision())
	if err != nil {
		t.Fatalf("revision handoff Acquire: %v", err)
	}
	t.Cleanup(func() { _ = secondStore.Release(context.Background(), second) })
	if second.Epoch() <= first.Epoch() || second.Revision() <= first.Revision() {
		t.Fatalf("ownership epoch did not advance: first=%d/%d second=%d/%d", first.Epoch(), first.Revision(), second.Epoch(), second.Revision())
	}
	waitOwnershipDone(t, first)

	if _, err := firstStore.GuardedTxn(ctx, first, clientv3.OpPut(protectedKey, "stale")); !errors.Is(err, ErrOwnershipLost) {
		t.Fatalf("stale GuardedTxn error = %v, want ErrOwnershipLost", err)
	}
	if err := firstStore.Release(ctx, first); !errors.Is(err, ErrOwnershipLost) {
		t.Fatalf("stale Release error = %v, want ErrOwnershipLost", err)
	}
	if _, err := secondStore.GuardedTxn(ctx, second, clientv3.OpPut(protectedKey, "second")); err != nil {
		t.Fatalf("second GuardedTxn: %v", err)
	}
	response, err := client.Get(ctx, protectedKey)
	if err != nil {
		t.Fatalf("read protected key: %v", err)
	}
	if len(response.Kvs) != 1 || string(response.Kvs[0].Value) != "second" {
		t.Fatalf("protected value = %q, want second", response.Kvs[0].Value)
	}
}

func TestEtcdOwnershipStoreLeaseLossFencesOldEpoch(t *testing.T) {
	client := dialTestEtcd(t)
	ctx := testCtx(t)
	options := OwnershipOptions{Prefix: contractKey(t, "lease-loss"), LeaseTTL: 2}
	oldStore := NewEtcdOwnershipStore(client, options)
	newStore := NewEtcdOwnershipStore(client, options)
	resourceKey := "service-instance/game/0001/000101"
	protectedKey := contractKey(t, "lease-protected")

	oldHandle, err := oldStore.Acquire(ctx, resourceKey, "old-process", 0)
	if err != nil {
		t.Fatalf("old Acquire: %v", err)
	}
	if _, err := client.Revoke(ctx, oldHandle.LeaseID()); err != nil {
		t.Fatalf("revoke old lease: %v", err)
	}
	waitOwnershipDone(t, oldHandle)

	newHandle, err := newStore.Acquire(ctx, resourceKey, "new-process", 0)
	if err != nil {
		t.Fatalf("new Acquire: %v", err)
	}
	t.Cleanup(func() { _ = newStore.Release(context.Background(), newHandle) })
	if newHandle.Epoch() <= oldHandle.Epoch() {
		t.Fatalf("epoch did not advance after lease loss: old=%d new=%d", oldHandle.Epoch(), newHandle.Epoch())
	}
	if _, err := oldStore.Renew(ctx, oldHandle); !errors.Is(err, ErrOwnershipLost) {
		t.Fatalf("stale Renew error = %v, want ErrOwnershipLost", err)
	}
	if _, err := oldStore.GuardedTxn(ctx, oldHandle, clientv3.OpPut(protectedKey, "stale")); !errors.Is(err, ErrOwnershipLost) {
		t.Fatalf("stale GuardedTxn error = %v, want ErrOwnershipLost", err)
	}
	if _, err := newStore.GuardedTxn(ctx, newHandle, clientv3.OpPut(protectedKey, "current")); err != nil {
		t.Fatalf("new GuardedTxn: %v", err)
	}
}

func TestEtcdOwnershipStoreWatchPublishesMonotonicSnapshots(t *testing.T) {
	client := dialTestEtcd(t)
	options := OwnershipOptions{Prefix: contractKey(t, "watch"), LeaseTTL: 3}
	store := NewEtcdOwnershipStore(client, options)
	resourceKey := "service-instance/global/control/global-1"

	watchCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events, err := store.Watch(watchCtx, resourceKey)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	first, err := store.Acquire(testCtx(t), resourceKey, "process-a", 0)
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	firstEvent := waitOwnershipEvent(t, events, OwnershipEventAcquired)
	if firstEvent.Record.OwnerToken != "process-a" || firstEvent.Record.Revision != first.Revision() {
		t.Fatalf("first event = %+v, handle revision=%d", firstEvent, first.Revision())
	}

	second, err := store.Acquire(testCtx(t), resourceKey, "process-b", first.Revision())
	if err != nil {
		t.Fatalf("handoff Acquire: %v", err)
	}
	changed := waitOwnershipEvent(t, events, OwnershipEventChanged)
	if changed.Record.OwnerToken != "process-b" || changed.Revision <= firstEvent.Revision {
		t.Fatalf("changed event = %+v, first revision=%d", changed, firstEvent.Revision)
	}

	if err := store.Release(testCtx(t), second); err != nil {
		t.Fatalf("Release second: %v", err)
	}
	if err := store.Release(testCtx(t), second); err != nil {
		t.Fatalf("repeated Release second: %v", err)
	}
	released := waitOwnershipEvent(t, events, OwnershipEventReleased)
	if released.Record.OwnerToken != "process-b" || released.Revision <= changed.Revision {
		t.Fatalf("released event = %+v, changed revision=%d", released, changed.Revision)
	}
	if record, found, err := store.Get(testCtx(t), resourceKey); err != nil || found {
		t.Fatalf("Get after Release: record=%+v found=%v err=%v", record, found, err)
	}
}

func TestEtcdOwnershipStoreWatchRecoversAfterConnectionBreak(t *testing.T) {
	endpoint := strings.TrimSpace(os.Getenv("ZMMO_TEST_ETCD"))
	if endpoint == "" {
		t.Skip("ZMMO_TEST_ETCD is not set")
	}
	target := strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://")
	proxy := newOwnershipTCPProxy(t, target)

	proxyClient, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{proxy.Address()},
		DialTimeout: 3 * time.Second,
	})
	if err != nil {
		t.Fatalf("create proxy etcd client: %v", err)
	}
	t.Cleanup(func() { _ = proxyClient.Close() })
	directClient := dialTestEtcd(t)
	options := OwnershipOptions{Prefix: contractKey(t, "watch-reconnect"), LeaseTTL: 5}
	proxyStore := NewEtcdOwnershipStore(proxyClient, options)
	directStore := NewEtcdOwnershipStore(directClient, options)
	resourceKey := "service-instance/gateway/0001/000101"

	watchCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	events, err := proxyStore.Watch(watchCtx, resourceKey)
	if err != nil {
		t.Fatalf("Watch through proxy: %v", err)
	}
	first, err := proxyStore.Acquire(watchCtx, resourceKey, "process-a", 0)
	if err != nil {
		t.Fatalf("first Acquire through proxy: %v", err)
	}
	firstEvent := waitOwnershipEvent(t, events, OwnershipEventAcquired)
	if firstEvent.Revision != first.Revision() {
		t.Fatalf("first watch revision = %d, want %d", firstEvent.Revision, first.Revision())
	}

	if disconnected := proxy.Disconnect(); disconnected == 0 {
		t.Fatal("proxy did not have an active etcd connection to interrupt")
	}
	second, err := directStore.Acquire(watchCtx, resourceKey, "process-b", first.Revision())
	if err != nil {
		t.Fatalf("handoff during watch disconnect: %v", err)
	}
	t.Cleanup(func() { _ = directStore.Release(context.Background(), second) })

	changed := waitOwnershipEvent(t, events, OwnershipEventChanged)
	if changed.Record.OwnerToken != "process-b" || changed.Revision != second.Revision() || changed.Revision <= firstEvent.Revision {
		t.Fatalf("watch did not resume at the handoff revision: first=%+v changed=%+v second=%+v", firstEvent, changed, second.Record())
	}
}

func waitOwnershipDone(t *testing.T, handle *OwnershipHandle) {
	t.Helper()
	select {
	case <-handle.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("ownership handle did not observe ownership loss")
	}
}

func waitOwnershipEvent(t *testing.T, events <-chan OwnershipEvent, want OwnershipEventType) OwnershipEvent {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case event, ok := <-events:
			if !ok {
				t.Fatalf("ownership watch closed before %s", want)
			}
			if event.Type == want {
				return event
			}
		case <-deadline:
			t.Fatalf("timed out waiting for ownership event %s", want)
		}
	}
}

type ownershipTCPProxy struct {
	listener net.Listener
	target   string

	mu          sync.Mutex
	connections map[*ownershipProxyConnection]struct{}
}

type ownershipProxyConnection struct {
	client   net.Conn
	upstream net.Conn
}

func newOwnershipTCPProxy(t *testing.T, target string) *ownershipTCPProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for ownership test proxy: %v", err)
	}
	proxy := &ownershipTCPProxy{
		listener:    listener,
		target:      target,
		connections: make(map[*ownershipProxyConnection]struct{}),
	}
	go proxy.serve()
	t.Cleanup(proxy.Close)
	return proxy
}

func (p *ownershipTCPProxy) Address() string { return p.listener.Addr().String() }

func (p *ownershipTCPProxy) serve() {
	for {
		client, err := p.listener.Accept()
		if err != nil {
			return
		}
		go p.forward(client)
	}
}

func (p *ownershipTCPProxy) forward(client net.Conn) {
	upstream, err := net.DialTimeout("tcp", p.target, 3*time.Second)
	if err != nil {
		_ = client.Close()
		return
	}
	connection := &ownershipProxyConnection{client: client, upstream: upstream}
	p.mu.Lock()
	p.connections[connection] = struct{}{}
	p.mu.Unlock()

	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(upstream, client)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(client, upstream)
		done <- struct{}{}
	}()
	<-done
	_ = client.Close()
	_ = upstream.Close()
	<-done

	p.mu.Lock()
	delete(p.connections, connection)
	p.mu.Unlock()
}

func (p *ownershipTCPProxy) Disconnect() int {
	p.mu.Lock()
	connections := make([]*ownershipProxyConnection, 0, len(p.connections))
	for connection := range p.connections {
		connections = append(connections, connection)
	}
	p.mu.Unlock()
	for _, connection := range connections {
		_ = connection.client.Close()
		_ = connection.upstream.Close()
	}
	return len(connections)
}

func (p *ownershipTCPProxy) Close() {
	_ = p.listener.Close()
	p.Disconnect()
}
