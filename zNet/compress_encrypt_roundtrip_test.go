package zNet

import (
	"bytes"
	"testing"
	"time"
)

// TestTcpCompressEncryptRoundTrip 验证「加密 + 压缩同时开启」下的收发往返正确性。
//
// 这是 Phase 1.1.2 的回归护栏：修复前发送端顺序为 encrypt→compress、接收端为
// decrypt→decompress（二者不互逆），当一条消息的密文恰好可压缩、IsCompressed 置位时，
// 接收端会对「压缩后的密文」先解密 → GCM 认证失败 → 整包被丢弃。
// 修复后发送端为 compress→encrypt，与接收端 decrypt→decompress 互逆。
//
// 用例发送一个远大于压缩阈值、且高度可压缩（重复串）的负载，强制触发压缩，
// 断言接收端完整还原原始明文，且 wire 上确实标记了压缩（IsCompressed==CompressionSnappy）。
func TestTcpCompressEncryptRoundTrip(t *testing.T) {
	const addr = "127.0.0.1:18099"

	comp := CompressionConfig{
		Enabled:              true,
		CompressionThreshold: 1024,
		MaxCompressSize:      1024 * 1024,
	}

	serverCfg := &TcpConfig{
		ListenAddress:     addr,
		MaxClientCount:    16,
		ChanSize:          256,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: false, // 开启加密（连接时走 DH 密钥交换）
		DisableCompression: false,
	}

	server := NewTcpServer(serverCfg, WithCompressionConfig(&comp))
	defer server.Close()

	type recv struct {
		data         []byte
		isCompressed int32
	}
	recvCh := make(chan recv, 1)
	server.RegisterDispatcher(func(session Session, packet *NetPacket) error {
		// packet.Data 已由接收端解密+解压，IsCompressed 保留 wire 上的标记
		buf := make([]byte, len(packet.Data))
		copy(buf, packet.Data)
		select {
		case recvCh <- recv{data: buf, isCompressed: packet.IsCompressed}:
		default:
		}
		return nil
	})

	if err := server.Start(); err != nil {
		t.Fatalf("server start failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	clientCfg := &TcpClientConfig{
		ServerAddr:        "127.0.0.1",
		ServerPort:        18099,
		ChanSize:          256,
		MaxPacketDataSize: 1024 * 1024,
		AutoReconnect:     false,
		DisableEncryption: false,
		Compression:       comp,
	}
	client := NewTcpClient(clientCfg)
	defer client.Close()

	if err := client.Connect(); err != nil {
		t.Fatalf("client connect failed: %v", err)
	}

	// 高度可压缩且远超阈值的负载
	payload := bytes.Repeat([]byte("zEngine-compress-encrypt-roundtrip-0123456789"), 200) // ~9KB
	if len(payload) <= comp.CompressionThreshold {
		t.Fatalf("payload not larger than threshold")
	}

	if err := client.Send(1000, payload); err != nil {
		t.Fatalf("client send failed: %v", err)
	}

	select {
	case got := <-recvCh:
		if !bytes.Equal(got.data, payload) {
			t.Fatalf("payload mismatch: got %d bytes, want %d bytes", len(got.data), len(payload))
		}
		if got.isCompressed != CompressionSnappy {
			t.Fatalf("expected wire payload to be compressed (IsCompressed=%d), got %d; "+
				"compression did not trigger, test cannot prove compress→encrypt order",
				CompressionSnappy, got.isCompressed)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for server to receive packet (likely decrypt/decompress order regression)")
	}
}
