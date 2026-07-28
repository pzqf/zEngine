package zHealth

import (
	"time"

	"github.com/pzqf/zUtil/zMap"
)

// HealthManager 健康管理器
type HealthManager struct {
	serverID     string
	serverType   string
	healthChecks *zMap.TypedMap[string, HealthChecker]
	startTime    time.Time
}

// NewHealthManager 创建健康管理器
func NewHealthManager(serverID, serverType string) *HealthManager {
	return &HealthManager{
		serverID:     serverID,
		serverType:   serverType,
		healthChecks: zMap.NewTypedMap[string, HealthChecker](),
		startTime:    time.Now(),
	}
}

// RegisterCheck 注册健康检查
func (m *HealthManager) RegisterCheck(checker HealthChecker) {
	m.healthChecks.Store(checker.Name(), checker)
}

// UnregisterCheck 注销健康检查
func (m *HealthManager) UnregisterCheck(name string) {
	m.healthChecks.Delete(name)
}

// IsHealthy 检查是否健康
func (m *HealthManager) IsHealthy() bool {
	details := m.GetHealthDetails()
	for _, check := range details {
		if check.Status == HealthStatusUnhealthy {
			return false
		}
	}
	return true
}

// GetHealthDetails 获取健康检查详情
func (m *HealthManager) GetHealthDetails() map[string]HealthCheck {
	details := make(map[string]HealthCheck)

	m.healthChecks.Range(func(name string, checker HealthChecker) bool {
		start := time.Now()
		status, message, err := checker.Check()
		latency := time.Since(start)

		if err != nil {
			status = HealthStatusUnhealthy
			if message == "" {
				message = err.Error()
			}
		}

		details[name] = HealthCheck{
			Name:      name,
			Status:    status,
			Latency:   latency,
			LastCheck: time.Now(),
			Message:   message,
		}
		return true
	})

	return details
}

// GetHealthReport 获取健康报告
func (m *HealthManager) GetHealthReport() HealthReport {
	details := m.GetHealthDetails()
	checks := make([]HealthCheck, 0, len(details))
	for _, check := range details {
		checks = append(checks, check)
	}

	return HealthReport{
		ServerID:   m.serverID,
		ServerType: m.serverType,
		Healthy:    m.IsHealthy(),
		StartTime:  m.startTime,
		Checks:     checks,
	}
}