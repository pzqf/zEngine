package zHealth

import (
	"context"
	"sync"
	"time"

	"github.com/pzqf/zEngine/zLog"
	"go.uber.org/zap"
)

// CheckFunc is the legacy context-free check callback.
type CheckFunc func() CheckResult

// Checker preserves the old public facade while sharing HealthManager's probe
// execution and snapshot semantics.
type Checker struct {
	manager       *HealthManager
	mu            sync.RWMutex
	checkInterval time.Duration
}

type ComponentStatus struct {
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Message     string    `json:"message,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
	LastSuccess time.Time `json:"lastSuccess,omitempty"`
}

func NewChecker() *Checker {
	return &Checker{
		manager:       NewHealthManager("", ""),
		checkInterval: 30 * time.Second,
	}
}

func (c *Checker) SetCheckInterval(interval time.Duration) {
	c.mu.Lock()
	c.checkInterval = interval
	c.mu.Unlock()
}

func (c *Checker) RegisterCheck(name string, checkFn CheckFunc) {
	if checkFn == nil {
		return
	}
	_ = c.manager.RegisterProbe(ProbeConfig{
		Name:    name,
		Scope:   ProbeScopeReadiness,
		Timeout: defaultProbeTimeout,
		Check: func(context.Context) (CheckResult, error) {
			return checkFn(), nil
		},
	})
}

func (c *Checker) RegisterProbe(config ProbeConfig) error {
	return c.manager.RegisterProbe(config)
}

func (c *Checker) UpdateComponentStatus(name, status, message string) {
	_ = c.manager.UpdateStatus(name, ProbeScopeReadiness, HealthStatus(status), message, nil)
}

func (c *Checker) UpdateStatus(name string, scope ProbeScope, status HealthStatus, message string, details map[string]interface{}) error {
	return c.manager.UpdateStatus(name, scope, status, message, details)
}

func (c *Checker) GetComponentStatus(name string) (ComponentStatus, bool) {
	check, ok := c.manager.GetHealthDetails()[name]
	if !ok {
		return ComponentStatus{}, false
	}
	return componentStatus(check), true
}

func (c *Checker) GetAllComponentStatus() map[string]ComponentStatus {
	details := c.manager.GetHealthDetails()
	statuses := make(map[string]ComponentStatus, len(details))
	for name, check := range details {
		statuses[name] = componentStatus(check)
	}
	return statuses
}

func (c *Checker) Start(ctx context.Context) {
	zLog.Info("Starting health checker")
	c.mu.RLock()
	interval := c.checkInterval
	c.mu.RUnlock()
	c.manager.Run(ctx, interval)
	zLog.Info("Health checker stopped")
}

func (c *Checker) Refresh(ctx context.Context) HealthReport {
	return c.manager.Refresh(ctx)
}

func (c *Checker) CheckHealth() HealthReport {
	return c.manager.GetHealthReport()
}

func (c *Checker) GetHealthReport() HealthReport {
	return c.manager.GetHealthReport()
}

func (c *Checker) IsLive() bool {
	return c.manager.IsLive()
}

func (c *Checker) IsReady() bool {
	return c.manager.IsReady()
}

func (c *Checker) IsHealthy() bool {
	return c.manager.IsHealthy()
}

func (c *Checker) HealthStatus() (live, ready, healthy bool) {
	return c.manager.HealthStatus()
}

func (c *Checker) LogStatus() {
	statuses := c.GetAllComponentStatus()
	for name, status := range statuses {
		zLog.Info("Component status",
			zap.String("component", name),
			zap.String("status", status.Status),
			zap.String("message", status.Message))
	}
}

func componentStatus(check HealthCheck) ComponentStatus {
	return ComponentStatus{
		Name:        check.Name,
		Status:      string(check.Status),
		Message:     check.Message,
		Timestamp:   check.LastCheck,
		LastSuccess: check.LastSuccess,
	}
}

type CheckRunner interface {
	SetCheckInterval(interval time.Duration)
	Start(ctx context.Context)
}

func StartChecker(ctx context.Context, c CheckRunner, interval ...time.Duration) {
	d := 30 * time.Second
	if len(interval) > 0 {
		d = interval[0]
	}
	c.SetCheckInterval(d)
	c.Start(ctx)
}
