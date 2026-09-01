package zServer

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrCleanupNameRequired       = errors.New("cleanup name is required")
	ErrCleanupFuncRequired       = errors.New("cleanup function is required")
	ErrCleanupAlreadyRegistered  = errors.New("cleanup already registered")
	ErrCleanupRegistrationClosed = errors.New("cleanup registration is closed")
)

// CleanupFunc releases one process-scoped resource. The function must be safe
// to call after the server context has been canceled.
type CleanupFunc func() error

type cleanupEntry struct {
	name string
	fn   CleanupFunc
}

// RegisterCleanup transfers ownership of a named process resource to the
// server. Registered functions run once in reverse order on both startup
// rollback and normal shutdown.
func (s *BaseServer) RegisterCleanup(name string, cleanup CleanupFunc) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrCleanupNameRequired
	}
	if cleanup == nil {
		return ErrCleanupFuncRequired
	}

	s.cleanupMu.Lock()
	defer s.cleanupMu.Unlock()
	if s.cleanupClosed {
		return ErrCleanupRegistrationClosed
	}
	if _, exists := s.cleanupNames[name]; exists {
		return fmt.Errorf("%w: %s", ErrCleanupAlreadyRegistered, name)
	}
	s.cleanupNames[name] = struct{}{}
	s.cleanups = append(s.cleanups, cleanupEntry{name: name, fn: cleanup})
	return nil
}

// StartBackground starts one process-owned task after first registering a
// cleanup barrier for it. Shutdown cancels the shared server context, then
// waits for the task to return before earlier resources are released.
func (s *BaseServer) StartBackground(name string, run func(context.Context)) error {
	if run == nil {
		return ErrCleanupFuncRequired
	}
	done := make(chan struct{})
	if err := s.RegisterCleanup("background:"+name, func() error {
		<-done
		return nil
	}); err != nil {
		return err
	}
	go func() {
		defer close(done)
		run(s.ctx)
	}()
	return nil
}

func (s *BaseServer) runCleanups() error {
	s.cleanupMu.Lock()
	s.cleanupClosed = true
	cleanups := append([]cleanupEntry(nil), s.cleanups...)
	s.cleanups = nil
	s.cleanupMu.Unlock()

	var errs []error
	for i := len(cleanups) - 1; i >= 0; i-- {
		if err := callCleanup(cleanups[i]); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func callCleanup(entry cleanupEntry) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("cleanup %q panicked: %v", entry.name, recovered)
		}
	}()
	if err := entry.fn(); err != nil {
		return fmt.Errorf("cleanup %q: %w", entry.name, err)
	}
	return nil
}
