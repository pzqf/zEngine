package zMetrics

import (
	"sync"
	"time"
)

// NetworkMetrics 网络指标监控
type NetworkMetrics struct {
	mu sync.RWMutex

	// 连接统计
	activeConnections  int
	totalConnections   int64
	droppedConnections int64

	// 延迟统计
	avgLatency     time.Duration
	maxLatency     time.Duration
	minLatency     time.Duration
	totalLatency   time.Duration
	latencySamples int64

	// 吞吐量统计
	totalBytesSent       int64
	totalBytesReceived   int64
	totalPacketsSent     int64
	totalPacketsReceived int64

	// 错误统计
	encodingErrors    int64
	decodingErrors    int64
	compressionErrors int64
	droppedPackets    int64

	// 采样时间
	lastSampleTime time.Time
}

// NewNetworkMetrics 创建网络指标监控实例
func NewNetworkMetrics() *NetworkMetrics {
	return &NetworkMetrics{
		lastSampleTime: time.Now(),
		minLatency:     time.Hour, // 初始化为较大值
	}
}

// IncActiveConnections 增加活跃连接数
func (m *NetworkMetrics) IncActiveConnections() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.activeConnections++
	m.totalConnections++
}

// DecActiveConnections 减少活跃连接数
func (m *NetworkMetrics) DecActiveConnections() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.activeConnections > 0 {
		m.activeConnections--
	}
}

// IncDroppedConnections 增加丢弃连接数
func (m *NetworkMetrics) IncDroppedConnections() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.droppedConnections++
}

// RecordLatency 记录延迟
func (m *NetworkMetrics) RecordLatency(latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalLatency += latency
	m.latencySamples++

	if latency > m.maxLatency {
		m.maxLatency = latency
	}

	if latency < m.minLatency {
		m.minLatency = latency
	}

	// 计算平均延迟
	if m.latencySamples > 0 {
		m.avgLatency = m.totalLatency / time.Duration(m.latencySamples)
	}
}

// RecordBytesSent 记录发送字节数
func (m *NetworkMetrics) RecordBytesSent(bytes int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalBytesSent += int64(bytes)
}

// RecordBytesReceived 记录接收字节数
func (m *NetworkMetrics) RecordBytesReceived(bytes int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalBytesReceived += int64(bytes)
}

// RecordPacketsSent 记录发送包数
func (m *NetworkMetrics) RecordPacketsSent(count int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalPacketsSent += int64(count)
}

// RecordPacketsReceived 记录接收包数
func (m *NetworkMetrics) RecordPacketsReceived(count int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalPacketsReceived += int64(count)
}

// IncEncodingErrors 增加编码错误数
func (m *NetworkMetrics) IncEncodingErrors() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.encodingErrors++
}

// IncDecodingErrors 增加解码错误数
func (m *NetworkMetrics) IncDecodingErrors() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.decodingErrors++
}

// IncCompressionErrors 增加压缩错误数
func (m *NetworkMetrics) IncCompressionErrors() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.compressionErrors++
}

// IncDroppedPackets 增加丢包数
func (m *NetworkMetrics) IncDroppedPackets() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.droppedPackets++
}

// GetActiveConnections 获取活跃连接数
func (m *NetworkMetrics) GetActiveConnections() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.activeConnections
}

// GetTotalConnections 获取总连接数
func (m *NetworkMetrics) GetTotalConnections() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.totalConnections
}

// GetDroppedConnections 获取丢弃连接数
func (m *NetworkMetrics) GetDroppedConnections() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.droppedConnections
}

// GetAvgLatency 获取平均延迟
func (m *NetworkMetrics) GetAvgLatency() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.avgLatency
}

// GetMaxLatency 获取最大延迟
func (m *NetworkMetrics) GetMaxLatency() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.maxLatency
}

// GetMinLatency 获取最小延迟
func (m *NetworkMetrics) GetMinLatency() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.minLatency
}

// GetTotalBytesSent 获取总发送字节数
func (m *NetworkMetrics) GetTotalBytesSent() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.totalBytesSent
}

// GetTotalBytesReceived 获取总接收字节数
func (m *NetworkMetrics) GetTotalBytesReceived() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.totalBytesReceived
}

// GetTotalPacketsSent 获取总发送包数
func (m *NetworkMetrics) GetTotalPacketsSent() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.totalPacketsSent
}

// GetTotalPacketsReceived 获取总接收包数
func (m *NetworkMetrics) GetTotalPacketsReceived() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.totalPacketsReceived
}

// GetEncodingErrors 获取编码错误数
func (m *NetworkMetrics) GetEncodingErrors() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.encodingErrors
}

// GetDecodingErrors 获取解码错误数
func (m *NetworkMetrics) GetDecodingErrors() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.decodingErrors
}

// GetCompressionErrors 获取压缩错误数
func (m *NetworkMetrics) GetCompressionErrors() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.compressionErrors
}

// GetDroppedPackets 获取丢包数
func (m *NetworkMetrics) GetDroppedPackets() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.droppedPackets
}

// Reset 重置所有指标
func (m *NetworkMetrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.activeConnections = 0
	m.totalConnections = 0
	m.droppedConnections = 0
	m.avgLatency = 0
	m.maxLatency = 0
	m.minLatency = time.Hour
	m.totalLatency = 0
	m.latencySamples = 0
	m.totalBytesSent = 0
	m.totalBytesReceived = 0
	m.totalPacketsSent = 0
	m.totalPacketsReceived = 0
	m.encodingErrors = 0
	m.decodingErrors = 0
	m.compressionErrors = 0
	m.droppedPackets = 0
	m.lastSampleTime = time.Now()
}
