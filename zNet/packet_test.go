package zNet

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/golang/snappy"
)

func TestNetPacketMarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		packet  NetPacket
		wantErr bool
	}{
		{
			name: "正常数据包",
			packet: NetPacket{
				ProtoId:      100,
				DataSize:     16,
				Version:      1,
				IsCompressed: CompressionNone,
				Sequence:     12345,
				Timestamp:    1234567890,
				KeyID:        1,
				Data:         []byte("test data packet"),
			},
			wantErr: false,
		},
		{
			name: "压缩数据包",
			packet: NetPacket{
				ProtoId:      200,
				DataSize:     20,
				Version:      2,
				IsCompressed: CompressionSnappy,
				Sequence:     54321,
				Timestamp:    9876543210,
				KeyID:        2,
				Data:         []byte("compressed test data"),
			},
			wantErr: false,
		},
		{
			name: "空数据包",
			packet: NetPacket{
				ProtoId:      HeartbeatProtoId,
				DataSize:     0,
				Version:      1,
				IsCompressed: CompressionNone,
				Sequence:     1,
				Timestamp:    1234567890,
				KeyID:        0,
				Data:         nil,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := tt.packet.Marshal()
			if len(data) != int(NetPacketHeadSize)+int(tt.packet.DataSize) {
				t.Errorf("Marshal() length = %d, want %d", len(data), NetPacketHeadSize+tt.packet.DataSize)
			}

			parsed := NetPacket{}
			err := parsed.UnmarshalHead(data[:NetPacketHeadSize])
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalHead() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if parsed.ProtoId != tt.packet.ProtoId {
					t.Errorf("ProtoId = %v, want %v", parsed.ProtoId, tt.packet.ProtoId)
				}
				if parsed.DataSize != tt.packet.DataSize {
					t.Errorf("DataSize = %v, want %v", parsed.DataSize, tt.packet.DataSize)
				}
				if parsed.Version != tt.packet.Version {
					t.Errorf("Version = %v, want %v", parsed.Version, tt.packet.Version)
				}
				if parsed.IsCompressed != tt.packet.IsCompressed {
					t.Errorf("IsCompressed = %v, want %v", parsed.IsCompressed, tt.packet.IsCompressed)
				}
				if parsed.Sequence != tt.packet.Sequence {
					t.Errorf("Sequence = %v, want %v", parsed.Sequence, tt.packet.Sequence)
				}
				if parsed.Timestamp != tt.packet.Timestamp {
					t.Errorf("Timestamp = %v, want %v", parsed.Timestamp, tt.packet.Timestamp)
				}
				if parsed.KeyID != tt.packet.KeyID {
					t.Errorf("KeyID = %v, want %v", parsed.KeyID, tt.packet.KeyID)
				}

				if tt.packet.DataSize > 0 {
					parsed.Data = data[NetPacketHeadSize:]
					if !bytes.Equal(parsed.Data, tt.packet.Data) {
						t.Errorf("Data = %v, want %v", parsed.Data, tt.packet.Data)
					}
				}
			}
		})
	}
}

func TestNetPacketByteOrder(t *testing.T) {
	packet := NetPacket{
		ProtoId:      100,
		DataSize:     16,
		Version:      1,
		IsCompressed: CompressionNone,
		Sequence:     12345,
		Timestamp:    1234567890,
		KeyID:        1,
		Data:         []byte("test data"),
	}

	t.Run("LittleEndian", func(t *testing.T) {
		SetByteOrder(binary.LittleEndian)
		data := packet.Marshal()

		parsed := NetPacket{}
		parsed.UnmarshalHead(data[:NetPacketHeadSize])
		parsed.Data = data[NetPacketHeadSize:]

		if parsed.ProtoId != packet.ProtoId {
			t.Errorf("LittleEndian ProtoId mismatch")
		}
	})

	t.Run("BigEndian", func(t *testing.T) {
		SetByteOrder(binary.BigEndian)
		data := packet.Marshal()

		parsed := NetPacket{}
		parsed.UnmarshalHead(data[:NetPacketHeadSize])
		parsed.Data = data[NetPacketHeadSize:]

		if parsed.ProtoId != packet.ProtoId {
			t.Errorf("BigEndian ProtoId mismatch")
		}
	})

	SetByteOrder(binary.LittleEndian)
}

func BenchmarkNetPacketMarshal(b *testing.B) {
	packet := NetPacket{
		ProtoId:      100,
		DataSize:     1024,
		Version:      1,
		IsCompressed: CompressionNone,
		Sequence:     12345,
		Timestamp:    1234567890,
		KeyID:        1,
		Data:         make([]byte, 1024),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = packet.Marshal()
	}
}

func BenchmarkNetPacketUnmarshalHead(b *testing.B) {
	packet := NetPacket{
		ProtoId:      100,
		DataSize:     1024,
		Version:      1,
		IsCompressed: CompressionNone,
		Sequence:     12345,
		Timestamp:    1234567890,
		KeyID:        1,
		Data:         make([]byte, 1024),
	}

	data := packet.Marshal()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parsed := NetPacket{}
		_ = parsed.UnmarshalHead(data[:NetPacketHeadSize])
	}
}

func BenchmarkNetPacketFullCycle(b *testing.B) {
	packet := NetPacket{
		ProtoId:      100,
		DataSize:     1024,
		Version:      1,
		IsCompressed: CompressionNone,
		Sequence:     12345,
		Timestamp:    1234567890,
		KeyID:        1,
		Data:         make([]byte, 1024),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data := packet.Marshal()
		parsed := NetPacket{}
		_ = parsed.UnmarshalHead(data[:NetPacketHeadSize])
		parsed.Data = data[NetPacketHeadSize:]
	}
}

func BenchmarkNetPacketWithCompression(b *testing.B) {
	packet := NetPacket{
		ProtoId:      100,
		DataSize:     1024,
		Version:      1,
		IsCompressed: CompressionSnappy,
		Sequence:     12345,
		Timestamp:    1234567890,
		KeyID:        1,
		Data:         make([]byte, 1024),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data := packet.Marshal()
		parsed := NetPacket{}
		_ = parsed.UnmarshalHead(data[:NetPacketHeadSize])
		parsed.Data = data[NetPacketHeadSize:]
		if parsed.IsCompressed == CompressionSnappy {
			parsed.Data, _ = snappy.Decode(nil, parsed.Data)
		}
	}
}
