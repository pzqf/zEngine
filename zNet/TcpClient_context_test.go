package zNet

import (
	"context"
	"errors"
	"net"
	"strconv"
	"testing"
	"time"
)

func TestTcpClientConnectContextCancelsStalledHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	accepted := make(chan struct{})
	release := make(chan struct{})
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		close(accepted)
		defer conn.Close()
		<-release
	}()
	defer close(release)

	host, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	client := NewTcpClient(&TcpClientConfig{ServerAddr: host, ServerPort: port})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	err = client.ConnectContext(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ConnectContext error=%v, want context deadline", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("ConnectContext ignored deadline for %s", elapsed)
	}
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("test listener never accepted the connection")
	}
	if client.GetState() != ClientStateDisconnected || client.GetSession() != nil {
		t.Fatalf("client state=%v session=%v after canceled handshake", client.GetState(), client.GetSession())
	}
}
