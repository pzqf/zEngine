package zNet

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

type backpressureMetricsStub struct {
	latestDropped  int
	bestRejected   int
	reliableReject int
}

func (m *backpressureMetricsStub) IncLatestFrameDropped()              { m.latestDropped++ }
func (m *backpressureMetricsStub) IncBestEffortEventRejected()         { m.bestRejected++ }
func (m *backpressureMetricsStub) IncReliableCommandRejected()         { m.reliableReject++ }
func (m *backpressureMetricsStub) RecordSendQueueWait(time.Duration)   {}
func (m *backpressureMetricsStub) AddSendQueueDepth(int)               {}
func (m *backpressureMetricsStub) AddSendQueueCapacity(int)            {}
func (m *backpressureMetricsStub) IncWorkerQueueRejected()             {}
func (m *backpressureMetricsStub) RecordWorkerQueueWait(time.Duration) {}

func TestOutboundQueueDeliveryClasses(t *testing.T) {
	metrics := &backpressureMetricsStub{}
	queue := newOutboundQueue(1, metrics)
	defer queue.Close(ErrSessionClosed)

	first := &outboundPacket{packet: &NetPacket{ProtoId: 10, Data: []byte("old")}}
	if err := queue.Enqueue(context.Background(), first, SendOptions{Class: LatestFrame, CoalesceKey: 7}); err != nil {
		t.Fatalf("enqueue latest: %v", err)
	}
	newest := &outboundPacket{packet: &NetPacket{ProtoId: 10, Data: []byte("new")}}
	if err := queue.Enqueue(context.Background(), newest, SendOptions{Class: LatestFrame, CoalesceKey: 7}); err != nil {
		t.Fatalf("replace latest: %v", err)
	}
	if metrics.latestDropped != 1 {
		t.Fatalf("latest drop metric = %d, want 1", metrics.latestDropped)
	}
	got, ok := queue.TryDequeue()
	if !ok || string(got.packet.Data) != "new" {
		t.Fatalf("dequeue latest = %#v, %v", got, ok)
	}

	if err := queue.Enqueue(context.Background(), first, SendOptions{Class: BestEffortEvent}); err != nil {
		t.Fatalf("enqueue best effort: %v", err)
	}
	if err := queue.Enqueue(context.Background(), newest, SendOptions{Class: BestEffortEvent}); !errors.Is(err, ErrSendQueueFull) {
		t.Fatalf("best effort full error = %v", err)
	}
	if metrics.bestRejected != 1 {
		t.Fatalf("best effort reject metric = %d, want 1", metrics.bestRejected)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := queue.Enqueue(ctx, newest, SendOptions{Class: ReliableCommand}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("reliable full error = %v, want deadline", err)
	}
	if metrics.reliableReject != 1 {
		t.Fatalf("reliable reject metric = %d, want 1", metrics.reliableReject)
	}
}

func TestOutboundQueueCloseFailsWaitingReliableCommand(t *testing.T) {
	queue := newOutboundQueue(1, nil)
	packet := &outboundPacket{packet: &NetPacket{ProtoId: 1}}
	if err := queue.Enqueue(context.Background(), packet, SendOptions{Class: BestEffortEvent}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- queue.Enqueue(context.Background(), packet, SendOptions{Class: ReliableCommand})
	}()
	time.Sleep(10 * time.Millisecond)
	queue.Close(ErrSessionClosed)
	select {
	case err := <-done:
		if !errors.Is(err, ErrSessionClosed) {
			t.Fatalf("waiting enqueue error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("waiting reliable command did not wake on close")
	}
}

func TestOutboundQueueConcurrentEnqueueAndClose(t *testing.T) {
	queue := newOutboundQueue(4, nil)
	const senders = 96
	start := make(chan struct{})
	errs := make(chan error, senders)
	var wg sync.WaitGroup
	for i := 0; i < senders; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			class := DeliveryClass(i%3 + 1)
			errs <- queue.Enqueue(ctx, &outboundPacket{packet: &NetPacket{ProtoId: ProtoIdType(i + 1)}},
				SendOptions{Class: class, CoalesceKey: uint64(i + 1)})
		}(i)
	}
	close(start)
	queue.Close(ErrSessionClosed)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil && !errors.Is(err, ErrSessionClosed) && !errors.Is(err, ErrSendQueueFull) {
			t.Fatalf("concurrent enqueue error = %v", err)
		}
	}
}

func TestTcpServerSessionSendContextReturnsAtDeadlineWhenQueueFull(t *testing.T) {
	server := NewTcpServer(&TcpConfig{
		ChanSize:          1,
		DisableEncryption: true,
		MaxWirePacketSize: 1024,
	})
	session := NewTcpServerSession(server, nil, 1, nil, nil)
	defer session.sendQueue.Close(ErrSessionClosed)

	if err := session.SendWithOptions(context.Background(), 100, []byte("first"), SendOptions{Class: BestEffortEvent}); err != nil {
		t.Fatalf("fill send queue: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := session.SendContext(ctx, 101, []byte("second"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("SendContext error = %v, want deadline", err)
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("SendContext exceeded deadline bound: %v", elapsed)
	}
}

func TestTcpServerSessionStoppedReaderBoundsAdmissionAndClose(t *testing.T) {
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen TCP: %v", err)
	}
	defer listener.Close()

	acceptedCh := make(chan *net.TCPConn, 1)
	acceptErrCh := make(chan error, 1)
	go func() {
		conn, acceptErr := listener.AcceptTCP()
		if acceptErr != nil {
			acceptErrCh <- acceptErr
			return
		}
		acceptedCh <- conn
	}()

	peer, err := net.DialTCP("tcp4", nil, listener.Addr().(*net.TCPAddr))
	if err != nil {
		t.Fatalf("dial TCP: %v", err)
	}
	defer peer.Close()
	if err := peer.SetReadBuffer(1024); err != nil {
		t.Fatalf("set peer read buffer: %v", err)
	}

	var serverConn *net.TCPConn
	select {
	case serverConn = <-acceptedCh:
	case acceptErr := <-acceptErrCh:
		t.Fatalf("accept TCP: %v", acceptErr)
	case <-time.After(time.Second):
		t.Fatal("accept TCP timed out")
	}
	if err := serverConn.SetWriteBuffer(1024); err != nil {
		t.Fatalf("set server write buffer: %v", err)
	}

	server := NewTcpServer(&TcpConfig{
		ChanSize:             1,
		DisableEncryption:    true,
		DisableCompression:   true,
		MaxWirePacketSize:    8 << 20,
		MaxDecodedPacketSize: 8 << 20,
	})
	session := NewTcpServerSession(server, serverConn, 1, nil, nil)
	session.Start()
	defer session.Close()

	payload := make([]byte, 4<<20)
	if err := session.SendWithOptions(context.Background(), 200, payload, SendOptions{Class: BestEffortEvent}); err != nil {
		t.Fatalf("start blocking write: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for queueDepth(session.sendQueue) != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if queueDepth(session.sendQueue) != 0 {
		t.Fatal("writer did not dequeue the first packet")
	}

	saturated := false
	for protoID := ProtoIdType(201); protoID < 217; protoID++ {
		err := session.SendWithOptions(context.Background(), protoID, payload, SendOptions{Class: BestEffortEvent})
		if errors.Is(err, ErrSendQueueFull) {
			saturated = true
			break
		}
		if err != nil {
			t.Fatalf("fill queue behind blocked writer: %v", err)
		}
		time.Sleep(25 * time.Millisecond)
		if queueDepth(session.sendQueue) == 1 {
			saturated = true
			break
		}
	}
	if !saturated {
		t.Fatal("could not saturate send queue while peer stopped reading")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	start := time.Now()
	err = session.SendContext(ctx, 202, []byte("must not block forever"))
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("SendContext error = %v, want deadline", err)
	}
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("SendContext exceeded deadline bound: %v", elapsed)
	}

	closed := make(chan struct{})
	go func() {
		session.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("session Close hung behind a blocked TCP writer")
	}
}

func queueDepth(queue *outboundQueue) int {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	return queue.items.Len()
}
