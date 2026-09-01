package zNet

import (
	"bytes"
	"encoding/binary"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/golang/snappy"
	"github.com/pzqf/zUtil/zCrypto"
)

func TestPacketSizeLimitDefaultsAndLegacyCompatibility(t *testing.T) {
	tcpConfig := &TcpConfig{}
	NewTcpServer(tcpConfig)
	assertPacketLimits(t, tcpConfig.MaxWirePacketSize, tcpConfig.MaxDecodedPacketSize,
		DefaultWirePacketSize, DefaultDecodedPacketSize)
	if tcpConfig.MaxPacketDataSize != DefaultWirePacketSize {
		t.Fatalf("legacy TCP wire limit = %d, want %d", tcpConfig.MaxPacketDataSize, DefaultWirePacketSize)
	}

	legacyConfig := &TcpConfig{MaxPacketDataSize: 64}
	NewTcpServer(legacyConfig)
	assertPacketLimits(t, legacyConfig.MaxWirePacketSize, legacyConfig.MaxDecodedPacketSize,
		64, DefaultDecodedPacketSize)

	newConfig := &TcpConfig{MaxPacketDataSize: 64, MaxWirePacketSize: 32, MaxDecodedPacketSize: 128}
	NewTcpServer(newConfig)
	assertPacketLimits(t, newConfig.MaxWirePacketSize, newConfig.MaxDecodedPacketSize, 32, 128)
	if newConfig.MaxPacketDataSize != 32 {
		t.Fatalf("legacy field was not synchronized to the explicit wire limit: %d", newConfig.MaxPacketDataSize)
	}

	optionConfig := &TcpConfig{}
	NewTcpServer(optionConfig, WithMaxPacketDataSize(48), WithMaxDecodedPacketSize(96))
	assertPacketLimits(t, optionConfig.MaxWirePacketSize, optionConfig.MaxDecodedPacketSize, 48, 96)

	udpConfig := &UdpConfig{}
	NewUdpServer(udpConfig)
	assertPacketLimits(t, udpConfig.MaxWirePacketSize, udpConfig.MaxDecodedPacketSize,
		DefaultWirePacketSize, DefaultDecodedPacketSize)

	webSocketConfig := &WebSocketConfig{}
	NewWebSocketServer(webSocketConfig)
	assertPacketLimits(t, webSocketConfig.MaxWirePacketSize, webSocketConfig.MaxDecodedPacketSize,
		DefaultWirePacketSize, DefaultDecodedPacketSize)

	tcpClientConfig := &TcpClientConfig{}
	client := NewTcpClient(tcpClientConfig)
	t.Cleanup(client.Close)
	assertPacketLimits(t, tcpClientConfig.MaxWirePacketSize, tcpClientConfig.MaxDecodedPacketSize,
		DefaultWirePacketSize, DefaultDecodedPacketSize)
}

func assertPacketLimits(t *testing.T, gotWire, gotDecoded, wantWire, wantDecoded int32) {
	t.Helper()
	if gotWire != wantWire || gotDecoded != wantDecoded {
		t.Fatalf("packet limits = wire:%d decoded:%d, want wire:%d decoded:%d",
			gotWire, gotDecoded, wantWire, wantDecoded)
	}
}

func compressedTestPacket(protoID ProtoIdType, decodedSize int) NetPacket {
	compressed := snappy.Encode(nil, bytes.Repeat([]byte("x"), decodedSize))
	return NetPacket{
		ProtoId:      protoID,
		DataSize:     int32(len(compressed)),
		Data:         compressed,
		IsCompressed: CompressionSnappy,
	}
}

func TestDecodePacketPayloadSnappyBoundaries(t *testing.T) {
	payload := bytes.Repeat([]byte("decoded-payload-"), 16)
	compressed := snappy.Encode(nil, payload)

	packet := &NetPacket{
		ProtoId:      1,
		DataSize:     int32(len(compressed)),
		Data:         append([]byte(nil), compressed...),
		IsCompressed: CompressionSnappy,
	}
	if err := decodePacketPayload(packet, nil, false, int32(len(payload))); err != nil {
		t.Fatalf("decode at boundary failed: %v", err)
	}
	if !bytes.Equal(packet.Data, payload) {
		t.Fatalf("decoded payload mismatch: got %d bytes, want %d", len(packet.Data), len(payload))
	}
	if packet.DataSize != int32(len(payload)) {
		t.Fatalf("decoded DataSize = %d, want %d", packet.DataSize, len(payload))
	}

	overLimit := &NetPacket{
		ProtoId:      1,
		DataSize:     int32(len(compressed)),
		Data:         append([]byte(nil), compressed...),
		IsCompressed: CompressionSnappy,
	}
	if err := decodePacketPayload(overLimit, nil, false, int32(len(payload)-1)); !errors.Is(err, ErrPacketDecodedTooLarge) {
		t.Fatalf("decode over limit error = %v, want %v", err, ErrPacketDecodedTooLarge)
	}
}

func TestDecodePacketPayloadRejectsDeclaredSnappyExpansionBeforeAllocation(t *testing.T) {
	var header [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(header[:], 1<<30)
	packet := &NetPacket{
		ProtoId:      1,
		DataSize:     int32(n),
		Data:         header[:n],
		IsCompressed: CompressionSnappy,
	}

	err := decodePacketPayload(packet, nil, false, 1024)
	if !errors.Is(err, ErrPacketDecodedTooLarge) {
		t.Fatalf("declared expansion error = %v, want %v", err, ErrPacketDecodedTooLarge)
	}
}

func TestDecodePacketPayloadRejectsMalformedSnappy(t *testing.T) {
	packet := &NetPacket{
		ProtoId:      1,
		DataSize:     1,
		Data:         []byte{0xff},
		IsCompressed: CompressionSnappy,
	}
	if err := decodePacketPayload(packet, nil, false, 1024); !errors.Is(err, ErrPacketDecompress) {
		t.Fatalf("malformed Snappy error = %v, want %v", err, ErrPacketDecompress)
	}
}

func TestDecodePacketPayloadUncompressedBoundary(t *testing.T) {
	packet := &NetPacket{ProtoId: 1, DataSize: 4, Data: []byte("abcd")}
	if err := decodePacketPayload(packet, nil, false, 4); err != nil {
		t.Fatalf("uncompressed boundary failed: %v", err)
	}

	packet = &NetPacket{ProtoId: 1, DataSize: 4, Data: []byte("abcd")}
	if err := decodePacketPayload(packet, nil, false, 3); !errors.Is(err, ErrPacketDecodedTooLarge) {
		t.Fatalf("uncompressed over limit error = %v, want %v", err, ErrPacketDecodedTooLarge)
	}
}

func TestDecodePacketPayloadAESGCM(t *testing.T) {
	key := []byte("0123456789abcdef")
	payload := []byte("encrypted payload")
	ciphertext, err := zCrypto.AESEncrypt(payload, key, nil, zCrypto.AESModeGCM)
	if err != nil {
		t.Fatal(err)
	}

	packet := &NetPacket{ProtoId: 1, DataSize: int32(len(ciphertext)), Data: append([]byte(nil), ciphertext...)}
	if err := decodePacketPayload(packet, key, true, int32(len(payload))); err != nil {
		t.Fatalf("decrypt at boundary failed: %v", err)
	}
	if !bytes.Equal(packet.Data, payload) || packet.DataSize != int32(len(payload)) {
		t.Fatalf("decrypted packet = size:%d data:%q", packet.DataSize, packet.Data)
	}

	packet = &NetPacket{ProtoId: 1, DataSize: int32(len(ciphertext)), Data: append([]byte(nil), ciphertext...)}
	if err := decodePacketPayload(packet, key, true, int32(len(payload)-1)); !errors.Is(err, ErrPacketDecodedTooLarge) {
		t.Fatalf("decrypt over limit error = %v, want %v", err, ErrPacketDecodedTooLarge)
	}

	wrongKey := []byte("fedcba9876543210")
	packet = &NetPacket{ProtoId: 1, DataSize: int32(len(ciphertext)), Data: append([]byte(nil), ciphertext...)}
	if err := decodePacketPayload(packet, wrongKey, true, int32(len(payload))); !errors.Is(err, ErrPacketDecrypt) {
		t.Fatalf("wrong-key error = %v, want %v", err, ErrPacketDecrypt)
	}

	packet = &NetPacket{ProtoId: 1, DataSize: 3, Data: []byte("bad")}
	if err := decodePacketPayload(packet, key, true, 1024); !errors.Is(err, ErrPacketDecrypt) {
		t.Fatalf("short-ciphertext error = %v, want %v", err, ErrPacketDecrypt)
	}
}

type detailedDecodeRecorder struct {
	mockRecorder
	wireOversize    atomic.Int64
	decodedOversize atomic.Int64
	decrypt         atomic.Int64
	decompress      atomic.Int64
}

func (r *detailedDecodeRecorder) IncWirePacketOversizeErrors()    { r.wireOversize.Add(1) }
func (r *detailedDecodeRecorder) IncDecodedPacketOversizeErrors() { r.decodedOversize.Add(1) }
func (r *detailedDecodeRecorder) IncDecryptErrors()               { r.decrypt.Add(1) }
func (r *detailedDecodeRecorder) IncDecompressErrors()            { r.decompress.Add(1) }

func TestRecordPacketDecodeErrorClassifiesDetailedMetrics(t *testing.T) {
	recorder := &detailedDecodeRecorder{}
	for _, err := range []error{
		ErrPacketTooLarge,
		ErrPacketDecodedTooLarge,
		ErrPacketDecrypt,
		ErrPacketDecompress,
	} {
		recordPacketDecodeError(recorder, err)
	}

	if recorder.decodeErr.Load() != 4 || recorder.wireOversize.Load() != 1 ||
		recorder.decodedOversize.Load() != 1 || recorder.decrypt.Load() != 1 ||
		recorder.decompress.Load() != 1 {
		t.Fatalf("unexpected decode metrics: total=%d wire=%d decoded=%d decrypt=%d decompress=%d",
			recorder.decodeErr.Load(), recorder.wireOversize.Load(), recorder.decodedOversize.Load(),
			recorder.decrypt.Load(), recorder.decompress.Load())
	}
}

func FuzzDecodePacketPayload(f *testing.F) {
	valid := snappy.Encode(nil, []byte("valid payload"))
	f.Add(valid, true)
	f.Add([]byte{0xff}, true)
	f.Add([]byte("plain"), false)

	f.Fuzz(func(t *testing.T, data []byte, compressed bool) {
		packet := &NetPacket{ProtoId: 1, DataSize: int32(len(data)), Data: append([]byte(nil), data...)}
		if compressed {
			packet.IsCompressed = CompressionSnappy
		}
		err := decodePacketPayload(packet, nil, false, 1024)
		if err != nil {
			return
		}
		if len(packet.Data) > 1024 || packet.DataSize != int32(len(packet.Data)) {
			t.Fatalf("accepted payload outside decoded limit: size=%d DataSize=%d", len(packet.Data), packet.DataSize)
		}
	})
}
