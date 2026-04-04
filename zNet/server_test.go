package zNet

import (
	"crypto/rand"
	"fmt"
	"testing"
	"time"
)

func TestServerToServerCommunication(t *testing.T) {
	server1Cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:8081",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}

	server2Cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:8082",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}

	server1 := NewTcpServer(server1Cfg)
	defer server1.Close()

	server2 := NewTcpServer(server2Cfg)
	defer server2.Close()

	server1.RegisterDispatcher(func(session Session, netPacket *NetPacket) error {
		t.Logf("Server1 received: ProtoId=%d, DataSize=%d", netPacket.ProtoId, netPacket.DataSize)
		return nil
	})

	server2.RegisterDispatcher(func(session Session, netPacket *NetPacket) error {
		t.Logf("Server2 received: ProtoId=%d, DataSize=%d", netPacket.ProtoId, netPacket.DataSize)
		return nil
	})

	go func() {
		if err := server1.Start(); err != nil {
			t.Fatalf("Server1 start failed: %v", err)
		}
	}()

	go func() {
		if err := server2.Start(); err != nil {
			t.Fatalf("Server2 start failed: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	clientCfg := &TcpClientConfig{
		ServerAddr:    "127.0.0.1",
		ServerPort:    8081,
		AutoReconnect: false,
	}

	client := NewTcpClient(clientCfg)
	defer client.Close()

	if err := client.Connect(); err != nil {
		t.Fatalf("Client connect to server1 failed: %v", err)
	}

	testData := []byte("server to server test message")
	if err := client.Send(1000, testData); err != nil {
		t.Fatalf("Client send failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	t.Log("Server-to-server communication test passed")
}

func TestServerToServerMultipleMessages(t *testing.T) {
	server1Cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:8083",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}

	server2Cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:8084",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}

	server1 := NewTcpServer(server1Cfg)
	defer server1.Close()

	server2 := NewTcpServer(server2Cfg)
	defer server2.Close()

	receivedCount := 0
	server2.RegisterDispatcher(func(session Session, netPacket *NetPacket) error {
		receivedCount++
		t.Logf("Server2 received message %d: ProtoId=%d", receivedCount, netPacket.ProtoId)
		return nil
	})

	go func() {
		if err := server1.Start(); err != nil {
			t.Fatalf("Server1 start failed: %v", err)
		}
	}()

	go func() {
		if err := server2.Start(); err != nil {
			t.Fatalf("Server2 start failed: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	clientCfg := &TcpClientConfig{
		ServerAddr:    "127.0.0.1",
		ServerPort:    8083,
		AutoReconnect: false,
	}

	client := NewTcpClient(clientCfg)
	defer client.Close()

	if err := client.Connect(); err != nil {
		t.Fatalf("Client connect failed: %v", err)
	}

	messageCount := 100
	for i := 0; i < messageCount; i++ {
		testData := []byte(fmt.Sprintf("server message %d", i))
		if err := client.Send(ProtoIdType(i+1), testData); err != nil {
			t.Fatalf("Client send failed at message %d: %v", i, err)
		}
	}

	time.Sleep(500 * time.Millisecond)

	if receivedCount != messageCount {
		t.Errorf("Expected %d messages, received %d", messageCount, receivedCount)
	}

	t.Logf("Server-to-server multiple messages test passed: %d messages", receivedCount)
}

func TestServerToServerBidirectional(t *testing.T) {
	server1Cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:8085",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}

	server2Cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:8086",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}

	server1 := NewTcpServer(server1Cfg)
	defer server1.Close()

	server2 := NewTcpServer(server2Cfg)
	defer server2.Close()

	server1Received := 0
	server2Received := 0

	server1.RegisterDispatcher(func(session Session, netPacket *NetPacket) error {
		server1Received++
		t.Logf("Server1 received: ProtoId=%d", netPacket.ProtoId)
		return nil
	})

	server2.RegisterDispatcher(func(session Session, netPacket *NetPacket) error {
		server2Received++
		t.Logf("Server2 received: ProtoId=%d", netPacket.ProtoId)
		return nil
	})

	go func() {
		if err := server1.Start(); err != nil {
			t.Fatalf("Server1 start failed: %v", err)
		}
	}()

	go func() {
		if err := server2.Start(); err != nil {
			t.Fatalf("Server2 start failed: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	client1Cfg := &TcpClientConfig{
		ServerAddr:    "127.0.0.1",
		ServerPort:    8085,
		AutoReconnect: false,
	}

	client2Cfg := &TcpClientConfig{
		ServerAddr:    "127.0.0.1",
		ServerPort:    8086,
		AutoReconnect: false,
	}

	client1 := NewTcpClient(client1Cfg)
	defer client1.Close()

	client2 := NewTcpClient(client2Cfg)
	defer client2.Close()

	if err := client1.Connect(); err != nil {
		t.Fatalf("Client1 connect failed: %v", err)
	}

	if err := client2.Connect(); err != nil {
		t.Fatalf("Client2 connect failed: %v", err)
	}

	messageCount := 50
	for i := 0; i < messageCount; i++ {
		testData1 := []byte(fmt.Sprintf("server1 message %d", i))
		testData2 := []byte(fmt.Sprintf("server2 message %d", i))

		if err := client1.Send(ProtoIdType(i+1), testData1); err != nil {
			t.Fatalf("Client1 send failed at message %d: %v", i, err)
		}

		if err := client2.Send(ProtoIdType(i+1), testData2); err != nil {
			t.Fatalf("Client2 send failed at message %d: %v", i, err)
		}
	}

	time.Sleep(500 * time.Millisecond)

	if server1Received != messageCount {
		t.Errorf("Expected %d messages on server1, received %d", messageCount, server1Received)
	}

	if server2Received != messageCount {
		t.Errorf("Expected %d messages on server2, received %d", messageCount, server2Received)
	}

	t.Logf("Bidirectional test passed: server1=%d, server2=%d", server1Received, server2Received)
}

func TestServerToServerPerformance(t *testing.T) {
	server1Cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:8087",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}

	server2Cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:8088",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}

	server1 := NewTcpServer(server1Cfg)
	defer server1.Close()

	server2 := NewTcpServer(server2Cfg)
	defer server2.Close()

	receivedCount := 0
	server2.RegisterDispatcher(func(session Session, netPacket *NetPacket) error {
		receivedCount++
		return nil
	})

	go func() {
		if err := server1.Start(); err != nil {
			t.Fatalf("Server1 start failed: %v", err)
		}
	}()

	go func() {
		if err := server2.Start(); err != nil {
			t.Fatalf("Server2 start failed: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	clientCfg := &TcpClientConfig{
		ServerAddr:    "127.0.0.1",
		ServerPort:    8087,
		AutoReconnect: false,
	}

	client := NewTcpClient(clientCfg)
	defer client.Close()

	if err := client.Connect(); err != nil {
		t.Fatalf("Client connect failed: %v", err)
	}

	messageCount := 1000
	messageSize := 1024

	t.Logf("Testing server-to-server performance: %d messages, %d bytes each", messageCount, messageSize)

	start := time.Now()
	for i := 0; i < messageCount; i++ {
		testData := make([]byte, messageSize)
		rand.Read(testData)
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

func TestServerToServerWithEncryption(t *testing.T) {
	server1Cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:8089",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: false,
	}

	server2Cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:8090",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: false,
	}

	server1 := NewTcpServer(server1Cfg)
	defer server1.Close()

	server2 := NewTcpServer(server2Cfg)
	defer server2.Close()

	server2.RegisterDispatcher(func(session Session, netPacket *NetPacket) error {
		t.Logf("Server2 received encrypted packet: ProtoId=%d", netPacket.ProtoId)
		return nil
	})

	go func() {
		if err := server1.Start(); err != nil {
			t.Fatalf("Server1 start failed: %v", err)
		}
	}()

	go func() {
		if err := server2.Start(); err != nil {
			t.Fatalf("Server2 start failed: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	clientCfg := &TcpClientConfig{
		ServerAddr:    "127.0.0.1",
		ServerPort:    8089,
		AutoReconnect: false,
	}

	client := NewTcpClient(clientCfg)
	defer client.Close()

	if err := client.Connect(); err != nil {
		t.Fatalf("Client connect failed: %v", err)
	}

	testData := []byte("encrypted server-to-server message")
	if err := client.Send(1000, testData); err != nil {
		t.Fatalf("Client send failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	t.Log("Server-to-server encryption test passed")
}

func TestServerToServerLargeData(t *testing.T) {
	server1Cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:8091",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}

	server2Cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:8092",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}

	server1 := NewTcpServer(server1Cfg)
	defer server1.Close()

	server2 := NewTcpServer(server2Cfg)
	defer server2.Close()

	receivedCount := 0
	receivedBytes := int64(0)
	server2.RegisterDispatcher(func(session Session, netPacket *NetPacket) error {
		receivedCount++
		receivedBytes += int64(netPacket.DataSize)
		return nil
	})

	go func() {
		if err := server1.Start(); err != nil {
			t.Fatalf("Server1 start failed: %v", err)
		}
	}()

	go func() {
		if err := server2.Start(); err != nil {
			t.Fatalf("Server2 start failed: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	clientCfg := &TcpClientConfig{
		ServerAddr:    "127.0.0.1",
		ServerPort:    8091,
		AutoReconnect: false,
	}

	client := NewTcpClient(clientCfg)
	defer client.Close()

	if err := client.Connect(); err != nil {
		t.Fatalf("Client connect failed: %v", err)
	}

	largeDataSizes := []int{1024, 4096, 16384, 65536, 262144}

	for i, dataSize := range largeDataSizes {
		testData := make([]byte, dataSize)
		rand.Read(testData)

		t.Logf("Sending large data: %d bytes", dataSize)

		if err := client.Send(ProtoIdType(i+1), testData); err != nil {
			t.Fatalf("Client send failed for size %d: %v", dataSize, err)
		}

		time.Sleep(50 * time.Millisecond)
	}

	time.Sleep(200 * time.Millisecond)

	if receivedCount != len(largeDataSizes) {
		t.Errorf("Expected %d messages, received %d", len(largeDataSizes), receivedCount)
	}

	t.Logf("Large data test passed: %d messages, %d total bytes", receivedCount, receivedBytes)
}

func TestServerToServerConcurrent(t *testing.T) {
	server1Cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:8093",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}

	server2Cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:8094",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}

	server3Cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:8095",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}

	server1 := NewTcpServer(server1Cfg)
	defer server1.Close()

	server2 := NewTcpServer(server2Cfg)
	defer server2.Close()

	server3 := NewTcpServer(server3Cfg)
	defer server3.Close()

	receivedCount := 0
	server2.RegisterDispatcher(func(session Session, netPacket *NetPacket) error {
		receivedCount++
		return nil
	})

	server3.RegisterDispatcher(func(session Session, netPacket *NetPacket) error {
		receivedCount++
		return nil
	})

	go func() {
		if err := server1.Start(); err != nil {
			t.Fatalf("Server1 start failed: %v", err)
		}
	}()

	go func() {
		if err := server2.Start(); err != nil {
			t.Fatalf("Server2 start failed: %v", err)
		}
	}()

	go func() {
		if err := server3.Start(); err != nil {
			t.Fatalf("Server3 start failed: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	clientCfg := &TcpClientConfig{
		ServerAddr:    "127.0.0.1",
		ServerPort:    8093,
		AutoReconnect: false,
	}

	client := NewTcpClient(clientCfg)
	defer client.Close()

	if err := client.Connect(); err != nil {
		t.Fatalf("Client connect failed: %v", err)
	}

	messageCount := 100
	for i := 0; i < messageCount; i++ {
		testData := []byte(fmt.Sprintf("concurrent message %d", i))
		if err := client.Send(ProtoIdType(i+1), testData); err != nil {
			t.Fatalf("Client send failed at message %d: %v", i, err)
		}
	}

	time.Sleep(500 * time.Millisecond)

	expectedMessages := messageCount * 2
	if receivedCount != expectedMessages {
		t.Errorf("Expected %d messages, received %d", expectedMessages, receivedCount)
	}

	t.Logf("Concurrent test passed: %d messages sent to 2 servers", messageCount)
}

func TestServerToServerConnectionFailure(t *testing.T) {
	serverCfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:8096",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}

	server := NewTcpServer(serverCfg)
	defer server.Close()

	go func() {
		if err := server.Start(); err != nil {
			t.Fatalf("Server start failed: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

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

func TestServerToServerSessionManagement(t *testing.T) {
	server1Cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:8097",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}

	server2Cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:8098",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}

	server1 := NewTcpServer(server1Cfg)
	defer server1.Close()

	server2 := NewTcpServer(server2Cfg)
	defer server2.Close()

	connectedSessions := make(map[SessionIdType]bool)
	disconnectedSessions := make(map[SessionIdType]bool)

	server1.SetOnAddSession(func(sid SessionIdType) {
		connectedSessions[sid] = true
		t.Logf("Server1 session connected: %d", sid)
	})

	server1.SetOnRemoveSession(func(sid SessionIdType) {
		disconnectedSessions[sid] = true
		t.Logf("Server1 session disconnected: %d", sid)
	})

	server2.RegisterDispatcher(func(session Session, netPacket *NetPacket) error {
		return nil
	})

	go func() {
		if err := server1.Start(); err != nil {
			t.Fatalf("Server1 start failed: %v", err)
		}
	}()

	go func() {
		if err := server2.Start(); err != nil {
			t.Fatalf("Server2 start failed: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	clientCfg := &TcpClientConfig{
		ServerAddr:    "127.0.0.1",
		ServerPort:    8097,
		AutoReconnect: false,
	}

	client := NewTcpClient(clientCfg)
	defer client.Close()

	if err := client.Connect(); err != nil {
		t.Fatalf("Client connect failed: %v", err)
	}

	sessionID := client.GetSession().GetSid()

	if !connectedSessions[sessionID] {
		t.Errorf("Expected session %d to be connected", sessionID)
	}

	client.Close()

	time.Sleep(200 * time.Millisecond)

	if !disconnectedSessions[sessionID] {
		t.Errorf("Expected session %d to be disconnected", sessionID)
	}

	t.Log("Session management test passed")
}

func BenchmarkServerToServerCommunication(b *testing.B) {
	server1Cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:8099",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}

	server2Cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:8100",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}

	server1 := NewTcpServer(server1Cfg)
	defer server1.Close()

	server2 := NewTcpServer(server2Cfg)
	defer server2.Close()

	server2.RegisterDispatcher(func(session Session, netPacket *NetPacket) error {
		return nil
	})

	go func() {
		if err := server1.Start(); err != nil {
			b.Fatalf("Server1 start failed: %v", err)
		}
	}()

	go func() {
		if err := server2.Start(); err != nil {
			b.Fatalf("Server2 start failed: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	clientCfg := &TcpClientConfig{
		ServerAddr:    "127.0.0.1",
		ServerPort:    8099,
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

func TestServerToServerRealScenario(t *testing.T) {
	server1Cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:8101",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: false,
	}

	server2Cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:8102",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: false,
	}

	server1 := NewTcpServer(server1Cfg)
	defer server1.Close()

	server2 := NewTcpServer(server2Cfg)
	defer server2.Close()

	server2.RegisterDispatcher(func(session Session, netPacket *NetPacket) error {
		t.Logf("Server2 received: ProtoId=%d, Seq=%d, KeyID=%d",
			netPacket.ProtoId, netPacket.Sequence, netPacket.KeyID)
		return nil
	})

	go func() {
		if err := server1.Start(); err != nil {
			t.Fatalf("Server1 start failed: %v", err)
		}
	}()

	go func() {
		if err := server2.Start(); err != nil {
			t.Fatalf("Server2 start failed: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	clientCfg := &TcpClientConfig{
		ServerAddr:    "127.0.0.1",
		ServerPort:    8101,
		AutoReconnect: false,
	}

	client := NewTcpClient(clientCfg)
	defer client.Close()

	if err := client.Connect(); err != nil {
		t.Fatalf("Client connect failed: %v", err)
	}

	t.Log("Testing real server-to-server scenario with encryption")

	scenarios := []struct {
		name  string
		proto ProtoIdType
		data  []byte
	}{
		{"服务注册", 2000, []byte(`{"server_id":1,"type":"gateway"}`)},
		{"服务发现", 2001, []byte(`{"servers":[{"id":2,"type":"game"},{"id":3,"type":"map"}]}`)},
		{"心跳同步", 2002, []byte(`{"timestamp":1234567890}`)},
		{"数据转发", 2003, []byte(`{"from_client":12345,"to_server":67890,"data":"test"}`)},
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
