package zNet

import (
	"sync/atomic"
	"testing"
	"time"
)

// mockRecorder 是 NetworkMetricsRecorder 的测试替身，用原子计数记录各上报点被调用情况。
type mockRecorder struct {
	active    atomic.Int64
	dropped   atomic.Int64
	bytesRecv atomic.Int64
	bytesSent atomic.Int64
	pktRecv   atomic.Int64
	pktSent   atomic.Int64
	decodeErr atomic.Int64
}

func (m *mockRecorder) IncActiveConnections()       { m.active.Add(1) }
func (m *mockRecorder) DecActiveConnections()       { m.active.Add(-1) }
func (m *mockRecorder) IncDroppedConnections()      { m.dropped.Add(1) }
func (m *mockRecorder) RecordBytesReceived(b int)   { m.bytesRecv.Add(int64(b)) }
func (m *mockRecorder) RecordBytesSent(b int)       { m.bytesSent.Add(int64(b)) }
func (m *mockRecorder) RecordPacketsReceived(c int) { m.pktRecv.Add(int64(c)) }
func (m *mockRecorder) RecordPacketsSent(c int)     { m.pktSent.Add(int64(c)) }
func (m *mockRecorder) IncDecodingErrors()          { m.decodeErr.Add(1) }

// TestServerMetrics_RecordsConnectionAndTraffic 验证 Phase 3.5 的 zNet 指标上报接入：
// 经 WithServerMetrics 注入 recorder 后，真实 client→server 往返应触发连接生命周期
// （IncActiveConnections）、接收字节/包（RecordBytesReceived/PacketsReceived）与
// 发送字节/包（服务端回显经 send() 上报 RecordBytesSent/PacketsSent）。
func TestServerMetrics_RecordsConnectionAndTraffic(t *testing.T) {
	rec := &mockRecorder{}
	cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:0",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}
	server := NewTcpServer(cfg, WithServerMetrics(rec))
	defer server.Close()

	// 回显 dispatcher，触发服务端发送路径（send() 上报字节/包）。
	server.RegisterDispatcher(func(session Session, p *NetPacket) error {
		_ = session.Send(p.ProtoId, p.Data)
		return nil
	})
	go func() {
		if err := server.Start(); err != nil {
			t.Errorf("server start: %v", err)
		}
	}()
	time.Sleep(100 * time.Millisecond)

	host, port := parseAddr(server.GetListenAddress())
	client := NewTcpClient(&TcpClientConfig{
		DisableEncryption: true,
		ServerAddr:        host,
		ServerPort:        port,
	})
	defer client.Close()
	client.RegisterDispatcher(func(session Session, p *NetPacket) error { return nil })
	if err := client.Connect(); err != nil {
		t.Fatalf("client connect: %v", err)
	}

	if err := client.Send(100, []byte("hello-metrics")); err != nil {
		t.Fatalf("client send: %v", err)
	}

	// 等待服务端接收并回显。
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && (rec.pktRecv.Load() == 0 || rec.pktSent.Load() == 0) {
		time.Sleep(20 * time.Millisecond)
	}

	if rec.active.Load() < 1 {
		t.Fatalf("expected active connections >=1, got %d", rec.active.Load())
	}
	if rec.pktRecv.Load() < 1 {
		t.Fatalf("expected packets received >=1, got %d", rec.pktRecv.Load())
	}
	if rec.bytesRecv.Load() < int64(NetPacketHeadSize) {
		t.Fatalf("expected bytes received >= head size, got %d", rec.bytesRecv.Load())
	}
	if rec.pktSent.Load() < 1 {
		t.Fatalf("expected packets sent >=1 (server echo), got %d", rec.pktSent.Load())
	}
	if rec.bytesSent.Load() <= 0 {
		t.Fatalf("expected bytes sent >0 (server echo), got %d", rec.bytesSent.Load())
	}
}
