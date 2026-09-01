package zMetrics_test

import (
	"testing"

	"github.com/pzqf/zEngine/zMetrics"
	"github.com/pzqf/zEngine/zNet"
)

var _ zNet.DetailedNetworkDecodeMetricsRecorder = (*zMetrics.NetworkMetrics)(nil)

func TestNetworkMetricsDetailedDecodeCountersAndReset(t *testing.T) {
	metrics := zMetrics.NewNetworkMetrics()
	metrics.IncWirePacketOversizeErrors()
	metrics.IncDecodedPacketOversizeErrors()
	metrics.IncDecryptErrors()
	metrics.IncDecompressErrors()

	if metrics.GetWirePacketOversizeErrors() != 1 ||
		metrics.GetDecodedPacketOversizeErrors() != 1 ||
		metrics.GetDecryptErrors() != 1 ||
		metrics.GetDecompressErrors() != 1 {
		t.Fatalf("unexpected detailed decode counters: wire=%d decoded=%d decrypt=%d decompress=%d",
			metrics.GetWirePacketOversizeErrors(), metrics.GetDecodedPacketOversizeErrors(),
			metrics.GetDecryptErrors(), metrics.GetDecompressErrors())
	}

	metrics.Reset()
	if metrics.GetWirePacketOversizeErrors() != 0 ||
		metrics.GetDecodedPacketOversizeErrors() != 0 ||
		metrics.GetDecryptErrors() != 0 ||
		metrics.GetDecompressErrors() != 0 {
		t.Fatalf("detailed decode counters survived Reset: wire=%d decoded=%d decrypt=%d decompress=%d",
			metrics.GetWirePacketOversizeErrors(), metrics.GetDecodedPacketOversizeErrors(),
			metrics.GetDecryptErrors(), metrics.GetDecompressErrors())
	}
}
