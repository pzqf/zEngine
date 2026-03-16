package zMetrics

// MetricType 指标类型
type MetricType string

const (
	// MetricTypeCounter 计数器类型
	MetricTypeCounter MetricType = "counter"
	// MetricTypeGauge 仪表盘类型
	MetricTypeGauge MetricType = "gauge"
	// MetricTypeHistogram 直方图类型
	MetricTypeHistogram MetricType = "histogram"
)

// MetricCategory 指标分类
type MetricCategory string

const (
	// CategoryNetwork 网络相关指标
	CategoryNetwork MetricCategory = "network"
	// CategorySystem 系统相关指标
	CategorySystem MetricCategory = "system"
	// CategoryDatabase 数据库相关指标
	CategoryDatabase MetricCategory = "database"
	// CategoryCustom 自定义指标
	CategoryCustom MetricCategory = "custom"
)

// MetricConfig 指标配置
type MetricConfig struct {
	Name     string            // 指标名称
	Help     string            // 指标帮助信息
	Type     MetricType        // 指标类型
	Category MetricCategory    // 指标分类
	Labels   map[string]string // 标签
	Buckets  []float64         // 仅用于Histogram类型
}

// Counter 计数器接口
type Counter interface {
	Inc()
	Add(float64)
}

// Gauge 仪表盘接口
type Gauge interface {
	Set(float64)
	Inc()
	Dec()
	Add(float64)
	Sub(float64)
}

// Histogram 直方图接口
type Histogram interface {
	Observe(float64)
}
