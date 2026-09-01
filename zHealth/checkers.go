package zHealth

import (
	"runtime"
)

// MemoryChecker 内存检查器
type MemoryChecker struct {
	maxMemoryMB int
}

// NewMemoryChecker 创建内存检查器
// maxMemoryMB: 最大内存使用阈值（MB），默认 1024 MB
func NewMemoryChecker(maxMemoryMB ...int) *MemoryChecker {
	max := 1024
	if len(maxMemoryMB) > 0 && maxMemoryMB[0] > 0 {
		max = maxMemoryMB[0]
	}
	return &MemoryChecker{maxMemoryMB: max}
}

// Name 返回检查器名称
func (c *MemoryChecker) Name() string {
	return "memory"
}

// Check 执行内存检查
func (c *MemoryChecker) Check() (HealthStatus, string, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// 计算当前内存使用量（MB）
	currentMB := m.Alloc / 1024 / 1024

	if currentMB > uint64(c.maxMemoryMB) {
		return HealthStatusUnhealthy, "Memory usage exceeds threshold", nil
	} else if currentMB > uint64(c.maxMemoryMB)*80/100 {
		return HealthStatusDegraded, "Memory usage approaching threshold", nil
	}

	return HealthStatusHealthy, "Memory usage is normal", nil
}

// GoroutineChecker Goroutine 检查器
type GoroutineChecker struct {
	maxGoroutines int
}

// NewGoroutineChecker 创建 Goroutine 检查器
// maxGoroutines: 最大 Goroutine 数量阈值，默认 1000
func NewGoroutineChecker(maxGoroutines ...int) *GoroutineChecker {
	max := 1000
	if len(maxGoroutines) > 0 && maxGoroutines[0] > 0 {
		max = maxGoroutines[0]
	}
	return &GoroutineChecker{maxGoroutines: max}
}

// Name 返回检查器名称
func (c *GoroutineChecker) Name() string {
	return "goroutine"
}

// Check 执行 Goroutine 检查
func (c *GoroutineChecker) Check() (HealthStatus, string, error) {
	current := runtime.NumGoroutine()

	if current > c.maxGoroutines {
		return HealthStatusUnhealthy, "Goroutine count exceeds threshold", nil
	} else if current > c.maxGoroutines*80/100 {
		return HealthStatusDegraded, "Goroutine count approaching threshold", nil
	}

	return HealthStatusHealthy, "Goroutine count is normal", nil
}

// DiskChecker 磁盘检查器（预留）
type DiskChecker struct {
	maxDiskUsage int
}

// NewDiskChecker 创建磁盘检查器
// maxDiskUsage: 最大磁盘使用百分比，默认 90%
func NewDiskChecker(maxDiskUsage ...int) *DiskChecker {
	max := 90
	if len(maxDiskUsage) > 0 && maxDiskUsage[0] > 0 {
		max = maxDiskUsage[0]
	}
	return &DiskChecker{maxDiskUsage: max}
}

// Name 返回检查器名称
func (c *DiskChecker) Name() string {
	return "disk"
}

// Check 执行磁盘检查
func (c *DiskChecker) Check() (HealthStatus, string, error) {
	return HealthStatusUnknown, "Disk check not implemented", nil
}

// TimeChecker 时间检查器（预留）
type TimeChecker struct{}

// NewTimeChecker 创建时间检查器
func NewTimeChecker() *TimeChecker {
	return &TimeChecker{}
}

// Name 返回检查器名称
func (c *TimeChecker) Name() string {
	return "time"
}

// Check 执行时间检查
func (c *TimeChecker) Check() (HealthStatus, string, error) {
	return HealthStatusUnknown, "Time check not implemented", nil
}

// GCChecker 垃圾回收检查器
type GCChecker struct{}

// NewGCChecker 创建垃圾回收检查器
func NewGCChecker() *GCChecker {
	return &GCChecker{}
}

// Name 返回检查器名称
func (c *GCChecker) Name() string {
	return "gc"
}

// Check 执行垃圾回收检查
func (c *GCChecker) Check() (HealthStatus, string, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// 检查垃圾回收状态
	if m.NumGC > 1000 {
		return HealthStatusDegraded, "GC count is high", nil
	}

	return HealthStatusHealthy, "GC status is normal", nil
}
