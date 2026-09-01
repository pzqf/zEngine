package zHealth

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const defaultProbeTimeout = time.Second

type probeEntry struct {
	config    ProbeConfig
	result    HealthCheck
	hasResult bool
	nextRun   time.Time
	running   bool
	done      chan struct{}
}

// HealthManager owns probe registration and immutable result snapshots. HTTP
// and status readers never execute probes; only Refresh or Run does so.
type HealthManager struct {
	serverID   string
	serverType string
	startTime  time.Time

	mu        sync.RWMutex
	refreshMu sync.Mutex
	entries   map[string]*probeEntry
	updatedAt time.Time
}

func NewHealthManager(serverID, serverType string) *HealthManager {
	return &HealthManager{
		serverID:   serverID,
		serverType: serverType,
		startTime:  time.Now(),
		entries:    make(map[string]*probeEntry),
	}
}

// RegisterProbe registers a context-aware probe. A later registration with the
// same name replaces the old configuration and snapshot.
func (m *HealthManager) RegisterProbe(config ProbeConfig) error {
	config.Name = strings.TrimSpace(config.Name)
	if config.Name == "" {
		return fmt.Errorf("health probe name is required")
	}
	if !validProbeScope(config.Scope) {
		return fmt.Errorf("invalid health probe scope %q", config.Scope)
	}
	if config.Check == nil {
		return fmt.Errorf("health probe %q check is nil", config.Name)
	}
	if config.Timeout < 0 || config.CacheTTL < 0 {
		return fmt.Errorf("health probe %q timeout and cache TTL must be non-negative", config.Name)
	}
	if config.Timeout == 0 {
		config.Timeout = defaultProbeTimeout
	}

	m.mu.Lock()
	m.entries[config.Name] = &probeEntry{config: config}
	m.updatedAt = time.Now()
	m.mu.Unlock()
	return nil
}

// RegisterCheck preserves the old API. Legacy checks affect readiness and run
// under the manager's timeout, though they cannot observe context directly.
func (m *HealthManager) RegisterCheck(checker HealthChecker) {
	if checker == nil {
		return
	}
	_ = m.RegisterProbe(ProbeConfig{
		Name:    checker.Name(),
		Scope:   ProbeScopeReadiness,
		Timeout: defaultProbeTimeout,
		Check: func(context.Context) (CheckResult, error) {
			status, message, err := checker.Check()
			return CheckResult{Status: status, Message: message}, err
		},
	})
}

func (m *HealthManager) UnregisterCheck(name string) {
	m.mu.Lock()
	delete(m.entries, name)
	m.updatedAt = time.Now()
	m.mu.Unlock()
}

// UpdateStatus records a status maintained by an upper-layer lifecycle or
// heartbeat. Application code decides which dependencies gate readiness.
func (m *HealthManager) UpdateStatus(name string, scope ProbeScope, status HealthStatus, message string, details map[string]interface{}) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("health status name is required")
	}
	if !validProbeScope(scope) {
		return fmt.Errorf("invalid health probe scope %q", scope)
	}
	if status == "" {
		status = HealthStatusUnknown
	}

	now := time.Now()
	m.mu.Lock()
	entry, ok := m.entries[name]
	if !ok || entry.config.Check != nil {
		entry = &probeEntry{config: ProbeConfig{Name: name, Scope: scope}}
		m.entries[name] = entry
	}
	lastSuccess := entry.result.LastSuccess
	if status.IsAvailable() {
		lastSuccess = now
	}
	entry.result = HealthCheck{
		Name:        name,
		Scope:       scope,
		Status:      status,
		LastCheck:   now,
		LastSuccess: lastSuccess,
		Message:     message,
		Details:     cloneDetails(details),
	}
	entry.hasResult = true
	m.updatedAt = now
	m.mu.Unlock()
	return nil
}

// Refresh executes one round of due probes. Each probe runs at most once in a
// round and the returned report is built from that exact snapshot.
func (m *HealthManager) Refresh(ctx context.Context) HealthReport {
	if ctx == nil {
		ctx = context.Background()
	}
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()

	now := time.Now()
	m.mu.Lock()
	due := make([]*probeEntry, 0, len(m.entries))
	for _, entry := range m.entries {
		if entry.config.Check == nil || entry.running || (!entry.nextRun.IsZero() && now.Before(entry.nextRun)) {
			continue
		}
		entry.running = true
		entry.done = make(chan struct{})
		due = append(due, entry)
	}
	m.mu.Unlock()

	var wg sync.WaitGroup
	for _, entry := range due {
		wg.Add(1)
		go func(e *probeEntry) {
			defer wg.Done()
			m.executeProbe(ctx, e)
		}(entry)
	}
	wg.Wait()
	return m.GetHealthReport()
}

func (m *HealthManager) executeProbe(parent context.Context, entry *probeEntry) {
	started := time.Now()
	ctx, cancel := context.WithTimeout(parent, entry.config.Timeout)
	defer cancel()
	probeDone := entry.done

	type outcome struct {
		result CheckResult
		err    error
	}
	resultCh := make(chan outcome, 1)
	snapshotRecorded := make(chan struct{})
	go func() {
		completed := outcome{}
		defer func() {
			if recovered := recover(); recovered != nil {
				completed = outcome{err: fmt.Errorf("health probe panic: %v", recovered)}
			}
			resultCh <- completed
			<-snapshotRecorded
			m.mu.Lock()
			entry.running = false
			close(entry.done)
			m.mu.Unlock()
		}()
		completed.result, completed.err = entry.config.Check(ctx)
	}()

	var result CheckResult
	var err error
	timedOut := false
	completedBeforeTimeout := false
	select {
	case completed := <-resultCh:
		result, err = completed.result, completed.err
		completedBeforeTimeout = true
	case <-ctx.Done():
		err = ctx.Err()
		timedOut = errors.Is(err, context.DeadlineExceeded)
	}

	finished := time.Now()
	if err != nil {
		result.Status = HealthStatusUnhealthy
		if result.Message == "" {
			result.Message = err.Error()
		}
	} else if result.Status == "" {
		result.Status = HealthStatusUnknown
		if result.Message == "" {
			result.Message = "probe returned no status"
		}
	}

	m.mu.Lock()
	lastSuccess := entry.result.LastSuccess
	if result.Status.IsAvailable() {
		lastSuccess = finished
	}
	entry.result = HealthCheck{
		Name:        entry.config.Name,
		Scope:       entry.config.Scope,
		Status:      result.Status,
		Latency:     finished.Sub(started),
		LastCheck:   finished,
		LastSuccess: lastSuccess,
		Message:     result.Message,
		Details:     cloneDetails(result.Details),
		TimedOut:    timedOut,
	}
	entry.hasResult = true
	entry.nextRun = finished.Add(entry.config.CacheTTL)
	m.updatedAt = finished
	m.mu.Unlock()
	close(snapshotRecorded)
	if completedBeforeTimeout {
		<-probeDone
	}
}

// Run refreshes immediately and then periodically until ctx is canceled.
func (m *HealthManager) Run(ctx context.Context, interval time.Duration) {
	if ctx == nil {
		ctx = context.Background()
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	m.Refresh(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.Refresh(ctx)
		}
	}
}

func (m *HealthManager) IsLive() bool {
	return m.GetHealthReport().Live
}

func (m *HealthManager) IsReady() bool {
	return m.GetHealthReport().Ready
}

func (m *HealthManager) IsHealthy() bool {
	return m.GetHealthReport().Healthy
}

// HealthStatus returns one internally consistent snapshot for zServer.
func (m *HealthManager) HealthStatus() (live, ready, healthy bool) {
	report := m.GetHealthReport()
	return report.Live, report.Ready, report.Healthy
}

func (m *HealthManager) GetHealthDetails() map[string]HealthCheck {
	report := m.GetHealthReport()
	details := make(map[string]HealthCheck, len(report.Checks))
	for _, check := range report.Checks {
		details[check.Name] = check
	}
	return details
}

// GetHealthReport only copies cached data; it never calls a dependency.
func (m *HealthManager) GetHealthReport() HealthReport {
	m.mu.RLock()
	checks := make([]HealthCheck, 0, len(m.entries))
	for name, entry := range m.entries {
		if entry.hasResult {
			checks = append(checks, cloneHealthCheck(entry.result))
			continue
		}
		checks = append(checks, HealthCheck{
			Name:    name,
			Scope:   entry.config.Scope,
			Status:  HealthStatusUnknown,
			Message: "probe has not run",
		})
	}
	updatedAt := m.updatedAt
	m.mu.RUnlock()

	sort.Slice(checks, func(i, j int) bool { return checks[i].Name < checks[j].Name })
	liveness, hasLiveness := aggregate(checks, ProbeScopeLiveness)
	readiness, hasReadiness := aggregate(checks, ProbeScopeReadiness)
	if updatedAt.IsZero() {
		updatedAt = m.startTime
	}
	healthy := (hasLiveness || hasReadiness) &&
		(!hasLiveness || liveness.IsHealthy()) &&
		(!hasReadiness || readiness.IsHealthy())
	return HealthReport{
		ServerID:    m.serverID,
		ServerType:  m.serverType,
		Live:        hasLiveness && liveness.IsAvailable(),
		Ready:       hasReadiness && readiness.IsAvailable(),
		Healthy:     healthy,
		Liveness:    liveness,
		Readiness:   readiness,
		StartTime:   m.startTime,
		GeneratedAt: updatedAt,
		Checks:      checks,
	}
}

func validProbeScope(scope ProbeScope) bool {
	return scope == ProbeScopeLiveness || scope == ProbeScopeReadiness || scope == ProbeScopeDiagnostics
}

func aggregate(checks []HealthCheck, scope ProbeScope) (HealthStatus, bool) {
	status := HealthStatusHealthy
	found := false
	for _, check := range checks {
		if check.Scope != scope {
			continue
		}
		found = true
		switch check.Status {
		case HealthStatusUnhealthy:
			return HealthStatusUnhealthy, true
		case HealthStatusUnknown:
			status = HealthStatusUnknown
		case HealthStatusDegraded:
			if status == HealthStatusHealthy {
				status = HealthStatusDegraded
			}
		case HealthStatusHealthy:
		default:
			status = HealthStatusUnknown
		}
	}
	if !found {
		return HealthStatusUnknown, false
	}
	return status, true
}

func cloneHealthCheck(check HealthCheck) HealthCheck {
	check.Details = cloneDetails(check.Details)
	return check
}

func cloneDetails(details map[string]interface{}) map[string]interface{} {
	if details == nil {
		return nil
	}
	cloned := make(map[string]interface{}, len(details))
	for key, value := range details {
		cloned[key] = value
	}
	return cloned
}
