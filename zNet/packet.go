package zNet

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// HeartbeatProtoId 心跳协议ID，用于连接保活
const HeartbeatProtoId = int32(0)

// 压缩状态常量
const (
	CompressionNone   int32 = 0 // 未压缩
	CompressionSnappy int32 = 1 // Snappy压缩
)

// NetPacketHeadSize 网络包头部大小（字节）
// 包含: ProtoId(4) + Version(4) + DataSize(4) + IsCompressed(4) = 16字节
const NetPacketHeadSize = 16

// NetPacket 网络数据包结构
// 用于在网络层传输协议数据，包含协议ID、版本号、数据大小、压缩标志和实际数据
type NetPacket struct {
	ProtoId      int32  // 协议ID，用于标识消息类型
	DataSize     int32  // 数据体大小（字节）
	Version      int32  // 协议版本号，用于版本兼容
	IsCompressed int32  // 数据是否压缩 (0=未压缩, 1=压缩)
	Data         []byte // 实际数据内容（序列化后的协议数据）
}

// UnmarshalHead 从字节数组解析数据包头部
// 参数:
//   - data: 头部数据字节数组，长度必须 >= NetPacketHeadSize
//
// 返回:
//   - error: 解析失败时返回错误信息
func (p *NetPacket) UnmarshalHead(data []byte) error {
	buf := bytes.NewReader(data)
	if err := binary.Read(buf, binary.LittleEndian, &p.ProtoId); err != nil {
		return errors.New("NetPacket head field ProtoId error:" + err.Error())
	}
	if err := binary.Read(buf, binary.LittleEndian, &p.Version); err != nil {
		return errors.New("NetPacket head field Version error:" + err.Error())
	}
	if err := binary.Read(buf, binary.LittleEndian, &p.DataSize); err != nil {
		return errors.New("NetPacket head field DataSize error:" + err.Error())
	}
	if err := binary.Read(buf, binary.LittleEndian, &p.IsCompressed); err != nil {
		return errors.New("NetPacket head field IsCompressed error:" + err.Error())
	}
	return nil
}

// Marshal 将数据包序列化为字节数组
// 格式: [ProtoId][Version][DataSize][IsCompressed][Data...]
// 所有字段使用小端序编码
//
// 返回:
//   - []byte: 序列化后的字节数组
func (p *NetPacket) Marshal() []byte {
	sendBuf := new(bytes.Buffer)
	_ = binary.Write(sendBuf, binary.LittleEndian, p.ProtoId)
	_ = binary.Write(sendBuf, binary.LittleEndian, p.Version)
	_ = binary.Write(sendBuf, binary.LittleEndian, p.DataSize)
	_ = binary.Write(sendBuf, binary.LittleEndian, p.IsCompressed)
	if p.Data != nil {
		_ = binary.Write(sendBuf, binary.LittleEndian, p.Data)
	}

	return sendBuf.Bytes()
}
