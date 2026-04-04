package zNet

import (
	"bytes"
	"encoding/binary"
	"errors"
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
	order := GetByteOrder()
	buf := bytes.NewReader(data)
	if err := binary.Read(buf, order, &p.ProtoId); err != nil {
		return errors.New("NetPacket head field ProtoId error:" + err.Error())
	}
	if err := binary.Read(buf, order, &p.Version); err != nil {
		return errors.New("NetPacket head field Version error:" + err.Error())
	}
	if err := binary.Read(buf, order, &p.DataSize); err != nil {
		return errors.New("NetPacket head field DataSize error:" + err.Error())
	}
	if err := binary.Read(buf, order, &p.IsCompressed); err != nil {
		return errors.New("NetPacket head field IsCompressed error:" + err.Error())
	}
	if err := binary.Read(buf, order, &p.Sequence); err != nil {
		return errors.New("NetPacket head field Sequence error:" + err.Error())
	}
	if err := binary.Read(buf, order, &p.Timestamp); err != nil {
		return errors.New("NetPacket head field Timestamp error:" + err.Error())
	}
	if err := binary.Read(buf, order, &p.KeyID); err != nil {
		return errors.New("NetPacket head field KeyID error:" + err.Error())
	}
	return nil
}

// Marshal 将数据包序列化为字节数组
// 格式: [ProtoId][Version][DataSize][IsCompressed][Sequence][Timestamp][KeyID][Data...]
// 所有字段使用当前设置的字节序编码（默认小端序）
//
// 返回:
//   - []byte: 序列化后的字节数组
func (p *NetPacket) Marshal() []byte {
	order := GetByteOrder()
	sendBuf := new(bytes.Buffer)
	_ = binary.Write(sendBuf, order, p.ProtoId)
	_ = binary.Write(sendBuf, order, p.Version)
	_ = binary.Write(sendBuf, order, p.DataSize)
	_ = binary.Write(sendBuf, order, p.IsCompressed)
	_ = binary.Write(sendBuf, order, p.Sequence)
	_ = binary.Write(sendBuf, order, p.Timestamp)
	_ = binary.Write(sendBuf, order, p.KeyID)
	if p.Data != nil {
		_ = binary.Write(sendBuf, order, p.Data)
	}

	return sendBuf.Bytes()
}
