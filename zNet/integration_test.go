package zNet

import (
	"crypto/rand"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// parseAddr 解析地址字符串为host和port
func parseAddr(addr string) (string, int) {
	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)
	return host, port
}

func TestClientServerCommunication(t *testing.T) {
	cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:0",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}

	server := NewTcpServer(cfg)
	defer server.Close()

	server.RegisterDispatcher(func(session Session, netPacket *NetPacket) error {
		t.Logf("Server received packet: ProtoId=%d, DataSize=%d", netPacket.ProtoId, netPacket.DataSize)
		return nil
	})

	go func() {
		if err := server.Start(); err != nil {
			t.Errorf("Server start failed: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// 获取实际监听地址
	addr := server.GetListenAddress()
	host, port := parseAddr(addr)

	clientCfg := &TcpClientConfig{
		DisableEncryption: true,
		ServerAddr:    host,
		ServerPort:    port,
		AutoReconnect: false,
	}

	client := NewTcpClient(clientCfg)
	defer client.Close()

	if err := client.Connect(); err != nil {
		t.Fatalf("Client connect failed: %v", err)
	}

	t.Log("Client connected")

	testData := []byte("test message from client")
	if err := client.Send(100, testData); err != nil {
		t.Fatalf("Client send failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	t.Log("Client-Server communication test passed")
}

func TestClientServerMultipleMessages(t *testing.T) {
	cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:0",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}

	server := NewTcpServer(cfg)
	defer server.Close()

	var receivedCount atomic.Int64
	server.RegisterDispatcher(func(session Session, netPacket *NetPacket) error {
		receivedCount.Add(1)
		t.Logf("Server received packet %d: ProtoId=%d", receivedCount.Load(), netPacket.ProtoId)
		return nil
	})

	go func() {
		if err := server.Start(); err != nil {
			t.Errorf("Server start failed: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// 获取实际监听地址
	addr := server.GetListenAddress()
	host, port := parseAddr(addr)

	clientCfg := &TcpClientConfig{
		DisableEncryption: true,
		ServerAddr:    host,
		ServerPort:    port,
		AutoReconnect: false,
	}

	client := NewTcpClient(clientCfg)
	defer client.Close()

	if err := client.Connect(); err != nil {
		t.Fatalf("Client connect failed: %v", err)
	}

	messageCount := 100
	for i := 0; i < messageCount; i++ {
		testData := []byte(fmt.Sprintf("message %d", i))
		if err := client.Send(ProtoIdType(i+1), testData); err != nil {
			t.Fatalf("Client send failed at message %d: %v", i, err)
		}
	}

	time.Sleep(500 * time.Millisecond)

	if int(receivedCount.Load()) != messageCount {
		t.Errorf("Expected %d messages, received %d", messageCount, receivedCount.Load())
	}

	t.Logf("Client-Server multiple messages test passed: %d messages", receivedCount.Load())
}

func TestClientServerWithEncryption(t *testing.T) {
	cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:0",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: false,
	}

	server := NewTcpServer(cfg)
	defer server.Close()

	server.RegisterDispatcher(func(session Session, netPacket *NetPacket) error {
		t.Logf("Server received encrypted packet: ProtoId=%d, DataSize=%d", netPacket.ProtoId, netPacket.DataSize)
		return nil
	})

	go func() {
		if err := server.Start(); err != nil {
			t.Errorf("Server start failed: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// 获取实际监听地址
	addr := server.GetListenAddress()
	host, port := parseAddr(addr)

	clientCfg := &TcpClientConfig{
		ServerAddr:    host,
		ServerPort:    port,
		AutoReconnect: false,
	}

	client := NewTcpClient(clientCfg)
	defer client.Close()

	if err := client.Connect(); err != nil {
		t.Fatalf("Client connect failed: %v", err)
	}

	testData := []byte("encrypted test message")
	if err := client.Send(100, testData); err != nil {
		t.Fatalf("Client send failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	t.Log("Client-Server encryption test passed")
}

func TestClientServerWithCompression(t *testing.T) {
	cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:0",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}

	server := NewTcpServer(cfg)
	defer server.Close()

	server.RegisterDispatcher(func(session Session, netPacket *NetPacket) error {
		t.Logf("Server received packet: ProtoId=%d, IsCompressed=%d, DataSize=%d",
			netPacket.ProtoId, netPacket.IsCompressed, netPacket.DataSize)
		return nil
	})

	go func() {
		if err := server.Start(); err != nil {
			t.Errorf("Server start failed: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// 获取实际监听地址
	addr := server.GetListenAddress()
	host, port := parseAddr(addr)

	clientCfg := &TcpClientConfig{
		DisableEncryption: true,
		ServerAddr:    host,
		ServerPort:    port,
		AutoReconnect: false,
		Compression: CompressionConfig{
			Enabled:              true,
			CompressionThreshold: 100,
			MaxCompressSize:      1024 * 1024,
		},
	}

	client := NewTcpClient(clientCfg)
	defer client.Close()

	if err := client.Connect(); err != nil {
		t.Fatalf("Client connect failed: %v", err)
	}

	testData := make([]byte, 1024)
	rand.Read(testData)

	if err := client.Send(100, testData); err != nil {
		t.Fatalf("Client send failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	t.Log("Client-Server compression test passed")
}

func TestClientServerPerformance(t *testing.T) {
	cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:0",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}

	server := NewTcpServer(cfg)
	defer server.Close()

	var receivedCount atomic.Int64
	server.RegisterDispatcher(func(session Session, netPacket *NetPacket) error {
		receivedCount.Add(1)
		return nil
	})

	go func() {
		if err := server.Start(); err != nil {
			t.Errorf("Server start failed: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// 获取实际监听地址
	addr := server.GetListenAddress()
	host, port := parseAddr(addr)

	clientCfg := &TcpClientConfig{
		DisableEncryption: true,
		ServerAddr:    host,
		ServerPort:    port,
		AutoReconnect: false,
	}

	client := NewTcpClient(clientCfg)
	defer client.Close()

	if err := client.Connect(); err != nil {
		t.Fatalf("Client connect failed: %v", err)
	}

	messageCount := 1000
	messageSize := 1024

	t.Logf("Testing performance: %d messages, %d bytes each", messageCount, messageSize)

	start := time.Now()
	for i := 0; i < messageCount; i++ {
		testData := make([]byte, messageSize)
		if err := client.Send(ProtoIdType(i+1), testData); err != nil {
			t.Fatalf("Client send failed at message %d: %v", i, err)
		}
	}
	elapsed := time.Since(start)

	time.Sleep(200 * time.Millisecond)

	if int(receivedCount.Load()) != messageCount {
		t.Errorf("Expected %d messages, received %d", messageCount, receivedCount.Load())
	}

	totalBytes := int64(messageCount * messageSize)
	throughput := float64(totalBytes) / elapsed.Seconds() / 1024 / 1024

	t.Logf("Total time: %v", elapsed)
	t.Logf("Throughput: %.2f MB/s", throughput)
	t.Logf("Messages per second: %.2f", float64(messageCount)/elapsed.Seconds())
}

func TestClientServerConcurrent(t *testing.T) {
	cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:0",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}

	server := NewTcpServer(cfg)
	defer server.Close()

	var receivedCount atomic.Int64
	server.RegisterDispatcher(func(session Session, netPacket *NetPacket) error {
		receivedCount.Add(1)
		return nil
	})

	go func() {
		if err := server.Start(); err != nil {
			t.Errorf("Server start failed: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// 获取实际监听地址
	addr := server.GetListenAddress()
	host, port := parseAddr(addr)

	clientCount := 10
	clients := make([]*TcpClient, clientCount)

	for i := 0; i < clientCount; i++ {
		clientCfg := &TcpClientConfig{
			DisableEncryption: true,
			ServerAddr:    host,
			ServerPort:    port,
			AutoReconnect: false,
		}

		client := NewTcpClient(clientCfg)
		if err := client.Connect(); err != nil {
			t.Fatalf("Client %d connect failed: %v", i, err)
		}

		clients[i] = client
		defer client.Close()

		t.Logf("Client %d connected", i)
	}

	messagesPerClient := 100
	for i := 0; i < messagesPerClient; i++ {
		for j, client := range clients {
			testData := []byte(fmt.Sprintf("client %d message %d", j, i))
			if err := client.Send(ProtoIdType(i+1), testData); err != nil {
				t.Fatalf("Client %d send failed at message %d: %v", j, i, err)
			}
		}
	}

	time.Sleep(500 * time.Millisecond)

	expectedMessages := clientCount * messagesPerClient
	if int(receivedCount.Load()) != expectedMessages {
		t.Errorf("Expected %d messages, received %d", expectedMessages, receivedCount.Load())
	}

	t.Logf("Concurrent test passed: %d clients, %d messages per client, total %d messages",
		clientCount, messagesPerClient, receivedCount.Load())
}

func TestClientServerSessionManagement(t *testing.T) {
	cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:0",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}

	server := NewTcpServer(cfg)
	defer server.Close()

	connectedSessions := make(map[SessionIdType]bool)
	disconnectedSessions := make(map[SessionIdType]bool)
	var sessionsMutex sync.Mutex

	server.SetOnAddSession(func(sid SessionIdType) {
		sessionsMutex.Lock()
		connectedSessions[sid] = true
		sessionsMutex.Unlock()
		t.Logf("Session connected: %d", sid)
	})

	server.SetOnRemoveSession(func(sid SessionIdType) {
		sessionsMutex.Lock()
		disconnectedSessions[sid] = true
		sessionsMutex.Unlock()
		t.Logf("Session disconnected: %d", sid)
	})

	server.RegisterDispatcher(func(session Session, netPacket *NetPacket) error {
		return nil
	})

	go func() {
		if err := server.Start(); err != nil {
			t.Errorf("Server start failed: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// 获取实际监听地址
	addr := server.GetListenAddress()
	host, port := parseAddr(addr)

	clientCount := 5
	clients := make([]*TcpClient, clientCount)
	sessionIDs := make([]SessionIdType, clientCount)

	for idx := 0; idx < clientCount; idx++ {
		clientCfg := &TcpClientConfig{
			DisableEncryption: true,
			ServerAddr:    host,
			ServerPort:    port,
			AutoReconnect: false,
		}

		client := NewTcpClient(clientCfg)
		if err := client.Connect(); err != nil {
			t.Fatalf("Client %d connect failed: %v", idx, err)
		}

		clients[idx] = client
		sessionIDs[idx] = client.GetSession().GetSid()
	}

	// 在 mutex 下读 map 长度（回调在其它 goroutine 持锁写入，裸 len() 读会数据竞争）；
	// 用 waitFor 替代固定 sleep，稳健等待回调完成。
	count := func(m map[SessionIdType]bool) int {
		sessionsMutex.Lock()
		defer sessionsMutex.Unlock()
		return len(m)
	}

	if !waitFor(t, 2*time.Second, func() bool { return count(connectedSessions) == clientCount }) {
		t.Errorf("Expected %d connected sessions, got %d", clientCount, count(connectedSessions))
	}

	for _, client := range clients {
		client.Close()
	}

	if !waitFor(t, 3*time.Second, func() bool { return count(disconnectedSessions) == clientCount }) {
		t.Errorf("Expected %d disconnected sessions, got %d", clientCount, count(disconnectedSessions))
	}

	t.Logf("Session management test passed: %d sessions connected and disconnected", clientCount)
}

func BenchmarkClientServerCommunication(b *testing.B) {
	cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:0",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}

	server := NewTcpServer(cfg)
	defer server.Close()

	server.RegisterDispatcher(func(session Session, netPacket *NetPacket) error {
		return nil
	})

	go func() {
		if err := server.Start(); err != nil {
			b.Errorf("Server start failed: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// 获取实际监听地址
	addr := server.GetListenAddress()
	host, port := parseAddr(addr)

	clientCfg := &TcpClientConfig{
		DisableEncryption: true,
		ServerAddr:    host,
		ServerPort:    port,
		AutoReconnect: false,
	}

	client := NewTcpClient(clientCfg)
	defer client.Close()

	if err := client.Connect(); err != nil {
		b.Fatalf("Client connect failed: %v", err)
	}

	testData := make([]byte, 1024)
	rand.Read(testData)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = client.Send(ProtoIdType(i+1), testData)
	}
}

func TestClientServerRealScenario(t *testing.T) {
	cfg := &TcpConfig{
		ListenAddress:       "127.0.0.1:0",
		MaxClientCount:      100,
		ChanSize:            1024,
		HeartbeatDuration:   30,
		MaxPacketDataSize:   1024 * 1024,
		DisableEncryption:   false,
		EnableKeyRotation:   true,
		KeyRotationInterval: 30 * time.Minute,
		MaxHistoryKeys:      3,
		EnableSequenceCheck: true,
		SequenceWindowSize:  1000,
		TimestampTolerance:  30,
	}

	server := NewTcpServer(cfg)
	defer server.Close()

	server.RegisterDispatcher(func(session Session, netPacket *NetPacket) error {
		t.Logf("Server received: ProtoId=%d, Seq=%d, KeyID=%d",
			netPacket.ProtoId, netPacket.Sequence, netPacket.KeyID)
		return nil
	})

	go func() {
		if err := server.Start(); err != nil {
			t.Errorf("Server start failed: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// 获取实际监听地址
	addr := server.GetListenAddress()
	host, port := parseAddr(addr)

	clientCfg := &TcpClientConfig{
		ServerAddr:    host,
		ServerPort:    port,
		AutoReconnect: false,
	}

	client := NewTcpClient(clientCfg)
	defer client.Close()

	if err := client.Connect(); err != nil {
		t.Fatalf("Client connect failed: %v", err)
	}

	t.Log("Testing real scenario with encryption, key rotation, and sequence checking")

	scenarios := []struct {
		name  string
		proto ProtoIdType
		data  []byte
	}{
		{"登录请求", 100, []byte(`{"username":"test","password":"123456"}`)},
		{"心跳包", HeartbeatProtoId, nil},
		{"数据同步", 101, make([]byte, 256)},
		{"业务消息", 102, []byte("business message")},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			if err := client.Send(scenario.proto, scenario.data); err != nil {
				t.Errorf("Send failed: %v", err)
			}
			time.Sleep(10 * time.Millisecond)
		})
	}

	time.Sleep(100 * time.Millisecond)

	t.Log("Real scenario test passed")
}

func TestClientServerConnectionFailure(t *testing.T) {
	clientCfg := &TcpClientConfig{
		DisableEncryption: true,
		ServerAddr:    "127.0.0.1",
		ServerPort:    9999,
		AutoReconnect: false,
	}

	client := NewTcpClient(clientCfg)

	if err := client.Connect(); err == nil {
		t.Error("Expected connection to fail")
	}

	t.Log("Connection failure test passed")
}

func TestClientServerReconnect(t *testing.T) {
	cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:0",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}

	server := NewTcpServer(cfg)
	defer server.Close()

	server.RegisterDispatcher(func(session Session, netPacket *NetPacket) error {
		return nil
	})

	go func() {
		if err := server.Start(); err != nil {
			t.Errorf("Server start failed: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// 获取实际监听地址
	addr := server.GetListenAddress()
	host, port := parseAddr(addr)

	clientCfg := &TcpClientConfig{
		DisableEncryption: true,
		ServerAddr:    host,
		ServerPort:    port,
		AutoReconnect: true,
	}

	client := NewTcpClient(clientCfg)
	defer client.Close()

	if err := client.Connect(); err != nil {
		t.Fatalf("Client connect failed: %v", err)
	}

	testData := []byte("test message")
	if err := client.Send(100, testData); err != nil {
		t.Fatalf("Client send failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	t.Log("Reconnect test passed")
}

// TestClientServerReconnect_RecoversAfterDrop 真正验证断线自动重连：强制关闭当前会话模拟
// 掉线，唯一的 monitorConnection 应检测到 IsClosed 并经重连循环重连到仍在运行的 server，
// 重连后可正常发送。回归点：旧实现每次重连都 spawn 新 monitor（goroutine 泄漏 + 多监控
// 重复重连），且单次 Connect 失败即放弃不重试；本测试确保修复后能稳定恢复。
func TestClientServerReconnect_RecoversAfterDrop(t *testing.T) {
	cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:0",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}
	server := NewTcpServer(cfg)
	defer server.Close()
	server.RegisterDispatcher(func(session Session, netPacket *NetPacket) error { return nil })
	go func() {
		if err := server.Start(); err != nil {
			t.Errorf("Server start failed: %v", err)
		}
	}()
	time.Sleep(100 * time.Millisecond)

	host, port := parseAddr(server.GetListenAddress())
	clientCfg := &TcpClientConfig{
		DisableEncryption: true,
		ServerAddr:        host,
		ServerPort:        port,
		AutoReconnect:     true,
		ReconnectDelay:    1,
	}
	client := NewTcpClient(clientCfg)
	defer client.Close()
	if err := client.Connect(); err != nil {
		t.Fatalf("client connect failed: %v", err)
	}

	// 强制断开当前会话，模拟网络掉线。
	oldSession := client.GetSession()
	oldSession.Close()

	// 阶段一：等待监控检测到掉线（状态离开 Connected）。若不先见证掉线，
	// 直接判 Connected 会命中掉线前的陈旧状态而假通过。
	// 注意：monitor 每 1s 轮询一次 IsClosed；`-race` 下整机负载重、定时器慢，故给足裕量
	// （避免测试自身时序假失败——已确认非数据竞争）。
	dropDeadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(dropDeadline) && client.GetState() == ClientStateConnected {
		time.Sleep(20 * time.Millisecond)
	}
	if client.GetState() == ClientStateConnected {
		t.Fatalf("monitor did not detect the drop (state still Connected)")
	}

	// 阶段二：等待自动重连恢复到 Connected。
	upDeadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(upDeadline) && client.GetState() != ClientStateConnected {
		time.Sleep(20 * time.Millisecond)
	}
	if client.GetState() != ClientStateConnected {
		t.Fatalf("client did not auto-reconnect, state=%d", client.GetState())
	}

	// 应确实换了新会话（旧会话已关闭），并能正常发送。
	if client.GetSession() == oldSession {
		t.Fatalf("expected a fresh session after reconnect")
	}
	if err := client.Send(100, []byte("after-reconnect")); err != nil {
		t.Fatalf("send after reconnect failed: %v", err)
	}
	t.Log("auto-reconnect after drop verified")
}
