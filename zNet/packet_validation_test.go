package zNet

import (
	"encoding/binary"
	"errors"
	"math"
	"testing"
	"time"
)

func TestPacketCodecDecodeHeader(t *testing.T) {
	codec := NewPacketCodec(binary.LittleEndian)
	tests := []struct {
		name    string
		packet  NetPacket
		maxSize int32
		wantErr error
	}{
		{name: "business", packet: NetPacket{ProtoId: 100, DataSize: 8}, maxSize: 8},
		{name: "heartbeat", packet: NetPacket{ProtoId: HeartbeatProtoId}, maxSize: 8},
		{name: "key rotation", packet: NetPacket{ProtoId: KeyRotationNotifyProtoId}, maxSize: 8},
		{name: "zero proto", packet: NetPacket{}, maxSize: 8, wantErr: ErrPacketProtoID},
		{name: "unknown control", packet: NetPacket{ProtoId: -3}, maxSize: 8, wantErr: ErrPacketProtoID},
		{name: "negative data size", packet: NetPacket{ProtoId: 1, DataSize: -1}, maxSize: 8, wantErr: ErrPacketDataSize},
		{name: "over limit", packet: NetPacket{ProtoId: 1, DataSize: 9}, maxSize: 8, wantErr: ErrPacketTooLarge},
		{name: "invalid compression", packet: NetPacket{ProtoId: 1, IsCompressed: 2}, maxSize: 8, wantErr: ErrPacketCompression},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			head := codec.Marshal(&tt.packet)[:NetPacketHeadSize]
			got, err := codec.DecodeHeader(head, tt.maxSize)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("DecodeHeader() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("DecodeHeader() error = %v", err)
			}
			if got.ProtoId != tt.packet.ProtoId || got.DataSize != tt.packet.DataSize {
				t.Fatalf("DecodeHeader() = %+v, want proto=%d size=%d", got, tt.packet.ProtoId, tt.packet.DataSize)
			}
		})
	}
}

func TestLegacyUnmarshalHeadAcceptsLargerSlice(t *testing.T) {
	codec := NewPacketCodec(binary.LittleEndian)
	packet := NetPacket{ProtoId: 100, DataSize: 3, Data: []byte("abc")}
	frame := codec.Marshal(&packet)

	var decoded NetPacket
	if err := decoded.UnmarshalHead(frame); err != nil {
		t.Fatalf("UnmarshalHead() broke legacy larger-slice contract: %v", err)
	}
	if decoded.ProtoId != packet.ProtoId || decoded.DataSize != packet.DataSize {
		t.Fatalf("UnmarshalHead() = %+v, want proto=%d size=%d", decoded, packet.ProtoId, packet.DataSize)
	}
	if _, err := codec.DecodeHeader(frame, 3); !errors.Is(err, ErrPacketHeaderSize) {
		t.Fatalf("DecodeHeader(frame) error = %v, want %v", err, ErrPacketHeaderSize)
	}
}

func TestPacketCodecDecodeFrameRequiresExactLength(t *testing.T) {
	codec := NewPacketCodec(binary.LittleEndian)
	packet := NetPacket{ProtoId: 100, DataSize: 3, Data: []byte("abc")}
	frame := codec.Marshal(&packet)

	got, err := codec.DecodeFrame(frame, 3)
	if err != nil {
		t.Fatalf("DecodeFrame() error = %v", err)
	}
	if string(got.Data) != "abc" {
		t.Fatalf("DecodeFrame() data = %q, want abc", got.Data)
	}

	for _, tt := range []struct {
		name  string
		frame []byte
	}{
		{name: "short header", frame: frame[:NetPacketHeadSize-1]},
		{name: "short body", frame: frame[:len(frame)-1]},
		{name: "trailing body", frame: append(append([]byte(nil), frame...), 0)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := codec.DecodeFrame(tt.frame, 3)
			if err == nil {
				t.Fatal("DecodeFrame() error = nil")
			}
		})
	}
}

func TestPacketCodecRejectsOversizeBeforeFrameLengthArithmetic(t *testing.T) {
	codec := NewPacketCodec(binary.LittleEndian)
	packet := NetPacket{ProtoId: 1, DataSize: math.MaxInt32}
	headOnly := codec.Marshal(&packet)

	_, err := codec.DecodeFrame(headOnly, 1024)
	if !errors.Is(err, ErrPacketTooLarge) {
		t.Fatalf("DecodeFrame() error = %v, want %v", err, ErrPacketTooLarge)
	}
}

func TestPacketCodecByteOrderIsEndpointScoped(t *testing.T) {
	previous := GetByteOrder()
	t.Cleanup(func() { SetByteOrder(previous) })

	SetByteOrder(binary.LittleEndian)
	little, err := newEndpointPacketCodec(WireByteOrderDefault)
	if err != nil {
		t.Fatal(err)
	}
	big, err := newEndpointPacketCodec(WireByteOrderBig)
	if err != nil {
		t.Fatal(err)
	}
	SetByteOrder(binary.BigEndian)

	packet := NetPacket{ProtoId: 0x01020304, DataSize: 3, Data: []byte("abc")}
	littleFrame := little.Marshal(&packet)
	bigFrame := big.Marshal(&packet)
	if string(littleFrame[:4]) == string(bigFrame[:4]) {
		t.Fatalf("endpoint codecs produced the same proto bytes: %v", littleFrame[:4])
	}
	if _, err := little.DecodeFrame(littleFrame, 3); err != nil {
		t.Fatalf("little endpoint changed after SetByteOrder: %v", err)
	}
	if _, err := big.DecodeFrame(bigFrame, 3); err != nil {
		t.Fatalf("big endpoint decode failed: %v", err)
	}
	if _, err := little.DecodeFrame(bigFrame, 3); err == nil {
		t.Fatal("little endpoint accepted a big-endian frame")
	}
}

func TestNewEndpointPacketCodecRejectsUnknownOrder(t *testing.T) {
	_, err := newEndpointPacketCodec(WireByteOrder("middle"))
	if err == nil {
		t.Fatal("newEndpointPacketCodec() error = nil")
	}
}

func TestEndpointsRejectUnknownWireByteOrderBeforeIO(t *testing.T) {
	invalid := WireByteOrder("middle")
	if err := NewTcpServer(&TcpConfig{ByteOrder: invalid}).Start(); err == nil {
		t.Fatal("TcpServer.Start() accepted an unknown byte order")
	}
	if err := NewUdpServer(&UdpConfig{ByteOrder: invalid}).Start(); err == nil {
		t.Fatal("UdpServer.Start() accepted an unknown byte order")
	}
	if err := NewWebSocketServer(&WebSocketConfig{ByteOrder: invalid}).Start(); err == nil {
		t.Fatal("WebSocketServer.Start() accepted an unknown byte order")
	}
	if err := NewTcpClient(&TcpClientConfig{ByteOrder: invalid}).Connect(); err == nil {
		t.Fatal("TcpClient.Connect() accepted an unknown byte order")
	}
	if err := (&UdpClient{}).SetWireByteOrder(invalid); err == nil {
		t.Fatal("UdpClient.SetWireByteOrder() accepted an unknown byte order")
	}
	if err := (&WebSocketClient{}).SetWireByteOrder(invalid); err == nil {
		t.Fatal("WebSocketClient.SetWireByteOrder() accepted an unknown byte order")
	}
}

func TestServerPacketSizeDefaultsAreBounded(t *testing.T) {
	tcpConfig := &TcpConfig{}
	NewTcpServer(tcpConfig)
	if tcpConfig.MaxPacketDataSize != DefaultPacketDataSize {
		t.Fatalf("TCP default max = %d, want %d", tcpConfig.MaxPacketDataSize, DefaultPacketDataSize)
	}
	udpConfig := &UdpConfig{}
	NewUdpServer(udpConfig)
	if udpConfig.MaxPacketDataSize != DefaultPacketDataSize {
		t.Fatalf("UDP default max = %d, want %d", udpConfig.MaxPacketDataSize, DefaultPacketDataSize)
	}
	webSocketConfig := &WebSocketConfig{}
	NewWebSocketServer(webSocketConfig)
	if webSocketConfig.MaxPacketDataSize != DefaultPacketDataSize {
		t.Fatalf("WebSocket default max = %d, want %d", webSocketConfig.MaxPacketDataSize, DefaultPacketDataSize)
	}
}

func TestPacketErrorLogLimiter(t *testing.T) {
	var limiter packetErrorLogLimiter
	now := time.Unix(100, 0)
	if !limiter.Allow(now) {
		t.Fatal("first log was rejected")
	}
	if limiter.Allow(now.Add(invalidPacketLogInterval - time.Nanosecond)) {
		t.Fatal("log inside interval was accepted")
	}
	if !limiter.Allow(now.Add(invalidPacketLogInterval)) {
		t.Fatal("log at interval boundary was rejected")
	}
}

func FuzzPacketCodecDecodeFrame(f *testing.F) {
	codec := NewPacketCodec(binary.LittleEndian)
	valid := codec.Marshal(&NetPacket{ProtoId: 1, DataSize: 3, Data: []byte("abc")})
	f.Add(valid)
	f.Add([]byte{})
	f.Add(make([]byte, NetPacketHeadSize))

	f.Fuzz(func(t *testing.T, data []byte) {
		packet, err := codec.DecodeFrame(data, 1024)
		if err != nil {
			return
		}
		if !IsValidProtoID(packet.ProtoId) {
			t.Fatalf("accepted invalid proto id %d", packet.ProtoId)
		}
		if packet.DataSize < 0 || packet.DataSize > 1024 {
			t.Fatalf("accepted invalid data size %d", packet.DataSize)
		}
		if len(packet.Data) != int(packet.DataSize) {
			t.Fatalf("data length %d != declared %d", len(packet.Data), packet.DataSize)
		}
	})
}
