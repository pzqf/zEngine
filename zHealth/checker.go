package zHealth

import (
	"context"
	"sync"
	"time"

	"github.com/pzqf/zEngine/zLog"
	"go.uber.org/zap"
)

type CheckResult struct {
	Status  HealthStatus           `json:"status"`
	Message string                 `json:"message,omitempty"`
	Details map[string]interface{} `json:"details,omitempty"`
}

type CheckFunc func() CheckResult

type Checker struct {
	components    map[string]CheckFunc
	componentStat map[string]ComponentStatus
	mu            sync.RWMutex
	checkInterval time.Duration
}

type ComponentStatus struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Message   string    `json:"message,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

func NewChecker() *Checker {
	return &Checker{
		components:    make(map[string]CheckFunc),
		componentStat: make(map[string]ComponentStatus),
		checkInterval: 30 * time.Second,
	}
}

func (c *Checker) SetCheckInterval(interval time.Duration) {
	c.checkInterval = interval
}

func (c *Checker) RegisterCheck(name string, checkFn CheckFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.components[name] = checkFn
}

func (c *Checker) UpdateComponentStatus(name, status, message string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.componentStat[name] = ComponentStatus{
		Name:      name,
		Status:    status,
		Message:   message,
		Timestamp: time.Now(),
	}
}

func (c *Checker) GetComponentStatus(name string) (ComponentStatus, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	status, ok := c.componentStat[name]
	return status, ok
}

func (c *Checker) GetAllComponentStatus() map[string]ComponentStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	statuses := make(map[string]ComponentStatus, len(c.componentStat))
	for k, v := range c.componentStat {
		statuses[k] = v
	}
	return statuses
}

func (c *Checker) Start(ctx context.Context) {
	zLog.Info("Starting health checker")

	ticker := time.NewTicker(c.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			zLog.Info("Health checker stopped")
			return
		case <-ticker.C:
			c.runChecks()
		}
	}
}

func (c *Checker) runChecks() {
	c.mu.RLock()
	components := make(map[string]CheckFunc, len(c.components))
	for k, v := range c.components {
		components[k] = v
	}
	c.mu.RUnlock()

	for name, checkFn := range components {
		result := checkFn()
		c.UpdateComponentStatus(name, string(result.Status), result.Message)
		zLog.Debug("Health check",
			zap.String("component", name),
			zap.String("status", string(result.Status)))
	}
}

func (c *Checker) CheckHealth() HealthReport {
	c.mu.RLock()
	components := make(map[string]CheckFunc, len(c.components))
	for k, v := range c.components {
		components[k] = v
	}
	c.mu.RUnlock()

	checks := make([]HealthCheck, 0, len(components))
	overallHealthy := true

	for name, checkFn := range components {
		result := checkFn()
		checks = append(checks, HealthCheck{
			Name:      name,
			Status:    result.Status,
			Message:   result.Message,
			LastCheck: time.Now(),
		})
		if !result.Status.IsHealthy() {
			overallHealthy = false
		}
	}

	return HealthReport{
		Healthy: overallHealthy,
		Checks:  checks,
	}
}

func (c *Checker) IsHealthy() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, status := range c.componentStat {
		if status.Status != string(HealthStatusHealthy) {
			return false
		}
	}
	return len(c.componentStat) > 0
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
