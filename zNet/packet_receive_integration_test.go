package zNet

import (
	"encoding/binary"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type receivedPacket struct {
	protoID ProtoIdType
	data    string
}

func waitReceivedPacket(t *testing.T, ch <-chan receivedPacket, wantProto ProtoIdType, wantData string) {
	t.Helper()
	select {
	case got := <-ch:
		if got.protoID != wantProto || got.data != wantData {
			t.Fatalf("received packet = %+v, want proto=%d data=%q", got, wantProto, wantData)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for proto=%d data=%q", wantProto, wantData)
	}
}

func capturePackets(ch chan<- receivedPacket) HandlerFun {
	return func(_ Session, packet *NetPacket) error {
		ch <- receivedPacket{protoID: packet.ProtoId, data: string(packet.Data)}
		return nil
	}
}

func TestTCPReceivePathsUseEndpointCodec(t *testing.T) {
	serverReceived := make(chan receivedPacket, 1)
	clientReceived := make(chan receivedPacket, 1)
	recorder := &detailedDecodeRecorder{}
	server := NewTcpServer(&TcpConfig{
		ListenAddress:        "127.0.0.1:0",
		ChanSize:             8,
		MaxPacketDataSize:    64,
		MaxDecodedPacketSize: 64,
		DisableEncryption:    true,
		ByteOrder:            WireByteOrderBig,
	}, WithServerMetrics(recorder))
	server.RegisterDispatcher(capturePackets(serverReceived))
	if err := server.Start(); err != nil {
		t.Fatalf("server.Start() error = %v", err)
	}
	t.Cleanup(server.Close)

	host, portText, err := net.SplitHostPort(server.GetListenAddress())
	if err != nil {
		t.Fatal(err)
	}
	port, err := net.LookupPort("tcp", portText)
	if err != nil {
		t.Fatal(err)
	}
	client := NewTcpClient(&TcpClientConfig{
		ServerAddr:           host,
		ServerPort:           port,
		MaxPacketDataSize:    64,
		MaxDecodedPacketSize: 64,
		DisableEncryption:    true,
		ByteOrder:            WireByteOrderBig,
	})
	client.RegisterDispatcher(capturePackets(clientReceived))
	if err := client.Connect(); err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	t.Cleanup(client.Close)

	codec := NewPacketCodec(binary.BigEndian)
	clientBomb := compressedTestPacket(100, 256)
	if _, err := client.GetSession().conn.Write(codec.Marshal(&clientBomb)); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && recorder.decodedOversize.Load() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if recorder.decodedOversize.Load() != 1 {
		t.Fatalf("TCP server decoded oversize errors = %d, want 1", recorder.decodedOversize.Load())
	}
	select {
	case got := <-serverReceived:
		t.Fatalf("TCP server dispatched decoded-oversize packet: %+v", got)
	default:
	}

	if err := client.Send(101, []byte("client-to-server")); err != nil {
		t.Fatal(err)
	}
	waitReceivedPacket(t, serverReceived, 101, "client-to-server")

	var serverSession *TcpServerSession
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		sessions := server.GetAllSession()
		if len(sessions) == 1 {
			serverSession = sessions[0]
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if serverSession == nil {
		t.Fatal("server session was not registered")
	}
	serverBomb := compressedTestPacket(103, 256)
	if _, err := serverSession.send(&serverBomb); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-clientReceived:
		t.Fatalf("TCP client dispatched decoded-oversize packet: %+v", got)
	case <-time.After(100 * time.Millisecond):
	}
	if err := serverSession.Send(102, []byte("server-to-client")); err != nil {
		t.Fatal(err)
	}
	waitReceivedPacket(t, clientReceived, 102, "server-to-client")
}

func TestTCPRejectsOversizeHeaderBeforeReadingBody(t *testing.T) {
	recorder := &detailedDecodeRecorder{}
	server := NewTcpServer(&TcpConfig{
		ListenAddress:     "127.0.0.1:0",
		ChanSize:          8,
		MaxPacketDataSize: 8,
		DisableEncryption: true,
		ByteOrder:         WireByteOrderBig,
	}, WithServerMetrics(recorder))
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Close)

	conn, err := net.Dial("tcp", server.GetListenAddress())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	codec := NewPacketCodec(binary.BigEndian)
	header := codec.Marshal(&NetPacket{ProtoId: 1, DataSize: 1 << 30})[:NetPacketHeadSize]
	if _, err := conn.Write(header); err != nil {
		t.Fatal(err)
	}

	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var oneByte [1]byte
	if _, err := conn.Read(oneByte[:]); err == nil {
		t.Fatal("server kept TCP connection open after an invalid length header")
	}
	if recorder.decodeErr.Load() != 1 {
		t.Fatalf("decode errors = %d, want 1", recorder.decodeErr.Load())
	}
	if recorder.wireOversize.Load() != 1 {
		t.Fatalf("wire oversize errors = %d, want 1", recorder.wireOversize.Load())
	}
}

func TestUDPReceivePathsRejectBadFrameAndContinue(t *testing.T) {
	serverReceived := make(chan receivedPacket, 64)
	clientReceived := make(chan receivedPacket, 1)
	recorder := &detailedDecodeRecorder{}
	server := NewUdpServer(&UdpConfig{
		ListenAddress:        "127.0.0.1:0",
		ChanSize:             8,
		MaxPacketDataSize:    64,
		MaxDecodedPacketSize: 64,
		ByteOrder:            WireByteOrderBig,
	}, WithServerMetrics(recorder))
	server.RegisterDispatcher(capturePackets(serverReceived))
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Close)

	serverAddr := server.listener.LocalAddr().(*net.UDPAddr)
	conn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		t.Fatal(err)
	}
	client := &UdpClient{
		dispatcher:           capturePackets(clientReceived),
		maxPacketDataSize:    64,
		maxDecodedPacketSize: 64,
		packetCodec:          NewPacketCodec(binary.BigEndian),
	}
	clientSession := &UdpClientSession{}
	clientSession.Init(client, conn)
	clientSession.dhExchange = nil
	clientSession.keyExchanged = true
	client.session = clientSession
	clientSession.Start()
	t.Cleanup(func() {
		_ = conn.Close()
		clientSession.Close()
	})

	serverSession := NewUdpServerSession(server, conn.LocalAddr().(*net.UDPAddr), 1, nil, nil, nil)
	server.clientSessionMap.Store(serverSession.sid, serverSession)
	serverSession.Start()

	codec := NewPacketCodec(binary.BigEndian)
	badFrame := codec.Marshal(&NetPacket{ProtoId: 0})
	if _, err := conn.Write(badFrame); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && recorder.decodeErr.Load() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if recorder.decodeErr.Load() != 1 {
		t.Fatalf("decode errors = %d, want 1", recorder.decodeErr.Load())
	}
	bomb := compressedTestPacket(200, 256)
	if _, err := conn.Write(codec.Marshal(&bomb)); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && recorder.decodedOversize.Load() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if recorder.decodedOversize.Load() != 1 {
		t.Fatalf("UDP server decoded oversize errors = %d, want 1", recorder.decodedOversize.Load())
	}
	select {
	case got := <-serverReceived:
		t.Fatalf("UDP server dispatched decoded-oversize packet: %+v", got)
	default:
	}

	if err := clientSession.Send(201, []byte("client-to-server")); err != nil {
		t.Fatal(err)
	}
	waitReceivedPacket(t, serverReceived, 201, "client-to-server")

	const burstPackets = 32
	for i := 0; i < burstPackets; i++ {
		payload := []byte{byte(i)}
		frame := codec.Marshal(&NetPacket{ProtoId: 210, DataSize: 1, Data: payload})
		if _, err := conn.Write(frame); err != nil {
			t.Fatal(err)
		}
	}
	seen := make(map[byte]bool, burstPackets)
	for i := 0; i < burstPackets; i++ {
		select {
		case got := <-serverReceived:
			if got.protoID != 210 || len(got.data) != 1 {
				t.Fatalf("burst packet = %+v", got)
			}
			seen[got.data[0]] = true
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out after %d/%d UDP burst packets", i, burstPackets)
		}
	}
	if len(seen) != burstPackets {
		t.Fatalf("UDP burst preserved %d/%d distinct payloads", len(seen), burstPackets)
	}

	if _, err := server.listener.WriteToUDP(badFrame, serverSession.addr); err != nil {
		t.Fatal(err)
	}
	if _, err := server.listener.WriteToUDP(codec.Marshal(&bomb), serverSession.addr); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-clientReceived:
		t.Fatalf("UDP client dispatched decoded-oversize packet: %+v", got)
	case <-time.After(100 * time.Millisecond):
	}
	if err := serverSession.Send(202, []byte("server-to-client")); err != nil {
		t.Fatal(err)
	}
	waitReceivedPacket(t, clientReceived, 202, "server-to-client")
}

func TestWebSocketReceivePathsRejectBadFrameAndContinue(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	accepted := make(chan *websocket.Conn, 1)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		accepted <- conn
	}))
	defer httpServer.Close()

	clientConn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(httpServer.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	serverConn := <-accepted

	serverReceived := make(chan receivedPacket, 1)
	clientReceived := make(chan receivedPacket, 1)
	recorder := &detailedDecodeRecorder{}
	server := NewWebSocketServer(&WebSocketConfig{
		ChanSize:             8,
		MaxPacketDataSize:    64,
		MaxDecodedPacketSize: 64,
		ByteOrder:            WireByteOrderBig,
	}, WithServerMetrics(recorder))
	server.RegisterDispatcher(capturePackets(serverReceived))
	serverSession := NewWebSocketServerSession(server, serverConn, 1, nil, nil)
	serverSession.Start()

	client := &WebSocketClient{
		dispatcher:           capturePackets(clientReceived),
		maxPacketDataSize:    64,
		maxDecodedPacketSize: 64,
		packetCodec:          NewPacketCodec(binary.BigEndian),
	}
	clientSession := &WebSocketClientSession{
		conn:        clientConn,
		sid:         1,
		cli:         client,
		packetCodec: client.packetCodec,
	}
	client.session = clientSession
	clientSession.Start()
	t.Cleanup(func() {
		_ = clientConn.Close()
		clientSession.Close()
		serverSession.Close()
	})

	codec := NewPacketCodec(binary.BigEndian)
	badFrame := codec.Marshal(&NetPacket{ProtoId: 0})
	if err := clientConn.WriteMessage(websocket.BinaryMessage, badFrame); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && recorder.decodeErr.Load() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if recorder.decodeErr.Load() != 1 {
		t.Fatalf("decode errors = %d, want 1", recorder.decodeErr.Load())
	}
	bomb := compressedTestPacket(300, 256)
	if err := clientConn.WriteMessage(websocket.BinaryMessage, codec.Marshal(&bomb)); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && recorder.decodedOversize.Load() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if recorder.decodedOversize.Load() != 1 {
		t.Fatalf("WebSocket server decoded oversize errors = %d, want 1", recorder.decodedOversize.Load())
	}
	select {
	case got := <-serverReceived:
		t.Fatalf("WebSocket server dispatched decoded-oversize packet: %+v", got)
	default:
	}

	if err := clientSession.Send(301, []byte("client-to-server")); err != nil {
		t.Fatal(err)
	}
	waitReceivedPacket(t, serverReceived, 301, "client-to-server")

	if err := serverConn.WriteMessage(websocket.BinaryMessage, badFrame); err != nil {
		t.Fatal(err)
	}
	if err := serverConn.WriteMessage(websocket.BinaryMessage, codec.Marshal(&bomb)); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-clientReceived:
		t.Fatalf("WebSocket client dispatched decoded-oversize packet: %+v", got)
	case <-time.After(100 * time.Millisecond):
	}
	if err := serverSession.Send(302, []byte("server-to-client")); err != nil {
		t.Fatal(err)
	}
	waitReceivedPacket(t, clientReceived, 302, "server-to-client")
}
