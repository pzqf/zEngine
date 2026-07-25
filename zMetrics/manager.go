package zMetrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// MetricsManager 指标管理器
type MetricsManager struct {
	mu             sync.RWMutex
	networkMetrics *NetworkMetrics

	// Prometheus 相关
	registry   *prometheus.Registry
	counters   map[string]prometheus.Counter
	histograms map[string]prometheus.Histogram
	gauges     map[string]prometheus.Gauge

	// 指标分类管理
	metricsByCategory map[MetricCategory]map[string]bool

	// 指标配置管理
	metricConfigs map[string]MetricConfig
}

// NewMetricsManager 创建指标管理器实例
func NewMetricsManager() *MetricsManager {
	return &MetricsManager{
		networkMetrics:    NewNetworkMetrics(),
		registry:          prometheus.NewRegistry(),
		counters:          make(map[string]prometheus.Counter),
		histograms:        make(map[string]prometheus.Histogram),
		gauges:            make(map[string]prometheus.Gauge),
		metricsByCategory: make(map[MetricCategory]map[string]bool),
		metricConfigs:     make(map[string]MetricConfig),
	}
}

// GetNetworkMetrics 获取网络指标实例
func (m *MetricsManager) GetNetworkMetrics() *NetworkMetrics {
	return m.networkMetrics
}

// GetRegistry 获取 Prometheus 注册表
func (m *MetricsManager) GetRegistry() *prometheus.Registry {
	return m.registry
}

// RegisterCounter 注册计数器
func (m *MetricsManager) RegisterCounter(name, help string, labels map[string]string) prometheus.Counter {
	return m.RegisterCounterWithCategory(name, help, CategoryCustom, labels)
}

// RegisterCounterWithCategory 注册带分类的计数器
func (m *MetricsManager) RegisterCounterWithCategory(name, help string, category MetricCategory, labels map[string]string) prometheus.Counter {
	m.mu.Lock()
	defer m.mu.Unlock()

	if counter, exists := m.counters[name]; exists {
		return counter
	}

	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name:        name,
		Help:        help,
		ConstLabels: labels,
	})

	// OPT-5: 用 Register（返错）而非 MustRegister（panic）。此前只查 m.counters，若同名已被
	// gauge/histogram 注册则漏检、MustRegister 撞名 panic 崩进程。优雅降级：撞名时缓存并返回
	// 未注册的本地计数器（可正常 Inc、只是不被 /metrics 抓取），不崩进程。
	if err := m.registry.Register(counter); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok2 := are.ExistingCollector.(prometheus.Counter); ok2 {
				counter = existing
			}
		}
	}
	m.counters[name] = counter

	// 记录指标分类
	m.addMetricToCategory(name, category)

	// 记录指标配置
	m.metricConfigs[name] = MetricConfig{
		Name:     name,
		Help:     help,
		Type:     MetricTypeCounter,
		Category: category,
		Labels:   labels,
	}

	return counter
}

// RegisterGauge 注册仪表盘
func (m *MetricsManager) RegisterGauge(name, help string, labels map[string]string) prometheus.Gauge {
	return m.RegisterGaugeWithCategory(name, help, CategoryCustom, labels)
}

// RegisterGaugeWithCategory 注册带分类的仪表盘
func (m *MetricsManager) RegisterGaugeWithCategory(name, help string, category MetricCategory, labels map[string]string) prometheus.Gauge {
	m.mu.Lock()
	defer m.mu.Unlock()

	if gauge, exists := m.gauges[name]; exists {
		return gauge
	}

	gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        name,
		Help:        help,
		ConstLabels: labels,
	})

	// OPT-5: 同 counter，用 Register 优雅处理跨类型撞名，避免 MustRegister panic。
	if err := m.registry.Register(gauge); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok2 := are.ExistingCollector.(prometheus.Gauge); ok2 {
				gauge = existing
			}
		}
	}
	m.gauges[name] = gauge

	// 记录指标分类
	m.addMetricToCategory(name, category)

	// 记录指标配置
	m.metricConfigs[name] = MetricConfig{
		Name:     name,
		Help:     help,
		Type:     MetricTypeGauge,
		Category: category,
		Labels:   labels,
	}

	return gauge
}

// RegisterHistogram 注册直方图
func (m *MetricsManager) RegisterHistogram(name, help string, buckets []float64, labels map[string]string) prometheus.Histogram {
	return m.RegisterHistogramWithCategory(name, help, CategoryCustom, buckets, labels)
}

// RegisterHistogramWithCategory 注册带分类的直方图
func (m *MetricsManager) RegisterHistogramWithCategory(name, help string, category MetricCategory, buckets []float64, labels map[string]string) prometheus.Histogram {
	m.mu.Lock()
	defer m.mu.Unlock()

	if histogram, exists := m.histograms[name]; exists {
		return histogram
	}

	if buckets == nil {
		buckets = prometheus.DefBuckets
	}

	histogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:        name,
		Help:        help,
		Buckets:     buckets,
		ConstLabels: labels,
	})

	// OPT-5: 同 counter，用 Register 优雅处理跨类型撞名，避免 MustRegister panic。
	if err := m.registry.Register(histogram); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok2 := are.ExistingCollector.(prometheus.Histogram); ok2 {
				histogram = existing
			}
		}
	}
	m.histograms[name] = histogram

	// 记录指标分类
	m.addMetricToCategory(name, category)

	// 记录指标配置
	m.metricConfigs[name] = MetricConfig{
		Name:     name,
		Help:     help,
		Type:     MetricTypeHistogram,
		Category: category,
		Labels:   labels,
		Buckets:  buckets,
	}

	return histogram
}

// GetCounter 获取计数器
func (m *MetricsManager) GetCounter(name string) (prometheus.Counter, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	counter, exists := m.counters[name]
	return counter, exists
}

// GetGauge 获取仪表盘
func (m *MetricsManager) GetGauge(name string) (prometheus.Gauge, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	gauge, exists := m.gauges[name]
	return gauge, exists
}

// GetHistogram 获取直方图
func (m *MetricsManager) GetHistogram(name string) (prometheus.Histogram, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	histogram, exists := m.histograms[name]
	return histogram, exists
}

// GetMetricsByCategory 获取指定分类的所有指标
func (m *MetricsManager) GetMetricsByCategory(category MetricCategory) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metrics, exists := m.metricsByCategory[category]
	if !exists {
		return nil
	}

	result := make([]string, 0, len(metrics))
	for name := range metrics {
		result = append(result, name)
	}

	return result
}

// GetMetricConfig 获取指标配置
func (m *MetricsManager) GetMetricConfig(name string) (MetricConfig, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	config, exists := m.metricConfigs[name]
	return config, exists
}

// GetAllMetricConfigs 获取所有指标配置
func (m *MetricsManager) GetAllMetricConfigs() map[string]MetricConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 返回副本
	configs := make(map[string]MetricConfig, len(m.metricConfigs))
	for k, v := range m.metricConfigs {
		configs[k] = v
	}

	return configs
}

// UnregisterMetric 注销指标
func (m *MetricsManager) UnregisterMetric(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	config, exists := m.metricConfigs[name]
	if !exists {
		return false
	}

	// 从 registry 中注销
	switch config.Type {
	case MetricTypeCounter:
		if counter, exists := m.counters[name]; exists {
			m.registry.Unregister(counter)
			delete(m.counters, name)
		}
	case MetricTypeGauge:
		if gauge, exists := m.gauges[name]; exists {
			m.registry.Unregister(gauge)
			delete(m.gauges, name)
		}
	case MetricTypeHistogram:
		if histogram, exists := m.histograms[name]; exists {
			m.registry.Unregister(histogram)
			delete(m.histograms, name)
		}
	}

	// 从分类中移除
	if metrics, exists := m.metricsByCategory[config.Category]; exists {
		delete(metrics, name)
	}

	// 从配置中移除
	delete(m.metricConfigs, name)

	return true
}

// ResetAll 重置所有指标
func (m *MetricsManager) ResetAll() {
	m.networkMetrics.Reset()
}

// addMetricToCategory 添加指标到分类
func (m *MetricsManager) addMetricToCategory(name string, category MetricCategory) {
	if m.metricsByCategory[category] == nil {
		m.metricsByCategory[category] = make(map[string]bool)
	}
	m.metricsByCategory[category][name] = true
}
