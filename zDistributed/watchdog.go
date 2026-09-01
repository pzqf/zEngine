package zDistributed

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pzqf/zEngine/zLog"
	"go.uber.org/zap"
)

type WatchdogConfig struct {
	CheckInterval time.Duration
	// AutoRenew is retained for source compatibility and intentionally ignored.
	// Ownership loss requires an explicit Acquire so the caller receives the
	// new fencing token.
	// Deprecated: lease renewal is owned by EtcdLock's session.
	AutoRenew   bool
	MaxFailures int
}

func DefaultWatchdogConfig() WatchdogConfig {
	return WatchdogConfig{
		CheckInterval: 5 * time.Second,
		AutoRenew:     false,
		MaxFailures:   3,
	}
}

type LockWatchdog struct {
	manager  *LockManager
	config   WatchdogConfig
	running  atomic.Bool
	failures map[string]int
	mu       sync.Mutex
	cancel   context.CancelFunc
}

func NewLockWatchdog(manager *LockManager, config WatchdogConfig) *LockWatchdog {
	if config.CheckInterval <= 0 {
		config.CheckInterval = DefaultWatchdogConfig().CheckInterval
	}
	if config.MaxFailures <= 0 {
		config.MaxFailures = DefaultWatchdogConfig().MaxFailures
	}
	return &LockWatchdog{
		manager:  manager,
		config:   config,
		failures: make(map[string]int),
	}
}

func (w *LockWatchdog) Start(ctx context.Context) {
	if !w.running.CompareAndSwap(false, true) {
		return
	}

	ctx, cancel := context.WithCancel(ctx)
	w.cancel = cancel

	go w.run(ctx)
	zLog.Info("Lock watchdog started",
		zap.Duration("check_interval", w.config.CheckInterval))
}

func (w *LockWatchdog) Stop() {
	if !w.running.CompareAndSwap(true, false) {
		return
	}

	if w.cancel != nil {
		w.cancel()
	}
	zLog.Info("Lock watchdog stopped")
}

func (w *LockWatchdog) run(ctx context.Context) {
	ticker := time.NewTicker(w.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.checkLocks(ctx)
		}
	}
}

func (w *LockWatchdog) checkLocks(ctx context.Context) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.manager.lockMutex.RLock()
	locks := make(map[string]DistributedLock, len(w.manager.locks))
	for k, v := range w.manager.locks {
		locks[k] = v
	}
	w.manager.lockMutex.RUnlock()

	for key, lock := range locks {
		if !lock.IsLocked() {
			failCount := w.failures[key] + 1
			w.failures[key] = failCount

			if failCount >= w.config.MaxFailures {
				zLog.Error("Lock watchdog: lock lost and max failures reached, releasing",
					zap.String("key", key),
					zap.Int("failures", failCount))
				_ = w.manager.ReleaseLock(key)
				delete(w.failures, key)
				continue
			}

			zLog.Warn("Lock watchdog: lock ownership lost; explicit Acquire required",
				zap.String("key", key),
				zap.Int("failures", failCount))
		} else {
			w.failures[key] = 0
		}
	}
}

func (w *LockWatchdog) IsRunning() bool {
	return w.running.Load()
}
