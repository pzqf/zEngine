package zMetrics

import (
	"sync/atomic"
	"time"
)

type NetworkMetrics struct {
	activeConnections  atomic.Int32
	totalConnections   atomic.Int64
	droppedConnections atomic.Int64

	avgLatencyNano     atomic.Int64
	maxLatencyNano     atomic.Int64
	minLatencyNano     atomic.Int64
	totalLatencyNano   atomic.Int64
	latencySamples     atomic.Int64

	totalBytesSent       atomic.Int64
	totalBytesReceived   atomic.Int64
	totalPacketsSent     atomic.Int64
	totalPacketsReceived atomic.Int64

	encodingErrors    atomic.Int64
	decodingErrors    atomic.Int64
	compressionErrors atomic.Int64
	droppedPackets    atomic.Int64

	lastSampleTime atomic.Int64
}

func NewNetworkMetrics() *NetworkMetrics {
	m := &NetworkMetrics{}
	m.minLatencyNano.Store(int64(time.Hour))
	m.lastSampleTime.Store(time.Now().UnixNano())
	return m
}

func (m *NetworkMetrics) IncActiveConnections() {
	m.activeConnections.Add(1)
	m.totalConnections.Add(1)
}

func (m *NetworkMetrics) DecActiveConnections() {
	m.activeConnections.Add(-1)
}

func (m *NetworkMetrics) IncDroppedConnections() {
	m.droppedConnections.Add(1)
}

func (m *NetworkMetrics) RecordLatency(latency time.Duration) {
	latencyNano := int64(latency)
	m.totalLatencyNano.Add(latencyNano)
	samples := m.latencySamples.Add(1)

	for {
		current := m.maxLatencyNano.Load()
		if latencyNano <= current || m.maxLatencyNano.CompareAndSwap(current, latencyNano) {
			break
		}
	}

	for {
		current := m.minLatencyNano.Load()
		if latencyNano >= current || m.minLatencyNano.CompareAndSwap(current, latencyNano) {
			break
		}
	}

	m.avgLatencyNano.Store(m.totalLatencyNano.Load() / samples)
}

func (m *NetworkMetrics) RecordBytesSent(bytes int) {
	m.totalBytesSent.Add(int64(bytes))
}

func (m *NetworkMetrics) RecordBytesReceived(bytes int) {
	m.totalBytesReceived.Add(int64(bytes))
}

func (m *NetworkMetrics) RecordPacketsSent(count int) {
	m.totalPacketsSent.Add(int64(count))
}

func (m *NetworkMetrics) RecordPacketsReceived(count int) {
	m.totalPacketsReceived.Add(int64(count))
}

func (m *NetworkMetrics) IncEncodingErrors() {
	m.encodingErrors.Add(1)
}

func (m *NetworkMetrics) IncDecodingErrors() {
	m.decodingErrors.Add(1)
}

func (m *NetworkMetrics) IncCompressionErrors() {
	m.compressionErrors.Add(1)
}

func (m *NetworkMetrics) IncDroppedPackets() {
	m.droppedPackets.Add(1)
}

func (m *NetworkMetrics) GetActiveConnections() int {
	return int(m.activeConnections.Load())
}

func (m *NetworkMetrics) GetTotalConnections() int64 {
	return m.totalConnections.Load()
}

func (m *NetworkMetrics) GetDroppedConnections() int64 {
	return m.droppedConnections.Load()
}

func (m *NetworkMetrics) GetAvgLatency() time.Duration {
	return time.Duration(m.avgLatencyNano.Load())
}

func (m *NetworkMetrics) GetMaxLatency() time.Duration {
	return time.Duration(m.maxLatencyNano.Load())
}

func (m *NetworkMetrics) GetMinLatency() time.Duration {
	return time.Duration(m.minLatencyNano.Load())
}

func (m *NetworkMetrics) GetTotalBytesSent() int64 {
	return m.totalBytesSent.Load()
}

func (m *NetworkMetrics) GetTotalBytesReceived() int64 {
	return m.totalBytesReceived.Load()
}

func (m *NetworkMetrics) GetTotalPacketsSent() int64 {
	return m.totalPacketsSent.Load()
}

func (m *NetworkMetrics) GetTotalPacketsReceived() int64 {
	return m.totalPacketsReceived.Load()
}

func (m *NetworkMetrics) GetEncodingErrors() int64 {
	return m.encodingErrors.Load()
}

func (m *NetworkMetrics) GetDecodingErrors() int64 {
	return m.decodingErrors.Load()
}

func (m *NetworkMetrics) GetCompressionErrors() int64 {
	return m.compressionErrors.Load()
}

func (m *NetworkMetrics) GetDroppedPackets() int64 {
	return m.droppedPackets.Load()
}

func (m *NetworkMetrics) Reset() {
	m.activeConnections.Store(0)
	m.totalConnections.Store(0)
	m.droppedConnections.Store(0)
	m.avgLatencyNano.Store(0)
	m.maxLatencyNano.Store(0)
	m.minLatencyNano.Store(int64(time.Hour))
	m.totalLatencyNano.Store(0)
	m.latencySamples.Store(0)
	m.totalBytesSent.Store(0)
	m.totalBytesReceived.Store(0)
	m.totalPacketsSent.Store(0)
	m.totalPacketsReceived.Store(0)
	m.encodingErrors.Store(0)
	m.decodingErrors.Store(0)
	m.compressionErrors.Store(0)
	m.droppedPackets.Store(0)
	m.lastSampleTime.Store(time.Now().UnixNano())
}
