package zServer

import "testing"

type reportHealthProvider struct {
	live    bool
	ready   bool
	healthy bool
}

func (p reportHealthProvider) HealthStatus() (bool, bool, bool) {
	return p.live, p.ready, p.healthy
}

func TestStateReportUsesHealthProvider(t *testing.T) {
	server := NewBaseServer("test", "server-1", "test", "0.0.1", testLifecycleHooks{})
	if err := server.SetState(StateInitializing, "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.SetState(StateReady, "test"); err != nil {
		t.Fatal(err)
	}
	server.SetHealthProvider(reportHealthProvider{live: true, ready: false, healthy: false})

	report := server.GetStateReport()
	if !report.Live || report.Ready || report.Healthy {
		t.Fatalf("state report = live:%t ready:%t healthy:%t", report.Live, report.Ready, report.Healthy)
	}
}

func TestStateReportNeverReportsHealthyWhenNotLive(t *testing.T) {
	server := NewBaseServer("test", "server-1", "test", "0.0.1", testLifecycleHooks{})
	if err := server.SetState(StateInitializing, "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.SetState(StateReady, "test"); err != nil {
		t.Fatal(err)
	}
	server.SetHealthProvider(reportHealthProvider{live: false, ready: true, healthy: true})

	report := server.GetStateReport()
	if report.Live || !report.Ready || report.Healthy {
		t.Fatalf("state report = live:%t ready:%t healthy:%t", report.Live, report.Ready, report.Healthy)
	}
}

type testLifecycleHooks struct{}

func (testLifecycleHooks) OnBeforeStart() error { return nil }
func (testLifecycleHooks) OnAfterStart() error  { return nil }
func (testLifecycleHooks) OnBeforeStop()        {}
