// health 包提供服务器健康检查功能
package zHealth

import "time"

// HealthStatus 健康检查状态
type HealthStatus string

const (
	// HealthStatusHealthy 健康 - 健康检查通过，服务运行正常
	HealthStatusHealthy HealthStatus = "healthy"

	// HealthStatusDegraded 降级 - 健康检查部分通过，服务可用但性能或功能受限
	HealthStatusDegraded HealthStatus = "degraded"

	// HealthStatusUnhealthy 不健康 - 健康检查未通过，服务不可用
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

const (
	StatusHealthy   = string(HealthStatusHealthy)
	StatusUnhealthy = string(HealthStatusUnhealthy)
	StatusDegraded  = string(HealthStatusDegraded)
	StatusStarting  = "starting"
	StatusStopping  = "stopping"
	StatusUnknown   = "unknown"
)

const (
	ComponentTCP       = "tcp_service"
	ComponentMap       = "map_service"
	ComponentDiscovery = "service_discovery"
	ComponentDatabase  = "database"
	ComponentConfig    = "config"
	ComponentContainer = "container"
	ComponentGateway   = "gateway"
	ComponentSession   = "session"
	ComponentPlayer    = "player"
)

// String 返回健康状态的字符串表示
func (h HealthStatus) String() string {
	return string(h)
}

// IsHealthy 判断是否健康
func (h HealthStatus) IsHealthy() bool {
	return h == HealthStatusHealthy
}

// HealthCheck 健康检查项
type HealthCheck struct {
	Name      string        `json:"name"`
	Status    HealthStatus  `json:"status"`
	Latency   time.Duration `json:"latency"`
	LastCheck time.Time     `json:"lastCheck"`
	Message   string        `json:"message"`
}

// HealthReport 健康报告
type HealthReport struct {
	ServerID   string        `json:"serverId"`
	ServerType string        `json:"serverType"`
	Healthy    bool          `json:"healthy"`
	StartTime  time.Time     `json:"startTime"`
	Checks     []HealthCheck `json:"checks"`
}

// HealthChecker 健康检查接口
type HealthChecker interface {
	Name() string
	Check() (HealthStatus, string, error)
}

// HealthManager 健康管理器接口
type HealthManagerInterface interface {
	// 健康检查
	RegisterCheck(checker HealthChecker)
	UnregisterCheck(name string)
	IsHealthy() bool
	GetHealthDetails() map[string]HealthCheck

	// 健康报告
	GetHealthReport() HealthReport
}
