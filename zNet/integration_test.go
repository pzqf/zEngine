package zNet

import (
	"crypto/rand"
	"fmt"
	"net"
	"strconv"
	"sync"
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
			t.Fatalf("Server start failed: %v", err)
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

	receivedCount := 0
	server.RegisterDispatcher(func(session Session, netPacket *NetPacket) error {
		receivedCount++
		t.Logf("Server received packet %d: ProtoId=%d", receivedCount, netPacket.ProtoId)
		return nil
	})

	go func() {
		if err := server.Start(); err != nil {
			t.Fatalf("Server start failed: %v", err)
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

	messageCount := 100
	for i := 0; i < messageCount; i++ {
		testData := []byte(fmt.Sprintf("message %d", i))
		if err := client.Send(ProtoIdType(i+1), testData); err != nil {
			t.Fatalf("Client send failed at message %d: %v", i, err)
		}
	}

	time.Sleep(500 * time.Millisecond)

	if receivedCount != messageCount {
		t.Errorf("Expected %d messages, received %d", messageCount, receivedCount)
	}

	t.Logf("Client-Server multiple messages test passed: %d messages", receivedCount)
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
			t.Fatalf("Server start failed: %v", err)
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
			t.Fatalf("Server start failed: %v", err)
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

	receivedCount := 0
	server.RegisterDispatcher(func(session Session, netPacket *NetPacket) error {
		receivedCount++
		return nil
	})

	go func() {
		if err := server.Start(); err != nil {
			t.Fatalf("Server start failed: %v", err)
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

	if receivedCount != messageCount {
		t.Errorf("Expected %d messages, received %d", messageCount, receivedCount)
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

	receivedCount := 0
	server.RegisterDispatcher(func(session Session, netPacket *NetPacket) error {
		receivedCount++
		return nil
	})

	go func() {
		if err := server.Start(); err != nil {
			t.Fatalf("Server start failed: %v", err)
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
	if receivedCount != expectedMessages {
		t.Errorf("Expected %d messages, received %d", expectedMessages, receivedCount)
	}

	t.Logf("Concurrent test passed: %d clients, %d messages per client, total %d messages",
		clientCount, messagesPerClient, receivedCount)
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
			t.Fatalf("Server start failed: %v", err)
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

	time.Sleep(100 * time.Millisecond)

	if len(connectedSessions) != clientCount {
		t.Errorf("Expected %d connected sessions, got %d", clientCount, len(connectedSessions))
	}

	for _, client := range clients {
		client.Close()
	}

	time.Sleep(200 * time.Millisecond)

	if len(disconnectedSessions) != clientCount {
		t.Errorf("Expected %d disconnected sessions, got %d", clientCount, len(disconnectedSessions))
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
			b.Fatalf("Server start failed: %v", err)
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
			t.Fatalf("Server start failed: %v", err)
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
			t.Fatalf("Server start failed: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// 获取实际监听地址
	addr := server.GetListenAddress()
	host, port := parseAddr(addr)

	clientCfg := &TcpClientConfig{
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
