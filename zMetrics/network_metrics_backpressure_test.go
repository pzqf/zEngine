package zMetrics

import (
	"testing"
	"time"

	"github.com/pzqf/zEngine/zNet"
)

func TestNetworkMetricsBackpressureCountersAndReset(t *testing.T) {
	metrics := NewNetworkMetrics()
	var _ zNet.NetworkBackpressureMetricsRecorder = metrics

	metrics.AddSendQueueCapacity(8)
	metrics.AddSendQueueDepth(3)
	metrics.IncLatestFrameDropped()
	metrics.IncBestEffortEventRejected()
	metrics.IncReliableCommandRejected()
	metrics.RecordSendQueueWait(7 * time.Millisecond)
	metrics.IncWorkerQueueRejected()
	metrics.RecordWorkerQueueWait(5 * time.Millisecond)

	if metrics.GetSendQueueCapacity() != 8 || metrics.GetSendQueueDepth() != 3 {
		t.Fatalf("queue gauges = capacity %d depth %d", metrics.GetSendQueueCapacity(), metrics.GetSendQueueDepth())
	}
	if metrics.GetLatestFrameDropped() != 1 || metrics.GetBestEffortEventRejected() != 1 || metrics.GetReliableCommandRejected() != 1 {
		t.Fatal("delivery rejection counters were not recorded")
	}
	if metrics.GetSendQueueWait() != 7*time.Millisecond || metrics.GetWorkerQueueWait() != 5*time.Millisecond {
		t.Fatal("queue wait durations were not recorded")
	}
	if metrics.GetWorkerQueueRejected() != 1 {
		t.Fatal("worker rejection was not recorded")
	}

	metrics.Reset()
	if metrics.GetSendQueueCapacity() != 0 || metrics.GetSendQueueDepth() != 0 ||
		metrics.GetLatestFrameDropped() != 0 || metrics.GetBestEffortEventRejected() != 0 ||
		metrics.GetReliableCommandRejected() != 0 || metrics.GetSendQueueWait() != 0 ||
		metrics.GetWorkerQueueRejected() != 0 || metrics.GetWorkerQueueWait() != 0 {
		t.Fatal("backpressure metrics were not reset")
	}
}
