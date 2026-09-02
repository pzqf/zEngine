package zNet

import (
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func startAdmissionTestServer(t *testing.T) (*TcpServer, string, *atomic.Int32) {
	t.Helper()
	server := NewTcpServer(&TcpConfig{
		ListenAddress:     "127.0.0.1:0",
		ChanSize:          8,
		UseWorkerPool:     true,
		WorkerPoolSize:    2,
		WorkerQueueSize:   8,
		DisableEncryption: true,
	})
	removed := &atomic.Int32{}
	server.SetOnRemoveSession(func(SessionIdType) {
		removed.Add(1)
	})
	if err := server.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(server.Close)
	return server, server.GetListenAddress(), removed
}

func waitForServerSessions(t *testing.T, server *TcpServer, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(server.GetAllSession()) == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("session count=%d, want %d", len(server.GetAllSession()), want)
}

func TestTcpServerCloseAdmissionKeepsExistingSession(t *testing.T) {
	server, address, removed := startAdmissionTestServer(t)
	received := make(chan ProtoIdType, 1)
	server.RegisterDispatcher(func(_ Session, packet *NetPacket) error {
		received <- packet.ProtoId
		return nil
	})
	client, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		t.Fatalf("Dial existing client: %v", err)
	}
	defer client.Close()
	waitForServerSessions(t, server, 1)

	if err := server.CloseAdmission(); err != nil {
		t.Fatalf("CloseAdmission: %v", err)
	}
	if got := len(server.GetAllSession()); got != 1 {
		t.Fatalf("CloseAdmission closed existing sessions: got %d", got)
	}
	if got := removed.Load(); got != 0 {
		t.Fatalf("remove callbacks after CloseAdmission=%d, want 0", got)
	}
	if rejected, dialErr := net.DialTimeout("tcp", address, 100*time.Millisecond); dialErr == nil {
		rejected.Close()
		t.Fatal("new connection succeeded after CloseAdmission")
	}
	request := server.packetCodec.Marshal(&NetPacket{ProtoId: 76, Data: []byte("request"), DataSize: 7})
	if _, err := client.Write(request); err != nil {
		t.Fatalf("write on admitted session: %v", err)
	}
	select {
	case protoID := <-received:
		if protoID != 76 {
			t.Fatalf("dispatcher proto=%d, want 76", protoID)
		}
	case <-time.After(time.Second):
		t.Fatal("existing session stopped dispatching after CloseAdmission")
	}

	session := server.GetAllSession()[0]
	if err := session.Send(77, []byte("still-open")); err != nil {
		t.Fatalf("send on admitted session: %v", err)
	}
	if err := client.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	one := make([]byte, 1)
	if _, err := client.Read(one); err != nil {
		t.Fatalf("existing client did not remain usable: %v", err)
	}

	if err := client.Close(); err != nil {
		t.Fatalf("close client: %v", err)
	}
	waitForServerSessions(t, server, 0)
	if got := removed.Load(); got != 1 {
		t.Fatalf("remove callbacks=%d, want 1", got)
	}
}

func TestTcpServerAdmissionAndCloseAreConcurrentIdempotent(t *testing.T) {
	server, address, removed := startAdmissionTestServer(t)
	client, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()
	waitForServerSessions(t, server, 1)

	var admissionWG sync.WaitGroup
	for i := 0; i < 16; i++ {
		admissionWG.Add(1)
		go func() {
			defer admissionWG.Done()
			if err := server.CloseAdmission(); err != nil {
				t.Errorf("CloseAdmission: %v", err)
			}
		}()
	}
	admissionWG.Wait()
	if got := len(server.GetAllSession()); got != 1 {
		t.Fatalf("sessions after concurrent CloseAdmission=%d, want 1", got)
	}

	var closeWG sync.WaitGroup
	for i := 0; i < 16; i++ {
		closeWG.Add(1)
		go func() {
			defer closeWG.Done()
			server.Close()
		}()
	}
	closeWG.Wait()
	server.Close()

	if got := len(server.GetAllSession()); got != 0 {
		t.Fatalf("sessions after Close=%d, want 0", got)
	}
	if got := removed.Load(); got != 1 {
		t.Fatalf("remove callbacks=%d, want exactly 1", got)
	}
	if err := client.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	if _, err := client.Read(make([]byte, 1)); err == nil {
		t.Fatal("client connection remained open after Close")
	}
}
