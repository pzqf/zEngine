package zNet

import (
	"sync/atomic"
	"testing"
	"time"
)

// TestServerSession_DispatcherPanicRecovered 验证服务端 dispatcher panic 被就地 recover 隔离：
// 每条消息都 panic 也不崩进程/不关会话，后续消息仍被处理。旧实现下第一条 panic 会经 process()
// 终端 recover 关闭会话（传统模式）→ 第二条不再处理，calls 停在 1。
func TestServerSession_DispatcherPanicRecovered(t *testing.T) {
	var calls atomic.Int64
	cfg := &TcpConfig{
		ListenAddress: "127.0.0.1:0", MaxClientCount: 10, ChanSize: 64,
		HeartbeatDuration: 30, MaxPacketDataSize: 1024 * 1024, DisableEncryption: true,
	}
	server := NewTcpServer(cfg)
	defer server.Close()
	server.RegisterDispatcher(func(session Session, p *NetPacket) error {
		calls.Add(1)
		panic("server dispatcher boom")
	})
	go func() { _ = server.Start() }()
	time.Sleep(100 * time.Millisecond)

	host, port := parseAddr(server.GetListenAddress())
	client := NewTcpClient(&TcpClientConfig{DisableEncryption: true, ServerAddr: host, ServerPort: port})
	defer client.Close()
	client.RegisterDispatcher(func(Session, *NetPacket) error { return nil })
	if err := client.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}

	_ = client.Send(100, []byte("a"))
	_ = client.Send(101, []byte("b"))

	deadline := time.Now().Add(3 * time.Second)
	for calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if calls.Load() < 2 {
		t.Fatalf("server must survive dispatcher panics and process both packets, got %d", calls.Load())
	}
}

// TestClientSession_DispatcherPanicRecovered 验证客户端 dispatcher panic 被 recover：
// 旧实现在独立 goroutine 调 dispatcher 且无 recover，panic 会逃逸崩溃整个进程（测试二进制直接死）。
func TestClientSession_DispatcherPanicRecovered(t *testing.T) {
	var calls atomic.Int64
	cfg := &TcpConfig{
		ListenAddress: "127.0.0.1:0", MaxClientCount: 10, ChanSize: 64,
		HeartbeatDuration: 30, MaxPacketDataSize: 1024 * 1024, DisableEncryption: true,
	}
	server := NewTcpServer(cfg)
	defer server.Close()
	server.RegisterDispatcher(func(session Session, p *NetPacket) error {
		_ = session.Send(p.ProtoId, p.Data) // 回显
		return nil
	})
	go func() { _ = server.Start() }()
	time.Sleep(100 * time.Millisecond)

	host, port := parseAddr(server.GetListenAddress())
	client := NewTcpClient(&TcpClientConfig{DisableEncryption: true, ServerAddr: host, ServerPort: port})
	defer client.Close()
	client.RegisterDispatcher(func(Session, *NetPacket) error {
		calls.Add(1)
		panic("client dispatcher boom")
	})
	if err := client.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}

	_ = client.Send(100, []byte("x"))
	_ = client.Send(101, []byte("y"))

	deadline := time.Now().Add(3 * time.Second)
	for calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if calls.Load() < 2 {
		t.Fatalf("client must survive dispatcher panics (no process crash), got %d", calls.Load())
	}
}

// TestClientSession_NilDispatcherNoCrash 验证客户端未注册 dispatcher 时收到消息不 nil 解引用崩溃。
func TestClientSession_NilDispatcherNoCrash(t *testing.T) {
	cfg := &TcpConfig{
		ListenAddress: "127.0.0.1:0", MaxClientCount: 10, ChanSize: 64,
		HeartbeatDuration: 30, MaxPacketDataSize: 1024 * 1024, DisableEncryption: true,
	}
	server := NewTcpServer(cfg)
	defer server.Close()
	server.RegisterDispatcher(func(session Session, p *NetPacket) error {
		_ = session.Send(p.ProtoId, p.Data) // 回显，触发客户端接收路径
		return nil
	})
	go func() { _ = server.Start() }()
	time.Sleep(100 * time.Millisecond)

	host, port := parseAddr(server.GetListenAddress())
	client := NewTcpClient(&TcpClientConfig{DisableEncryption: true, ServerAddr: host, ServerPort: port})
	defer client.Close()
	// 故意不 RegisterDispatcher。
	if err := client.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}

	_ = client.Send(100, []byte("z"))
	time.Sleep(500 * time.Millisecond) // 服务端回显；客户端接收路径遇 nil dispatcher 不得崩溃

	if !client.IsConnected() {
		t.Fatalf("client must stay connected after receiving with nil dispatcher (no crash)")
	}
}
