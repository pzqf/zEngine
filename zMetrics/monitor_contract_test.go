package zMetrics

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMemoryMonitorHandlersAreConcurrentSafe(t *testing.T) {
	monitor := NewMemoryMonitor(AlertConfig{
		HeapAllocThreshold: 1,
		CheckInterval:      time.Millisecond,
		AlertCooldown:      time.Nanosecond,
	})
	var calls atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				monitor.OnAlert(func(Alert) { calls.Add(1) })
				monitor.fireAlert(Alert{Type: AlertType(j % 3)})
			}
		}()
	}
	wg.Wait()
	if calls.Load() == 0 {
		t.Fatal("no alert handler was called")
	}
}

func TestMemoryMonitorStopWakesImmediately(t *testing.T) {
	monitor := NewMemoryMonitor(AlertConfig{
		CheckInterval: time.Hour,
		AlertCooldown: time.Hour,
	})
	monitor.Start()

	done := make(chan struct{})
	go func() {
		monitor.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("Stop waited for the collection interval")
	}
}

func TestMemoryMonitorStartStopIsReusable(t *testing.T) {
	monitor := NewMemoryMonitor(AlertConfig{CheckInterval: time.Millisecond})
	for i := 0; i < 3; i++ {
		monitor.Start()
		time.Sleep(5 * time.Millisecond)
		monitor.Stop()
	}
	if len(monitor.History()) == 0 {
		t.Fatal("restarted monitor never collected a sample")
	}
}
