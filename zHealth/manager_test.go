package zHealth

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestHealthManager_EmptySnapshotIsUnknown(t *testing.T) {
	m := NewHealthManager("server-1", "test")

	report := m.GetHealthReport()
	if report.Liveness != HealthStatusUnknown || report.Readiness != HealthStatusUnknown {
		t.Fatalf("empty report status = (%s, %s), want (unknown, unknown)", report.Liveness, report.Readiness)
	}
	if report.Live || report.Ready || report.Healthy {
		t.Fatalf("empty report = live:%t ready:%t healthy:%t, want all false", report.Live, report.Ready, report.Healthy)
	}
}

func TestHealthManager_ReportReusesOneProbeSnapshot(t *testing.T) {
	m := NewHealthManager("server-1", "test")
	var calls atomic.Int32
	if err := m.RegisterProbe(ProbeConfig{
		Name:    "dependency",
		Scope:   ProbeScopeReadiness,
		Timeout: time.Second,
		Check: func(context.Context) (CheckResult, error) {
			calls.Add(1)
			return CheckResult{Status: HealthStatusHealthy, Message: "ready"}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}

	refreshed := m.Refresh(context.Background())
	first := m.GetHealthReport()
	second := m.GetHealthReport()
	if got := calls.Load(); got != 1 {
		t.Fatalf("probe calls = %d, want 1", got)
	}
	if !refreshed.Ready || !first.Ready || !second.Ready {
		t.Fatalf("reports should reuse the ready snapshot: refreshed=%+v first=%+v second=%+v", refreshed, first, second)
	}
}

func TestHealthManager_CacheAndLastSuccessSurviveFailure(t *testing.T) {
	m := NewHealthManager("server-1", "test")
	var calls atomic.Int32
	var fail atomic.Bool
	if err := m.RegisterProbe(ProbeConfig{
		Name:     "database",
		Scope:    ProbeScopeReadiness,
		Timeout:  time.Second,
		CacheTTL: 30 * time.Millisecond,
		Check: func(context.Context) (CheckResult, error) {
			calls.Add(1)
			if fail.Load() {
				return CheckResult{}, errors.New("database unavailable")
			}
			return CheckResult{Status: HealthStatusHealthy, Message: "database ready"}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}

	first := m.Refresh(context.Background())
	lastSuccess := first.Checks[0].LastSuccess
	if lastSuccess.IsZero() {
		t.Fatal("successful probe did not record LastSuccess")
	}
	m.Refresh(context.Background())
	if got := calls.Load(); got != 1 {
		t.Fatalf("cached probe calls = %d, want 1", got)
	}

	fail.Store(true)
	time.Sleep(35 * time.Millisecond)
	failed := m.Refresh(context.Background())
	if failed.Ready || failed.Checks[0].Status != HealthStatusUnhealthy {
		t.Fatalf("failed report = %+v, want not ready/unhealthy", failed)
	}
	if !failed.Checks[0].LastSuccess.Equal(lastSuccess) {
		t.Fatalf("LastSuccess changed on failure: got %s want %s", failed.Checks[0].LastSuccess, lastSuccess)
	}

	fail.Store(false)
	time.Sleep(35 * time.Millisecond)
	recovered := m.Refresh(context.Background())
	if !recovered.Ready || recovered.Checks[0].Status != HealthStatusHealthy {
		t.Fatalf("recovered report = %+v, want ready/healthy", recovered)
	}
	if !recovered.Checks[0].LastSuccess.After(lastSuccess) {
		t.Fatalf("LastSuccess = %s, want after %s", recovered.Checks[0].LastSuccess, lastSuccess)
	}
}

func TestHealthManager_ProbeTimeoutDoesNotBlockRecovery(t *testing.T) {
	m := NewHealthManager("server-1", "test")
	var slow atomic.Bool
	slow.Store(true)
	if err := m.RegisterProbe(ProbeConfig{
		Name:    "etcd",
		Scope:   ProbeScopeReadiness,
		Timeout: 20 * time.Millisecond,
		Check: func(ctx context.Context) (CheckResult, error) {
			if slow.Load() {
				<-ctx.Done()
				return CheckResult{}, ctx.Err()
			}
			return CheckResult{Status: HealthStatusHealthy}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	timedOut := m.Refresh(context.Background())
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("timeout refresh took %s", elapsed)
	}
	if !timedOut.Checks[0].TimedOut || timedOut.Checks[0].Status != HealthStatusUnhealthy {
		t.Fatalf("timeout check = %+v", timedOut.Checks[0])
	}

	slow.Store(false)
	m.mu.RLock()
	probeDone := m.entries["etcd"].done
	m.mu.RUnlock()
	select {
	case <-probeDone:
	case <-time.After(time.Second):
		t.Fatal("timed-out probe did not release its in-flight slot after context cancellation")
	}
	recovered := m.Refresh(context.Background())
	if !recovered.Ready || recovered.Checks[0].TimedOut {
		t.Fatalf("recovered report = %+v", recovered)
	}
}

func TestHealthManager_ConcurrentRefreshAndSnapshot(t *testing.T) {
	m := NewHealthManager("server-1", "test")
	if err := m.RegisterProbe(ProbeConfig{
		Name:    "process",
		Scope:   ProbeScopeLiveness,
		Timeout: time.Second,
		Check: func(context.Context) (CheckResult, error) {
			return CheckResult{Status: HealthStatusHealthy}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			m.Refresh(context.Background())
		}()
		go func() {
			defer wg.Done()
			_ = m.GetHealthDetails()
			_ = m.GetHealthReport()
		}()
	}
	wg.Wait()
	if !m.IsLive() {
		t.Fatal("manager should be live after concurrent refresh")
	}
}

func TestHealthManager_SequentialRefreshRunsDueProbe(t *testing.T) {
	m := NewHealthManager("server-1", "test")
	var healthy atomic.Bool
	if err := m.RegisterProbe(ProbeConfig{
		Name: "dependency", Scope: ProbeScopeReadiness, Timeout: time.Second,
		Check: func(context.Context) (CheckResult, error) {
			status := HealthStatusUnhealthy
			if healthy.Load() {
				status = HealthStatusHealthy
			}
			return CheckResult{Status: status}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}

	if first := m.Refresh(context.Background()); first.Ready {
		t.Fatalf("first refresh = %+v, want not ready", first)
	}
	healthy.Store(true)
	if second := m.Refresh(context.Background()); !second.Ready {
		t.Fatalf("second refresh = %+v, want ready", second)
	}
}

func TestHealthManager_ProbePanicBecomesUnhealthy(t *testing.T) {
	m := NewHealthManager("server-1", "test")
	if err := m.RegisterProbe(ProbeConfig{
		Name: "panic", Scope: ProbeScopeReadiness, Timeout: time.Second,
		Check: func(context.Context) (CheckResult, error) {
			panic("broken probe")
		},
	}); err != nil {
		t.Fatal(err)
	}

	report := m.Refresh(context.Background())
	if report.Ready || report.Checks[0].Status != HealthStatusUnhealthy {
		t.Fatalf("panic report = %+v", report)
	}
}

func TestHealthManager_TimedOutProbeDoesNotOverlap(t *testing.T) {
	m := NewHealthManager("server-1", "test")
	release := make(chan struct{})
	var calls atomic.Int32
	if err := m.RegisterProbe(ProbeConfig{
		Name: "legacy", Scope: ProbeScopeReadiness, Timeout: 10 * time.Millisecond,
		Check: func(context.Context) (CheckResult, error) {
			calls.Add(1)
			<-release
			return CheckResult{Status: HealthStatusHealthy}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}

	report := m.Refresh(context.Background())
	if !report.Checks[0].TimedOut {
		t.Fatalf("first report = %+v, want timeout", report)
	}
	for i := 0; i < 10; i++ {
		m.Refresh(context.Background())
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("blocked probe calls = %d, want one in-flight call", got)
	}

	close(release)
	m.mu.RLock()
	probeDone := m.entries["legacy"].done
	m.mu.RUnlock()
	select {
	case <-probeDone:
	case <-time.After(time.Second):
		t.Fatal("released probe did not clear its in-flight slot")
	}
}
