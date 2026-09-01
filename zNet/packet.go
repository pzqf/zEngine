package zNet

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync/atomic"
	"time"
)

// ProtoIdType 协议ID类型
type ProtoIdType int32

const (
	// HeartbeatProtoId 心跳协议ID，用于连接保活
	HeartbeatProtoId ProtoIdType = -1
	// KeyRotationNotifyProtoId 密钥轮换通知协议ID
	KeyRotationNotifyProtoId ProtoIdType = -2
)

// 压缩状态常量
const (
	CompressionNone   int32 = 0 // 未压缩
	CompressionSnappy int32 = 1 // Snappy压缩
)

// NetPacketHeadSize 网络包头部大小（字节）
// 包含: ProtoId(4) + Version(4) + DataSize(4) + IsCompressed(4) + Sequence(8) + Timestamp(8) + KeyID(4) = 36字节
const NetPacketHeadSize = 36

var (
	ErrPacketHeaderSize  = errors.New("invalid packet header size")
	ErrPacketProtoID     = errors.New("invalid packet proto id")
	ErrPacketDataSize    = errors.New("invalid packet data size")
	ErrPacketTooLarge    = errors.New("packet data exceeds limit")
	ErrPacketCompression = errors.New("invalid packet compression flag")
	ErrPacketFrameSize   = errors.New("packet frame size mismatch")
)

const invalidPacketLogInterval = 5 * time.Second

type packetErrorLogLimiter struct {
	nextLogUnixNano atomic.Int64
}

func (l *packetErrorLogLimiter) Allow(now time.Time) bool {
	nowUnixNano := now.UnixNano()
	for {
		next := l.nextLogUnixNano.Load()
		if nowUnixNano < next {
			return false
		}
		if l.nextLogUnixNano.CompareAndSwap(next, now.Add(invalidPacketLogInterval).UnixNano()) {
			return true
		}
	}
}

// PacketCodec 固定一个端点的线格式字节序。零值按小端处理；生产端点在构造时显式快照配置。
type PacketCodec struct {
	order binary.ByteOrder
}

func NewPacketCodec(order binary.ByteOrder) PacketCodec {
	if order == nil {
		order = binary.LittleEndian
	}
	return PacketCodec{order: order}
}

func newEndpointPacketCodec(order WireByteOrder) (PacketCodec, error) {
	resolved, err := resolveWireByteOrder(order)
	if err != nil {
		return PacketCodec{}, err
	}
	return NewPacketCodec(resolved), nil
}

func (c PacketCodec) ByteOrder() binary.ByteOrder {
	if c.order == nil {
		return binary.LittleEndian
	}
	return c.order
}

// NetPacket 网络数据包结构
// 用于在网络层传输协议数据，包含协议ID、版本号、数据大小、压缩标志、序列号、时间戳、密钥ID和实际数据
type NetPacket struct {
	Sequence     uint64      // 消息序列号，用于防重放攻击
	Timestamp    int64       // 时间戳（Unix秒），用于防重放攻击
	ProtoId      ProtoIdType // 协议ID，用于标识消息类型
	DataSize     int32       // 数据体大小（字节）
	Version      int32       // 协议版本号，用于版本兼容
	IsCompressed int32       // 数据是否压缩 (0=未压缩, 1=压缩)
	KeyID        uint32      // 密钥ID，用于密钥轮换
	Data         []byte      // 实际数据内容（序列化后的协议数据）
}

// UnmarshalHead 从字节数组解析数据包头部
// 参数:
//   - data: 头部数据字节数组，长度必须 >= NetPacketHeadSize
//
// 返回:
//   - error: 解析失败时返回错误信息
func (p *NetPacket) UnmarshalHead(data []byte) error {
	return NewPacketCodec(GetByteOrder()).UnmarshalHead(p, data)
}

// UnmarshalHead 按 codec 的端点字节序解析固定长度包头，不执行语义校验。
func (c PacketCodec) UnmarshalHead(p *NetPacket, data []byte) error {
	if len(data) < NetPacketHeadSize {
		return fmt.Errorf("%w: got %d, want at least %d", ErrPacketHeaderSize, len(data), NetPacketHeadSize)
	}
	order := c.ByteOrder()
	p.ProtoId = ProtoIdType(int32(order.Uint32(data[0:4])))
	p.Version = int32(order.Uint32(data[4:8]))
	p.DataSize = int32(order.Uint32(data[8:12]))
	p.IsCompressed = int32(order.Uint32(data[12:16]))
	p.Sequence = order.Uint64(data[16:24])
	p.Timestamp = int64(order.Uint64(data[24:32]))
	p.KeyID = order.Uint32(data[32:36])
	return nil
}

// Marshal 将数据包序列化为字节数组
// 格式: [ProtoId][Version][DataSize][IsCompressed][Sequence][Timestamp][KeyID][Data...]
// 所有字段使用当前设置的字节序编码（默认小端序）
//
// 返回:
//   - []byte: 序列化后的字节数组
func (p *NetPacket) Marshal() []byte {
	return NewPacketCodec(GetByteOrder()).Marshal(p)
}

// Marshal 按 codec 的端点字节序编码包头和数据。
func (c PacketCodec) Marshal(p *NetPacket) []byte {
	data := make([]byte, NetPacketHeadSize+len(p.Data))
	order := c.ByteOrder()
	order.PutUint32(data[0:4], uint32(p.ProtoId))
	order.PutUint32(data[4:8], uint32(p.Version))
	order.PutUint32(data[8:12], uint32(p.DataSize))
	order.PutUint32(data[12:16], uint32(p.IsCompressed))
	order.PutUint64(data[16:24], p.Sequence)
	order.PutUint64(data[24:32], uint64(p.Timestamp))
	order.PutUint32(data[32:36], p.KeyID)
	copy(data[NetPacketHeadSize:], p.Data)
	return data
}

func IsValidProtoID(protoID ProtoIdType) bool {
	return protoID > 0 || protoID == HeartbeatProtoId || protoID == KeyRotationNotifyProtoId
}

// ValidatePacketHeader 校验会影响分配、长度运算和解码分支的所有头字段。
func ValidatePacketHeader(packet *NetPacket, maxPacketDataSize int32) error {
	if !IsValidProtoID(packet.ProtoId) {
		return fmt.Errorf("%w: %d", ErrPacketProtoID, packet.ProtoId)
	}
	if packet.DataSize < 0 {
		return fmt.Errorf("%w: %d", ErrPacketDataSize, packet.DataSize)
	}
	if maxPacketDataSize > 0 && packet.DataSize > maxPacketDataSize {
		return fmt.Errorf("%w: got %d, max %d", ErrPacketTooLarge, packet.DataSize, maxPacketDataSize)
	}
	if packet.IsCompressed != CompressionNone && packet.IsCompressed != CompressionSnappy {
		return fmt.Errorf("%w: %d", ErrPacketCompression, packet.IsCompressed)
	}
	return nil
}

// DecodeHeader 解析并校验一个 TCP 头。调用方只有在成功后才可按 DataSize 分配 body。
func (c PacketCodec) DecodeHeader(data []byte, maxPacketDataSize int32) (NetPacket, error) {
	if len(data) != NetPacketHeadSize {
		return NetPacket{}, fmt.Errorf("%w: got %d, want %d", ErrPacketHeaderSize, len(data), NetPacketHeadSize)
	}
	var packet NetPacket
	if err := c.UnmarshalHead(&packet, data); err != nil {
		return NetPacket{}, err
	}
	if err := ValidatePacketHeader(&packet, maxPacketDataSize); err != nil {
		return NetPacket{}, err
	}
	return packet, nil
}

// DecodeFrame 解析 UDP/WebSocket 的完整消息。Data 直接引用输入切片，不额外分配。
func (c PacketCodec) DecodeFrame(data []byte, maxPacketDataSize int32) (NetPacket, error) {
	if len(data) < NetPacketHeadSize {
		return NetPacket{}, fmt.Errorf("%w: got %d, want at least %d", ErrPacketHeaderSize, len(data), NetPacketHeadSize)
	}
	packet, err := c.DecodeHeader(data[:NetPacketHeadSize], maxPacketDataSize)
	if err != nil {
		return NetPacket{}, err
	}
	expected := int64(NetPacketHeadSize) + int64(packet.DataSize)
	if int64(len(data)) != expected {
		return NetPacket{}, fmt.Errorf("%w: got %d, want %d", ErrPacketFrameSize, len(data), expected)
	}
	packet.Data = data[NetPacketHeadSize:]
	return packet, nil
}
