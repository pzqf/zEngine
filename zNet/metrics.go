package zNet

import (
	"errors"
	"time"
)

// NetworkMetricsRecorder 是 zNet 对外上报网络指标的最小接口（成熟化改造 Phase 3.5）。
//
// 设计意图：zNet 是底层网络库，不应硬依赖上层的 zMetrics 包。故在此定义仅含 zNet 需要
// 上报的方法的窄接口——zEngine/zMetrics.NetworkMetrics 结构上即满足它（无需显式实现），
// 通过 WithServerMetrics 注入即可把 zNet 的连接/流量/错误接入 Prometheus 指标体系。
// 未注入（nil）时所有上报点零开销跳过，不影响原有行为。
type NetworkMetricsRecorder interface {
	IncActiveConnections()
	DecActiveConnections()
	IncDroppedConnections()
	RecordBytesReceived(bytes int)
	RecordBytesSent(bytes int)
	RecordPacketsReceived(count int)
	RecordPacketsSent(count int)
	IncDecodingErrors()
}

// DetailedNetworkDecodeMetricsRecorder 是可选扩展。保留基础 recorder 接口不变，
// 已有自定义实现无需同步增加方法；支持该接口的 recorder 可区分四类资源/解码错误。
type DetailedNetworkDecodeMetricsRecorder interface {
	IncWirePacketOversizeErrors()
	IncDecodedPacketOversizeErrors()
	IncDecryptErrors()
	IncDecompressErrors()
}

// NetworkBackpressureMetricsRecorder 是 NET-03 的可选扩展。容量/深度是当前活跃 session 队列的聚合值，
// 其余值为单调累计计数或等待时长。zNet 不依赖具体 metrics 实现。
type NetworkBackpressureMetricsRecorder interface {
	AddSendQueueCapacity(delta int)
	AddSendQueueDepth(delta int)
	IncLatestFrameDropped()
	IncBestEffortEventRejected()
	IncReliableCommandRejected()
	RecordSendQueueWait(duration time.Duration)
	IncWorkerQueueRejected()
	RecordWorkerQueueWait(duration time.Duration)
}

func backpressureMetrics(recorder NetworkMetricsRecorder) NetworkBackpressureMetricsRecorder {
	metrics, _ := recorder.(NetworkBackpressureMetricsRecorder)
	return metrics
}

func recordPacketDecodeError(recorder NetworkMetricsRecorder, err error) {
	if recorder == nil {
		return
	}
	recorder.IncDecodingErrors()
	detailed, ok := recorder.(DetailedNetworkDecodeMetricsRecorder)
	if !ok {
		return
	}

	switch {
	case errors.Is(err, ErrPacketTooLarge):
		detailed.IncWirePacketOversizeErrors()
	case errors.Is(err, ErrPacketDecodedTooLarge):
		detailed.IncDecodedPacketOversizeErrors()
	case errors.Is(err, ErrPacketDecrypt):
		detailed.IncDecryptErrors()
	case errors.Is(err, ErrPacketDecompress):
		detailed.IncDecompressErrors()
	}
}
