package zMetrics

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	// ErrMetricSchemaConflict indicates that a name is already owned by a
	// different metric schema in this manager.
	ErrMetricSchemaConflict = errors.New("metric schema conflict")
	// ErrMetricRegistration indicates that Prometheus rejected a collector.
	ErrMetricRegistration = errors.New("metric registration failed")
	// ErrInvalidMetricSchema indicates that a requested schema is malformed.
	ErrInvalidMetricSchema = errors.New("invalid metric schema")
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
	counter, _ := m.RegisterCounterWithCategoryChecked(name, help, category, labels)
	return counter
}

// RegisterCounterChecked registers a counter or returns a visible schema or
// registry error. A failed collector is never cached.
func (m *MetricsManager) RegisterCounterChecked(name, help string, labels map[string]string) (prometheus.Counter, error) {
	return m.RegisterCounterWithCategoryChecked(name, help, CategoryCustom, labels)
}

// RegisterCounterWithCategoryChecked is the categorized checked counter API.
func (m *MetricsManager) RegisterCounterWithCategoryChecked(name, help string, category MetricCategory, labels map[string]string) (prometheus.Counter, error) {
	config, err := normalizeMetricConfig(MetricConfig{
		Name:     name,
		Help:     help,
		Type:     MetricTypeCounter,
		Category: category,
		Labels:   labels,
	})
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, found, err := m.existingMetricLocked(config); found || err != nil {
		if err != nil {
			return nil, err
		}
		counter, ok := existing.(prometheus.Counter)
		if !ok {
			return nil, fmt.Errorf("%w: cached collector %q is not a counter", ErrMetricRegistration, name)
		}
		return counter, nil
	}

	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name:        config.Name,
		Help:        config.Help,
		ConstLabels: config.Labels,
	})
	if err := m.registerMetricLocked(config, counter); err != nil {
		return nil, err
	}
	return counter, nil
}

// RegisterGauge 注册仪表盘
func (m *MetricsManager) RegisterGauge(name, help string, labels map[string]string) prometheus.Gauge {
	return m.RegisterGaugeWithCategory(name, help, CategoryCustom, labels)
}

// RegisterGaugeWithCategory 注册带分类的仪表盘
func (m *MetricsManager) RegisterGaugeWithCategory(name, help string, category MetricCategory, labels map[string]string) prometheus.Gauge {
	gauge, _ := m.RegisterGaugeWithCategoryChecked(name, help, category, labels)
	return gauge
}

// RegisterGaugeChecked registers a gauge or returns a visible schema or
// registry error. A failed collector is never cached.
func (m *MetricsManager) RegisterGaugeChecked(name, help string, labels map[string]string) (prometheus.Gauge, error) {
	return m.RegisterGaugeWithCategoryChecked(name, help, CategoryCustom, labels)
}

// RegisterGaugeWithCategoryChecked is the categorized checked gauge API.
func (m *MetricsManager) RegisterGaugeWithCategoryChecked(name, help string, category MetricCategory, labels map[string]string) (prometheus.Gauge, error) {
	config, err := normalizeMetricConfig(MetricConfig{
		Name:     name,
		Help:     help,
		Type:     MetricTypeGauge,
		Category: category,
		Labels:   labels,
	})
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, found, err := m.existingMetricLocked(config); found || err != nil {
		if err != nil {
			return nil, err
		}
		gauge, ok := existing.(prometheus.Gauge)
		if !ok {
			return nil, fmt.Errorf("%w: cached collector %q is not a gauge", ErrMetricRegistration, name)
		}
		return gauge, nil
	}

	gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        config.Name,
		Help:        config.Help,
		ConstLabels: config.Labels,
	})
	if err := m.registerMetricLocked(config, gauge); err != nil {
		return nil, err
	}
	return gauge, nil
}

// RegisterHistogram 注册直方图
func (m *MetricsManager) RegisterHistogram(name, help string, buckets []float64, labels map[string]string) prometheus.Histogram {
	return m.RegisterHistogramWithCategory(name, help, CategoryCustom, buckets, labels)
}

// RegisterHistogramWithCategory 注册带分类的直方图
func (m *MetricsManager) RegisterHistogramWithCategory(name, help string, category MetricCategory, buckets []float64, labels map[string]string) prometheus.Histogram {
	histogram, _ := m.RegisterHistogramWithCategoryChecked(name, help, category, buckets, labels)
	return histogram
}

// RegisterHistogramChecked registers a histogram or returns a visible schema
// or registry error. A failed collector is never cached.
func (m *MetricsManager) RegisterHistogramChecked(name, help string, buckets []float64, labels map[string]string) (prometheus.Histogram, error) {
	return m.RegisterHistogramWithCategoryChecked(name, help, CategoryCustom, buckets, labels)
}

// RegisterHistogramWithCategoryChecked is the categorized checked histogram API.
func (m *MetricsManager) RegisterHistogramWithCategoryChecked(name, help string, category MetricCategory, buckets []float64, labels map[string]string) (prometheus.Histogram, error) {
	config, err := normalizeMetricConfig(MetricConfig{
		Name:     name,
		Help:     help,
		Type:     MetricTypeHistogram,
		Category: category,
		Labels:   labels,
		Buckets:  buckets,
	})
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, found, err := m.existingMetricLocked(config); found || err != nil {
		if err != nil {
			return nil, err
		}
		histogram, ok := existing.(prometheus.Histogram)
		if !ok {
			return nil, fmt.Errorf("%w: cached collector %q is not a histogram", ErrMetricRegistration, name)
		}
		return histogram, nil
	}

	histogram, err := newHistogram(config)
	if err != nil {
		return nil, err
	}
	if err := m.registerMetricLocked(config, histogram); err != nil {
		return nil, err
	}
	return histogram, nil
}

func newHistogram(config MetricConfig) (histogram prometheus.Histogram, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			histogram = nil
			err = fmt.Errorf("%w: histogram %q: %v", ErrInvalidMetricSchema, config.Name, recovered)
		}
	}()
	histogram = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:        config.Name,
		Help:        config.Help,
		Buckets:     config.Buckets,
		ConstLabels: config.Labels,
	})
	return histogram, nil
}

func normalizeMetricConfig(config MetricConfig) (MetricConfig, error) {
	config.Labels = cloneLabels(config.Labels)
	if config.Type != MetricTypeHistogram {
		config.Buckets = nil
		return config, nil
	}

	buckets := config.Buckets
	if len(buckets) == 0 {
		buckets = prometheus.DefBuckets
	}
	config.Buckets = append([]float64(nil), buckets...)
	for i, bucket := range config.Buckets {
		if math.IsNaN(bucket) || math.IsInf(bucket, 0) {
			return MetricConfig{}, fmt.Errorf("%w: histogram %q bucket %d is not finite", ErrInvalidMetricSchema, config.Name, i)
		}
		if i > 0 && config.Buckets[i-1] >= bucket {
			return MetricConfig{}, fmt.Errorf("%w: histogram %q buckets are not strictly increasing", ErrInvalidMetricSchema, config.Name)
		}
	}
	return config, nil
}

func cloneLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(labels))
	for name, value := range labels {
		cloned[name] = value
	}
	return cloned
}

func cloneMetricConfig(config MetricConfig) MetricConfig {
	config.Labels = cloneLabels(config.Labels)
	config.Buckets = append([]float64(nil), config.Buckets...)
	return config
}

func sameMetricSchema(left, right MetricConfig) bool {
	return left.Name == right.Name &&
		left.Help == right.Help &&
		left.Type == right.Type &&
		reflect.DeepEqual(left.Labels, right.Labels) &&
		reflect.DeepEqual(left.Buckets, right.Buckets)
}

func (m *MetricsManager) existingMetricLocked(config MetricConfig) (prometheus.Collector, bool, error) {
	existingConfig, exists := m.metricConfigs[config.Name]
	if !exists {
		return nil, false, nil
	}
	if !sameMetricSchema(existingConfig, config) {
		return nil, true, fmt.Errorf(
			"%w: metric %q requested type=%s help=%q labels=%v buckets=%v; existing type=%s help=%q labels=%v buckets=%v",
			ErrMetricSchemaConflict,
			config.Name,
			config.Type,
			config.Help,
			config.Labels,
			config.Buckets,
			existingConfig.Type,
			existingConfig.Help,
			existingConfig.Labels,
			existingConfig.Buckets,
		)
	}

	switch existingConfig.Type {
	case MetricTypeCounter:
		return m.counters[config.Name], true, nil
	case MetricTypeGauge:
		return m.gauges[config.Name], true, nil
	case MetricTypeHistogram:
		return m.histograms[config.Name], true, nil
	default:
		return nil, true, fmt.Errorf("%w: metric %q has unknown cached type %q", ErrMetricRegistration, config.Name, existingConfig.Type)
	}
}

func (m *MetricsManager) registerMetricLocked(config MetricConfig, collector prometheus.Collector) error {
	if err := m.registry.Register(collector); err != nil {
		return fmt.Errorf("%w: metric %q: %v", ErrMetricRegistration, config.Name, err)
	}

	switch config.Type {
	case MetricTypeCounter:
		m.counters[config.Name] = collector.(prometheus.Counter)
	case MetricTypeGauge:
		m.gauges[config.Name] = collector.(prometheus.Gauge)
	case MetricTypeHistogram:
		m.histograms[config.Name] = collector.(prometheus.Histogram)
	default:
		m.registry.Unregister(collector)
		return fmt.Errorf("%w: metric %q has unknown type %q", ErrInvalidMetricSchema, config.Name, config.Type)
	}
	m.addMetricToCategory(config.Name, config.Category)
	m.metricConfigs[config.Name] = cloneMetricConfig(config)
	return nil
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
	return cloneMetricConfig(config), exists
}

// GetAllMetricConfigs 获取所有指标配置
func (m *MetricsManager) GetAllMetricConfigs() map[string]MetricConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 返回副本
	configs := make(map[string]MetricConfig, len(m.metricConfigs))
	for k, v := range m.metricConfigs {
		configs[k] = cloneMetricConfig(v)
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

// ResetNetworkMetrics resets only the in-memory network recorder.
func (m *MetricsManager) ResetNetworkMetrics() {
	m.networkMetrics.Reset()
}

// ResetAll is retained for compatibility. It never reset registered
// Prometheus collectors; use ResetNetworkMetrics for the actual scope.
// Deprecated: use ResetNetworkMetrics.
func (m *MetricsManager) ResetAll() {
	m.ResetNetworkMetrics()
}

// addMetricToCategory 添加指标到分类
func (m *MetricsManager) addMetricToCategory(name string, category MetricCategory) {
	if m.metricsByCategory[category] == nil {
		m.metricsByCategory[category] = make(map[string]bool)
	}
	m.metricsByCategory[category][name] = true
}
