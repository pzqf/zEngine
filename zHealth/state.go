// health 包提供服务器健康检查功能
package zHealth

import (
	"context"
	"time"
)

// HealthStatus 健康检查状态
type HealthStatus string

const (
	// HealthStatusHealthy 健康 - 健康检查通过，服务运行正常
	HealthStatusHealthy HealthStatus = "healthy"

	// HealthStatusDegraded 降级 - 健康检查部分通过，服务可用但性能或功能受限
	HealthStatusDegraded HealthStatus = "degraded"

	// HealthStatusUnhealthy 不健康 - 健康检查未通过，服务不可用
	HealthStatusUnhealthy HealthStatus = "unhealthy"

	// HealthStatusUnknown 未知 - 尚未检查或检查能力未实现
	HealthStatusUnknown HealthStatus = "unknown"
)

const (
	StatusHealthy   = string(HealthStatusHealthy)
	StatusUnhealthy = string(HealthStatusUnhealthy)
	StatusDegraded  = string(HealthStatusDegraded)
	StatusStarting  = "starting"
	StatusStopping  = "stopping"
	StatusUnknown   = string(HealthStatusUnknown)
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

// IsAvailable 判断当前状态是否仍允许提供服务。
func (h HealthStatus) IsAvailable() bool {
	return h == HealthStatusHealthy || h == HealthStatusDegraded
}

// ProbeScope 区分存活、就绪和仅供诊断的检查。
type ProbeScope string

const (
	ProbeScopeLiveness    ProbeScope = "liveness"
	ProbeScopeReadiness   ProbeScope = "readiness"
	ProbeScopeDiagnostics ProbeScope = "diagnostics"
)

// CheckResult 是 probe 返回的通用结果，不包含执行时间等管理器元数据。
type CheckResult struct {
	Status  HealthStatus           `json:"status"`
	Message string                 `json:"message,omitempty"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// ProbeFunc 必须尊重 context 取消；管理器用它实施单次检查超时。
type ProbeFunc func(context.Context) (CheckResult, error)

// ProbeConfig 描述一个可缓存、有界执行的健康探针。
type ProbeConfig struct {
	Name     string
	Scope    ProbeScope
	Timeout  time.Duration
	CacheTTL time.Duration
	Check    ProbeFunc
}

// HealthCheck 健康检查项
type HealthCheck struct {
	Name        string                 `json:"name"`
	Scope       ProbeScope             `json:"scope"`
	Status      HealthStatus           `json:"status"`
	Latency     time.Duration          `json:"latency"`
	LastCheck   time.Time              `json:"lastCheck"`
	LastSuccess time.Time              `json:"lastSuccess,omitempty"`
	Message     string                 `json:"message"`
	Details     map[string]interface{} `json:"details,omitempty"`
	TimedOut    bool                   `json:"timedOut,omitempty"`
}

// HealthReport 健康报告
type HealthReport struct {
	ServerID    string        `json:"serverId"`
	ServerType  string        `json:"serverType"`
	Live        bool          `json:"live"`
	Ready       bool          `json:"ready"`
	Healthy     bool          `json:"healthy"`
	Liveness    HealthStatus  `json:"liveness"`
	Readiness   HealthStatus  `json:"readiness"`
	StartTime   time.Time     `json:"startTime"`
	GeneratedAt time.Time     `json:"generatedAt"`
	Checks      []HealthCheck `json:"checks"`
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
