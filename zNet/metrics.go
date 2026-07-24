package zNet

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
