package zProfiling

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestDisabledServerDoesNotListen(t *testing.T) {
	address := unusedAddress(t)
	server := NewServer(Config{ListenAddress: address})
	if err := server.Start(); err != nil {
		t.Fatalf("Start disabled server: %v", err)
	}
	if server.IsRunning() || server.ListenAddress() != "" {
		t.Fatalf("disabled state running=%t address=%q", server.IsRunning(), server.ListenAddress())
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("disabled server acquired listener: %v", err)
	}
	_ = listener.Close()
	if err := server.Close(); err != nil {
		t.Fatalf("Close disabled server: %v", err)
	}
}

func TestStartRejectsWildcardAddresses(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    error
	}{
		{name: "empty", address: "", want: ErrListenAddressRequired},
		{name: "empty host", address: ":6060", want: ErrListenAddressRequired},
		{name: "ipv4 wildcard", address: "0.0.0.0:6060", want: ErrWildcardAddress},
		{name: "ipv6 wildcard", address: "[::]:6060", want: ErrWildcardAddress},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := NewServer(Config{Enabled: true, ListenAddress: tc.address})
			if err := server.Start(); !errors.Is(err, tc.want) {
				t.Fatalf("Start error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestServerUsesPrivateMuxAndServesPprof(t *testing.T) {
	oldDefault := http.DefaultServeMux
	http.DefaultServeMux = http.NewServeMux()
	t.Cleanup(func() { http.DefaultServeMux = oldDefault })
	http.DefaultServeMux.HandleFunc("/debug/pprof/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	server := NewServer(Config{Enabled: true, ListenAddress: "127.0.0.1:0"})
	if err := server.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })
	if !server.IsRunning() || server.ListenAddress() == "" {
		t.Fatalf("started state running=%t address=%q", server.IsRunning(), server.ListenAddress())
	}

	response, err := http.Get("http://" + server.ListenAddress() + "/debug/pprof/")
	if err != nil {
		t.Fatalf("GET pprof index: %v", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("pprof status = %d, want 200; private mux may not be in use", response.StatusCode)
	}
}

func TestStartReportsListenConflict(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupy address: %v", err)
	}
	defer occupied.Close()

	server := NewServer(Config{Enabled: true, ListenAddress: occupied.Addr().String()})
	if err := server.Start(); !errors.Is(err, ErrListen) {
		t.Fatalf("Start error = %v, want ErrListen", err)
	}
	if server.IsRunning() {
		t.Fatal("server reports running after listen failure")
	}
}

func TestCloseIsConcurrentAndIdempotent(t *testing.T) {
	server := NewServer(Config{Enabled: true, ListenAddress: "127.0.0.1:0"})
	if err := server.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	address := server.ListenAddress()

	const callers = 32
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- server.Close()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	}
	if server.IsRunning() {
		t.Fatal("server still reports running after Close")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("listen on released address %s: %v", address, err)
	}
	_ = listener.Close()
	if err := server.Start(); !errors.Is(err, ErrServerClosed) {
		t.Fatalf("Start after Close error = %v, want ErrServerClosed", err)
	}
}

func TestCloseIsBounded(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	server := newServer(Config{
		Enabled:         true,
		ListenAddress:   "127.0.0.1:0",
		ShutdownTimeout: 25 * time.Millisecond,
	}, func(mux *http.ServeMux) {
		mux.HandleFunc("/block", func(http.ResponseWriter, *http.Request) {
			close(entered)
			<-release
		})
	})
	if err := server.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		response, err := http.Get("http://" + server.ListenAddress() + "/block")
		if err == nil {
			_ = response.Body.Close()
		}
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("blocking handler did not start")
	}

	started := time.Now()
	if err := server.Close(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close error = %v, want context deadline", err)
	}
	if elapsed := time.Since(started); elapsed < 20*time.Millisecond || elapsed > time.Second {
		t.Fatalf("Close took %v, want bounded shutdown", elapsed)
	}
	close(release)
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("blocked request did not exit")
	}
}

func unusedAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve address: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release reserved address: %v", err)
	}
	return address
}
