package zNet

import (
	"errors"
	"testing"
	"time"
)

func TestTCPProtocolNegotiationAndPacketStamping(t *testing.T) {
	serverPolicy := ProtocolVersionPolicy{
		Enabled: true, MinVersion: 1, MaxVersion: 2, Capabilities: 0b1110, AcceptLegacy: true,
	}
	clientPolicy := ProtocolVersionPolicy{
		Enabled: true, MinVersion: 2, MaxVersion: 3, Capabilities: 0b1011, AcceptLegacy: true,
	}
	serverPackets := make(chan NetPacket, 1)
	clientPackets := make(chan NetPacket, 1)
	serverSessions := make(chan *TcpServerSession, 1)

	var server *TcpServer
	serverConfig := &TcpConfig{
		ListenAddress: "127.0.0.1:0", ChanSize: 32,
		ProtocolVersion: serverPolicy,
	}
	server = NewTcpServer(serverConfig, WithAddSessionCallBack(func(sid SessionIdType) {
		serverSessions <- server.GetSession(sid)
	}))
	serverConfig.ProtocolVersion = ProtocolVersionPolicy{}
	server.RegisterDispatcher(func(session Session, packet *NetPacket) error {
		serverPackets <- cloneNetPacket(*packet)
		return session.Send(202, []byte("server-response"))
	})
	startProtocolTestServer(t, server)

	host, port := parseAddr(server.GetListenAddress())
	clientConfig := &TcpClientConfig{
		ServerAddr: host, ServerPort: port,
		ProtocolVersion: clientPolicy, ProtocolNegotiationTimeout: time.Second,
	}
	client := NewTcpClient(clientConfig)
	clientConfig.ProtocolVersion = ProtocolVersionPolicy{}
	clientConfig.ProtocolNegotiationTimeout = time.Nanosecond
	client.RegisterDispatcher(func(_ Session, packet *NetPacket) error {
		clientPackets <- cloneNetPacket(*packet)
		return nil
	})
	if err := client.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(client.Close)

	clientCompatibility, ok := client.GetSession().ProtocolCompatibility()
	want := ProtocolCompatibility{Version: 2, Capabilities: 0b1010}
	if !ok || clientCompatibility != want {
		t.Fatalf("client compatibility = (%+v, %v), want (%+v, true)", clientCompatibility, ok, want)
	}

	serverSession := receiveProtocolTestValue(t, serverSessions)
	if !waitFor(t, time.Second, func() bool {
		compatibility, established := serverSession.ProtocolCompatibility()
		return established && compatibility == want
	}) {
		compatibility, established := serverSession.ProtocolCompatibility()
		t.Fatalf("server compatibility = (%+v, %v), want (%+v, true)", compatibility, established, want)
	}

	if err := client.Send(201, []byte("client-request")); err != nil {
		t.Fatalf("client Send() error = %v", err)
	}
	serverPacket := receiveProtocolTestValue(t, serverPackets)
	if serverPacket.ProtoId != 201 || serverPacket.Version != want.Version || string(serverPacket.Data) != "client-request" {
		t.Fatalf("server packet = proto:%d version:%d data:%q", serverPacket.ProtoId, serverPacket.Version, serverPacket.Data)
	}
	clientPacket := receiveProtocolTestValue(t, clientPackets)
	if clientPacket.ProtoId != 202 || clientPacket.Version != want.Version || string(clientPacket.Data) != "server-response" {
		t.Fatalf("client packet = proto:%d version:%d data:%q", clientPacket.ProtoId, clientPacket.Version, clientPacket.Data)
	}
}

func TestTCPProtocolNegotiationPrecedesWorkerPoolPackets(t *testing.T) {
	policy := ProtocolVersionPolicy{
		Enabled: true, MinVersion: 1, MaxVersion: 2, Capabilities: 1,
	}
	packets := make(chan NetPacket, 1)
	server := NewTcpServer(&TcpConfig{
		ListenAddress: "127.0.0.1:0", ChanSize: 32, DisableEncryption: true,
		UseWorkerPool: true, WorkerPoolSize: 2, WorkerQueueSize: 8,
		ProtocolVersion: policy,
	})
	server.RegisterDispatcher(func(_ Session, packet *NetPacket) error {
		packets <- cloneNetPacket(*packet)
		return nil
	})
	startProtocolTestServer(t, server)

	host, port := parseAddr(server.GetListenAddress())
	client := NewTcpClient(&TcpClientConfig{
		ServerAddr: host, ServerPort: port, DisableEncryption: true,
		ProtocolVersion: policy, ProtocolNegotiationTimeout: time.Second,
	})
	client.RegisterDispatcher(func(Session, *NetPacket) error { return nil })
	if err := client.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(client.Close)
	if err := client.Send(211, []byte("after-handshake")); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	packet := receiveProtocolTestValue(t, packets)
	if packet.ProtoId != 211 || packet.Version != 2 || string(packet.Data) != "after-handshake" {
		t.Fatalf("worker packet = proto:%d version:%d data:%q", packet.ProtoId, packet.Version, packet.Data)
	}
}

func TestTCPNewClientFallsBackToLegacyServer(t *testing.T) {
	serverPackets := make(chan NetPacket, 2)
	server := NewTcpServer(&TcpConfig{
		ListenAddress: "127.0.0.1:0", ChanSize: 32, DisableEncryption: true,
	})
	server.RegisterDispatcher(func(_ Session, packet *NetPacket) error {
		serverPackets <- cloneNetPacket(*packet)
		return nil
	})
	startProtocolTestServer(t, server)

	host, port := parseAddr(server.GetListenAddress())
	client := NewTcpClient(&TcpClientConfig{
		ServerAddr: host, ServerPort: port, DisableEncryption: true,
		ProtocolVersion: ProtocolVersionPolicy{
			Enabled: true, MinVersion: 1, MaxVersion: 2, Capabilities: 1, AcceptLegacy: true,
		},
		ProtocolNegotiationTimeout: 50 * time.Millisecond,
	})
	client.RegisterDispatcher(func(Session, *NetPacket) error { return nil })
	if err := client.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(client.Close)

	compatibility, ok := client.GetSession().ProtocolCompatibility()
	if !ok || !compatibility.IsLegacy() || compatibility.Capabilities != 0 {
		t.Fatalf("client compatibility = (%+v, %v), want established legacy", compatibility, ok)
	}
	hello := receiveProtocolTestValue(t, serverPackets)
	if hello.ProtoId != ProtocolNegotiationProtoId || hello.Version != 2 {
		t.Fatalf("legacy server received hello proto=%d version=%d", hello.ProtoId, hello.Version)
	}

	if err := client.Send(301, []byte("legacy-request")); err != nil {
		t.Fatalf("client Send() error = %v", err)
	}
	request := receiveProtocolTestValue(t, serverPackets)
	if request.ProtoId != 301 || request.Version != LegacyProtocolVersion || string(request.Data) != "legacy-request" {
		t.Fatalf("legacy request = proto:%d version:%d data:%q", request.ProtoId, request.Version, request.Data)
	}
}

func TestTCPLegacyClientIsAcceptedByNewServer(t *testing.T) {
	serverPolicy := ProtocolVersionPolicy{
		Enabled: true, MinVersion: 1, MaxVersion: 2, Capabilities: 1, AcceptLegacy: true,
	}
	serverPackets := make(chan NetPacket, 1)
	clientPackets := make(chan NetPacket, 1)
	serverSessions := make(chan *TcpServerSession, 1)

	var server *TcpServer
	server = NewTcpServer(&TcpConfig{
		ListenAddress: "127.0.0.1:0", ChanSize: 32, DisableEncryption: true,
		ProtocolVersion: serverPolicy,
	}, WithAddSessionCallBack(func(sid SessionIdType) {
		serverSessions <- server.GetSession(sid)
	}))
	server.RegisterDispatcher(func(_ Session, packet *NetPacket) error {
		serverPackets <- cloneNetPacket(*packet)
		return nil
	})
	startProtocolTestServer(t, server)

	host, port := parseAddr(server.GetListenAddress())
	client := NewTcpClient(&TcpClientConfig{
		ServerAddr: host, ServerPort: port, DisableEncryption: true,
	})
	client.RegisterDispatcher(func(_ Session, packet *NetPacket) error {
		clientPackets <- cloneNetPacket(*packet)
		return nil
	})
	if err := client.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(client.Close)

	// The legacy endpoint receives the positive control ID as an ordinary
	// unknown message and remains connected.
	hello := receiveProtocolTestValue(t, clientPackets)
	if hello.ProtoId != ProtocolNegotiationProtoId || hello.Version != serverPolicy.MaxVersion {
		t.Fatalf("legacy client received hello proto=%d version=%d", hello.ProtoId, hello.Version)
	}
	if err := client.Send(401, []byte("legacy-client")); err != nil {
		t.Fatalf("legacy client Send() error = %v", err)
	}
	packet := receiveProtocolTestValue(t, serverPackets)
	if packet.Version != LegacyProtocolVersion || string(packet.Data) != "legacy-client" {
		t.Fatalf("new server received version=%d data=%q", packet.Version, packet.Data)
	}

	serverSession := receiveProtocolTestValue(t, serverSessions)
	compatibility, ok := serverSession.ProtocolCompatibility()
	if !ok || !compatibility.IsLegacy() || compatibility.Capabilities != 0 {
		t.Fatalf("server compatibility = (%+v, %v), want established legacy", compatibility, ok)
	}
}

func TestTCPProtocolNegotiationRejectsLegacyAndDisjointRanges(t *testing.T) {
	t.Run("legacy timeout", func(t *testing.T) {
		server := NewTcpServer(&TcpConfig{
			ListenAddress: "127.0.0.1:0", ChanSize: 32, DisableEncryption: true,
		})
		server.RegisterDispatcher(func(Session, *NetPacket) error { return nil })
		startProtocolTestServer(t, server)

		host, port := parseAddr(server.GetListenAddress())
		client := NewTcpClient(&TcpClientConfig{
			ServerAddr: host, ServerPort: port, DisableEncryption: true,
			ProtocolVersion:            ProtocolVersionPolicy{Enabled: true, MinVersion: 1, MaxVersion: 2},
			ProtocolNegotiationTimeout: 50 * time.Millisecond,
		})
		client.RegisterDispatcher(func(Session, *NetPacket) error { return nil })
		if err := client.Connect(); !errors.Is(err, ErrProtocolNegotiationTimeout) {
			t.Fatalf("Connect() error = %v, want ErrProtocolNegotiationTimeout", err)
		}
		if client.GetSession() != nil || client.IsConnected() {
			t.Fatal("failed negotiation published a connected session")
		}
		client.Close()
	})

	t.Run("disjoint ranges", func(t *testing.T) {
		server := NewTcpServer(&TcpConfig{
			ListenAddress: "127.0.0.1:0", ChanSize: 32, DisableEncryption: true,
			ProtocolVersion: ProtocolVersionPolicy{Enabled: true, MinVersion: 1, MaxVersion: 2},
		})
		startProtocolTestServer(t, server)

		host, port := parseAddr(server.GetListenAddress())
		client := NewTcpClient(&TcpClientConfig{
			ServerAddr: host, ServerPort: port, DisableEncryption: true,
			ProtocolVersion:            ProtocolVersionPolicy{Enabled: true, MinVersion: 3, MaxVersion: 4},
			ProtocolNegotiationTimeout: time.Second,
		})
		client.RegisterDispatcher(func(Session, *NetPacket) error { return nil })
		if err := client.Connect(); !errors.Is(err, ErrNoCompatibleProtocolVersion) {
			t.Fatalf("Connect() error = %v, want ErrNoCompatibleProtocolVersion", err)
		}
		if client.GetSession() != nil || client.IsConnected() {
			t.Fatal("incompatible negotiation published a connected session")
		}
		client.Close()
	})
}

func TestTCPProtocolPolicyValidationFailsBeforeIO(t *testing.T) {
	invalid := ProtocolVersionPolicy{Enabled: true, MinVersion: 2, MaxVersion: 1}
	server := NewTcpServer(&TcpConfig{ListenAddress: "127.0.0.1:0", ProtocolVersion: invalid})
	if err := server.Start(); !errors.Is(err, ErrInvalidProtocolVersionPolicy) {
		t.Fatalf("server Start() error = %v, want ErrInvalidProtocolVersionPolicy", err)
	}
	server.Close()

	client := NewTcpClient(&TcpClientConfig{ServerAddr: "127.0.0.1", ServerPort: 1, ProtocolVersion: invalid})
	if err := client.Connect(); !errors.Is(err, ErrInvalidProtocolVersionPolicy) {
		t.Fatalf("client Connect() error = %v, want ErrInvalidProtocolVersionPolicy", err)
	}
	client.Close()
}

func startProtocolTestServer(t *testing.T, server *TcpServer) {
	t.Helper()
	startResult := make(chan error, 1)
	go func() {
		startResult <- server.Start()
	}()
	t.Cleanup(server.Close)
	if !waitFor(t, 2*time.Second, func() bool { return server.GetListenAddress() != "" }) {
		select {
		case err := <-startResult:
			t.Fatalf("server Start() exited before listen: %v", err)
		default:
			t.Fatal("server did not report a listen address")
		}
	}
}

func receiveProtocolTestValue[T any](t *testing.T, values <-chan T) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(2 * time.Second):
		var zero T
		t.Fatal("timed out waiting for protocol test value")
		return zero
	}
}

func cloneNetPacket(packet NetPacket) NetPacket {
	packet.Data = append([]byte(nil), packet.Data...)
	return packet
}
