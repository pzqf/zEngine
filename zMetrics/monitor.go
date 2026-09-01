package zMetrics

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pzqf/zEngine/zLog"
	"go.uber.org/zap"
)

type MemoryStats struct {
	Alloc      uint64
	TotalAlloc uint64
	Sys        uint64
	NumGC      uint32
	Goroutines int
	HeapAlloc  uint64
	HeapSys    uint64
	HeapInUse  uint64
	StackInUse uint64
	Timestamp  time.Time
}

type AlertConfig struct {
	HeapAllocThreshold uint64
	GoroutineThreshold int
	GCCountThreshold   uint32
	CheckInterval      time.Duration
	AlertCooldown      time.Duration
}

func DefaultAlertConfig() AlertConfig {
	return AlertConfig{
		HeapAllocThreshold: 512 * 1024 * 1024,
		GoroutineThreshold: 10000,
		GCCountThreshold:   0,
		CheckInterval:      10 * time.Second,
		AlertCooldown:      60 * time.Second,
	}
}

type AlertType int

const (
	AlertHeapAlloc AlertType = iota
	AlertGoroutines
	AlertGCCount
)

type Alert struct {
	Type      AlertType
	Message   string
	Stats     MemoryStats
	Timestamp time.Time
}

type AlertHandler func(alert Alert)

type MemoryMonitor struct {
	config      AlertConfig
	lifecycleMu sync.Mutex
	running     bool
	stopCh      chan struct{}
	wg          sync.WaitGroup
	handlersMu  sync.RWMutex
	handlers    []AlertHandler
	lastAlert   sync.Map
	history     []MemoryStats
	historyMu   sync.Mutex
	historySize int
}

func NewMemoryMonitor(config AlertConfig) *MemoryMonitor {
	if config.CheckInterval <= 0 {
		config.CheckInterval = 10 * time.Second
	}
	if config.AlertCooldown <= 0 {
		config.AlertCooldown = 60 * time.Second
	}
	return &MemoryMonitor{
		config:      config,
		handlers:    make([]AlertHandler, 0),
		historySize: 360,
	}
}

func (mm *MemoryMonitor) OnAlert(handler AlertHandler) {
	if handler == nil {
		return
	}
	mm.handlersMu.Lock()
	defer mm.handlersMu.Unlock()
	mm.handlers = append(mm.handlers, handler)
}

func (mm *MemoryMonitor) Collect() MemoryStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return MemoryStats{
		Alloc:      m.Alloc,
		TotalAlloc: m.TotalAlloc,
		Sys:        m.Sys,
		NumGC:      m.NumGC,
		Goroutines: runtime.NumGoroutine(),
		HeapAlloc:  m.HeapAlloc,
		HeapSys:    m.HeapSys,
		HeapInUse:  m.HeapInuse,
		StackInUse: m.StackInuse,
		Timestamp:  time.Now(),
	}
}

func (mm *MemoryMonitor) checkAlerts(stats MemoryStats) {
	if mm.config.HeapAllocThreshold > 0 && stats.HeapAlloc > mm.config.HeapAllocThreshold {
		mm.fireAlert(Alert{
			Type:    AlertHeapAlloc,
			Message: "heap alloc exceeds threshold",
			Stats:   stats,
		})
	}

	if mm.config.GoroutineThreshold > 0 && stats.Goroutines > mm.config.GoroutineThreshold {
		mm.fireAlert(Alert{
			Type:    AlertGoroutines,
			Message: "goroutine count exceeds threshold",
			Stats:   stats,
		})
	}
}

func (mm *MemoryMonitor) fireAlert(alert Alert) {
	alert.Timestamp = time.Now()

	if lastTime, ok := mm.lastAlert.Load(alert.Type); ok {
		if time.Since(lastTime.(time.Time)) < mm.config.AlertCooldown {
			return
		}
	}

	mm.lastAlert.Store(alert.Type, alert.Timestamp)

	mm.handlersMu.RLock()
	handlers := append([]AlertHandler(nil), mm.handlers...)
	mm.handlersMu.RUnlock()

	for _, handler := range handlers {
		func(h AlertHandler) {
			defer func() {
				if r := recover(); r != nil {
					zLog.Error("Alert handler panic", zap.Any("recover", r))
				}
			}()
			h(alert)
		}(handler)
	}
}

func (mm *MemoryMonitor) recordHistory(stats MemoryStats) {
	mm.historyMu.Lock()
	defer mm.historyMu.Unlock()

	mm.history = append(mm.history, stats)
	if len(mm.history) > mm.historySize {
		mm.history = mm.history[len(mm.history)-mm.historySize:]
	}
}

func (mm *MemoryMonitor) History() []MemoryStats {
	mm.historyMu.Lock()
	defer mm.historyMu.Unlock()
	result := make([]MemoryStats, len(mm.history))
	copy(result, mm.history)
	return result
}

func (mm *MemoryMonitor) Start() {
	mm.lifecycleMu.Lock()
	if mm.running {
		mm.lifecycleMu.Unlock()
		return
	}
	mm.running = true
	mm.stopCh = make(chan struct{})
	stopCh := mm.stopCh

	mm.wg.Add(1)
	mm.lifecycleMu.Unlock()
	go func() {
		defer mm.wg.Done()

		ticker := time.NewTicker(mm.config.CheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				stats := mm.Collect()
				mm.recordHistory(stats)
				mm.checkAlerts(stats)
			}
		}
	}()

	zLog.Info("Memory monitor started",
		zap.Duration("interval", mm.config.CheckInterval),
		zap.Uint64("heap_threshold", mm.config.HeapAllocThreshold),
		zap.Int("goroutine_threshold", mm.config.GoroutineThreshold))
}

func (mm *MemoryMonitor) Stop() {
	mm.lifecycleMu.Lock()
	if !mm.running {
		mm.lifecycleMu.Unlock()
		return
	}
	mm.running = false
	close(mm.stopCh)
	mm.wg.Wait()
	mm.lifecycleMu.Unlock()
	zLog.Info("Memory monitor stopped")
}

func (mm *MemoryMonitor) ForceGC() {
	before := mm.Collect()
	runtime.GC()
	after := mm.Collect()

	zLog.Info("Force GC completed",
		zap.Uint64("before_alloc", before.Alloc),
		zap.Uint64("after_alloc", after.Alloc),
		zap.Uint64("freed", before.Alloc-after.Alloc))
}

type ServiceMonitor struct {
	monitor   *MemoryMonitor
	services  map[string]*ServiceStats
	serviceMu sync.RWMutex
}

// ServiceStats 服务运行时统计。含 atomic 字段，禁止按值拷贝，
// 故 map 中以指针存储，快照通过 ServiceStatsSnapshot 返回。
type ServiceStats struct {
	Name       string
	StartTime  time.Time
	MessageIn  atomic.Uint64
	MessageOut atomic.Uint64
	ErrorCount atomic.Uint64
}

// ServiceStatsSnapshot 是 ServiceStats 的只读值快照，用于对外返回，
// 避免拷贝 atomic 值（拷贝会破坏原子性）。
type ServiceStatsSnapshot struct {
	Name       string
	StartTime  time.Time
	MessageIn  uint64
	MessageOut uint64
	ErrorCount uint64
}

func NewServiceMonitor(alertConfig AlertConfig) *ServiceMonitor {
	return &ServiceMonitor{
		monitor:  NewMemoryMonitor(alertConfig),
		services: make(map[string]*ServiceStats),
	}
}

func (sm *ServiceMonitor) RegisterService(name string) {
	sm.serviceMu.Lock()
	defer sm.serviceMu.Unlock()
	sm.services[name] = &ServiceStats{
		Name:      name,
		StartTime: time.Now(),
	}
}

func (sm *ServiceMonitor) RecordMessageIn(serviceName string) {
	sm.serviceMu.RLock()
	defer sm.serviceMu.RUnlock()
	if s, ok := sm.services[serviceName]; ok {
		s.MessageIn.Add(1)
	}
}

func (sm *ServiceMonitor) RecordMessageOut(serviceName string) {
	sm.serviceMu.RLock()
	defer sm.serviceMu.RUnlock()
	if s, ok := sm.services[serviceName]; ok {
		s.MessageOut.Add(1)
	}
}

func (sm *ServiceMonitor) RecordError(serviceName string) {
	sm.serviceMu.RLock()
	defer sm.serviceMu.RUnlock()
	if s, ok := sm.services[serviceName]; ok {
		s.ErrorCount.Add(1)
	}
}

func (sm *ServiceMonitor) Start() {
	sm.monitor.OnAlert(func(alert Alert) {
		zLog.Warn("Memory alert triggered",
			zap.Int("type", int(alert.Type)),
			zap.String("message", alert.Message),
			zap.Uint64("heap_alloc", alert.Stats.HeapAlloc),
			zap.Int("goroutines", alert.Stats.Goroutines))
	})
	sm.monitor.Start()
}

func (sm *ServiceMonitor) Stop() {
	sm.monitor.Stop()
}

func (sm *ServiceMonitor) GetMemoryStats() MemoryStats {
	return sm.monitor.Collect()
}

func (sm *ServiceMonitor) GetServiceStats(name string) (ServiceStatsSnapshot, bool) {
	sm.serviceMu.RLock()
	defer sm.serviceMu.RUnlock()
	s, ok := sm.services[name]
	if !ok {
		return ServiceStatsSnapshot{}, false
	}
	return ServiceStatsSnapshot{
		Name:       s.Name,
		StartTime:  s.StartTime,
		MessageIn:  s.MessageIn.Load(),
		MessageOut: s.MessageOut.Load(),
		ErrorCount: s.ErrorCount.Load(),
	}, true
}
