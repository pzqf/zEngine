// Package zProfiling provides an explicitly owned HTTP server for Go runtime
// profiling endpoints.
package zProfiling

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"strings"
	"sync"
	"time"
)

const defaultShutdownTimeout = 5 * time.Second

var (
	ErrListenAddressRequired = errors.New("profiling listen address is required")
	ErrWildcardAddress       = errors.New("profiling wildcard listen address is not allowed")
	ErrAlreadyStarted        = errors.New("profiling server already started")
	ErrServerClosed          = errors.New("profiling server is closed")
	ErrListen                = errors.New("profiling listen failed")
)

// Config controls one process-owned profiling listener. Disabled is the safe
// zero value and never acquires a listener.
type Config struct {
	Enabled         bool
	ListenAddress   string
	ShutdownTimeout time.Duration
}

// Server owns one private profiling HTTP mux and listener.
type Server struct {
	config Config
	setup  func(*http.ServeMux)

	mu           sync.RWMutex
	listener     net.Listener
	httpServer   *http.Server
	boundAddress string
	serveDone    chan struct{}
	serveErr     error
	started      bool
	serving      bool
	closed       bool

	closeOnce sync.Once
	closeErr  error
}

// NewServer constructs a dormant profiling server. Start owns resource
// acquisition so callers can register Close immediately after Start succeeds.
func NewServer(config Config) *Server {
	return newServer(config, registerPprof)
}

func newServer(config Config, setup func(*http.ServeMux)) *Server {
	if config.ShutdownTimeout <= 0 {
		config.ShutdownTimeout = defaultShutdownTimeout
	}
	return &Server{config: config, setup: setup}
}

// Start synchronously acquires the configured listener before returning.
func (s *Server) Start() error {
	if s == nil {
		return ErrServerClosed
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrServerClosed
	}
	if !s.config.Enabled {
		return nil
	}
	if s.started {
		return ErrAlreadyStarted
	}
	address := strings.TrimSpace(s.config.ListenAddress)
	if err := validateListenAddress(address); err != nil {
		return err
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("%w on %s: %v", ErrListen, address, err)
	}
	mux := http.NewServeMux()
	s.setup(mux)
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	s.listener = listener
	s.httpServer = server
	s.boundAddress = listener.Addr().String()
	s.serveDone = make(chan struct{})
	s.started = true
	s.serving = true
	go s.serve(server, listener, s.serveDone)
	return nil
}

func validateListenAddress(address string) error {
	if address == "" {
		return ErrListenAddressRequired
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid profiling listen address %q: %w", address, err)
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return ErrListenAddressRequired
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		return fmt.Errorf("%w: %s", ErrWildcardAddress, address)
	}
	return nil
}

func registerPprof(mux *http.ServeMux) {
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
}

func (s *Server) serve(server *http.Server, listener net.Listener, done chan struct{}) {
	defer close(done)
	err := server.Serve(listener)
	s.mu.Lock()
	s.serving = false
	if err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
		s.serveErr = fmt.Errorf("profiling HTTP serve: %w", err)
	}
	s.mu.Unlock()
}

// Close releases the listener once. Shutdown is bounded and falls back to a
// force close before reporting the accumulated error.
func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.closeErr = s.close()
	})
	return s.closeErr
}

func (s *Server) close() error {
	s.mu.Lock()
	s.closed = true
	server := s.httpServer
	done := s.serveDone
	timeout := s.config.ShutdownTimeout
	s.mu.Unlock()

	if server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	shutdownErr := server.Shutdown(ctx)
	if shutdownErr != nil {
		shutdownErr = errors.Join(shutdownErr, server.Close())
	}
	if done != nil {
		<-done
	}

	s.mu.Lock()
	serveErr := s.serveErr
	s.mu.Unlock()
	return errors.Join(shutdownErr, serveErr)
}

// ListenAddress returns the actual bound address after Start succeeds.
func (s *Server) ListenAddress() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.boundAddress
}

// IsRunning reports whether Start acquired a listener that Close has not yet
// claimed. It is an ownership signal, not an HTTP health check.
func (s *Server) IsRunning() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.started && s.serving && !s.closed
}
