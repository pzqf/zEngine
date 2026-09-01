package zServer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"

	"github.com/pzqf/zEngine/zSignal"
)

var (
	ErrStartAlreadyCalled = errors.New("server start already called")
	ErrRestartUnsupported = errors.New("server restart is unsupported; create a new server")
	ErrServerStopping     = errors.New("server stopped while starting")
)

// LifecycleHooks keeps the original application hook contract. Final cleanup
// belongs in RegisterCleanup; OnAfterStop is discovered through AfterStopHook
// so existing implementations remain source compatible.
type LifecycleHooks interface {
	OnBeforeStart() error
	OnAfterStart() error
	OnBeforeStop()
}

// AfterStopHook is an optional notification invoked after resource cleanup and
// before the terminal state is published.
type AfterStopHook interface {
	OnAfterStop()
}

func defaultSignalContext() (context.Context, context.CancelFunc) {
	return zSignal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// Start executes the application startup hooks once. A failure cancels the
// server context, rolls back all registered resources, and returns only after
// the server reaches Stopped.
func (s *BaseServer) Start() error {
	s.lifecycleMu.Lock()
	if s.startClaimed {
		s.lifecycleMu.Unlock()
		if s.GetState().IsTerminal() {
			return ErrRestartUnsupported
		}
		return ErrStartAlreadyCalled
	}
	if s.stopping.Load() || s.GetState().IsTerminal() {
		s.lifecycleMu.Unlock()
		return ErrRestartUnsupported
	}
	s.startClaimed = true
	s.lifecycleMu.Unlock()

	logger := s.logger
	if logger != nil {
		logger.Info("Starting %s Server...", s.ServerType)
	}

	startErr := callErrorHook("OnBeforeStart", s.hooks.OnBeforeStart)
	if startErr == nil {
		startErr = callErrorHook("OnAfterStart", s.hooks.OnAfterStart)
	}

	s.lifecycleMu.Lock()
	s.startSucceeded = startErr == nil
	stoppedDuringStart := s.stopping.Load()
	if startErr == nil && !stoppedDuringStart {
		s.isRunning.Store(true)
	}
	s.startComplete = true
	close(s.startDone)
	s.lifecycleMu.Unlock()

	if startErr != nil {
		if logger != nil {
			logger.Error("Server startup failed: %v", startErr)
		}
		return errors.Join(startErr, s.StopWithError())
	}
	if stoppedDuringStart {
		return errors.Join(ErrServerStopping, s.StopWithError())
	}

	if logger != nil {
		logger.Info("%s Server started successfully!", s.ServerType)
	}
	return nil
}

// Stop preserves the original compatibility API. It waits for the complete
// stop transaction; callers that need the aggregate result use StopWithError.
func (s *BaseServer) Stop() {
	_ = s.StopWithError()
}

// StopWithError performs one complete shutdown transaction. Concurrent and
// repeated callers wait for, and receive, the same aggregate result.
func (s *BaseServer) StopWithError() error {
	s.stopOnce.Do(func() {
		s.stopping.Store(true)

		s.lifecycleMu.Lock()
		if !s.startClaimed && !s.startComplete {
			s.startComplete = true
			close(s.startDone)
		}
		startDone := s.startDone
		s.lifecycleMu.Unlock()

		<-startDone
		s.stopErr = s.stopSequence()
		close(s.stopDone)
	})
	<-s.stopDone
	return s.stopErr
}

func (s *BaseServer) stopSequence() error {
	logger := s.logger
	if logger != nil {
		logger.Info("Stopping %s Server...", s.ServerType)
	}

	s.lifecycleMu.Lock()
	started := s.startSucceeded
	s.lifecycleMu.Unlock()

	var errs []error
	if started {
		if err := s.SetState(StateDraining, "server stopping"); err != nil {
			errs = append(errs, fmt.Errorf("set draining state: %w", err))
		}
		s.isRunning.Store(false)
		if err := callVoidHook("OnBeforeStop", s.hooks.OnBeforeStop); err != nil {
			errs = append(errs, err)
		}
	} else {
		s.isRunning.Store(false)
	}

	// Background owners observe cancellation before individual resources close.
	s.cancel()
	if err := s.runCleanups(); err != nil {
		errs = append(errs, err)
	}
	if hook, ok := s.hooks.(AfterStopHook); ok {
		if err := callVoidHook("OnAfterStop", hook.OnAfterStop); err != nil {
			errs = append(errs, err)
		}
	}
	if err := s.SetState(StateStopped, "server stopped"); err != nil {
		errs = append(errs, fmt.Errorf("set stopped state: %w", err))
	}

	if logger != nil {
		logger.Info("%s Server stopped", s.ServerType)
	}
	return errors.Join(errs...)
}

func callErrorHook(name string, hook func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%s panicked: %v", name, recovered)
		}
	}()
	if err := hook(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func callVoidHook(name string, hook func()) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%s panicked: %v", name, recovered)
		}
	}()
	hook()
	return nil
}

// Shutdown preserves the original compatibility API.
func (s *BaseServer) Shutdown() {
	_ = s.ShutdownWithError()
}

// ShutdownWithError is the externally callable checked form of Stop.
func (s *BaseServer) ShutdownWithError() error {
	if s.logger != nil {
		s.logger.Info("Shutdown requested")
	}
	return s.StopWithError()
}

// Run starts the server, waits for an OS signal or an external shutdown, then
// waits for the complete stop transaction. Its signal subscription is always
// released before returning.
func (s *BaseServer) Run() error {
	logger := s.GetLogger()
	if logger == nil {
		return fmt.Errorf("logger not initialized: must call SetLogger() before Run()")
	}
	if err := s.Start(); err != nil {
		return err
	}

	signalCtx, releaseSignals := s.signalContext()
	defer releaseSignals()
	select {
	case <-signalCtx.Done():
		logger.Info("Shutdown signal received, stopping server...")
	case <-s.ctx.Done():
		logger.Info("Server context canceled, waiting for shutdown...")
	}
	return s.StopWithError()
}

// Wait returns only after cleanup, the optional after-stop hook, and the final
// Stopped state have completed.
func (s *BaseServer) Wait() {
	<-s.stopDone
}
