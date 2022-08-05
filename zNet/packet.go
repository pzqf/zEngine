package zNet

import (
	"bytes"
	"encoding/binary"
	"errors"
)

const DefaultPacketDataSize = 1024 * 1024

var maxPacketDataSize = int32(DefaultPacketDataSize)

const HeartbeatProtoId = int32(0)
const NetPacketHeadSize = 12

type NetPacket struct {
	ProtoId  int32
	DataSize int32
	Version  int32
	Data     []byte
}

func InitPacket(maxDataSize int) {
	if maxDataSize <= 0 {
		maxDataSize = DefaultPacketDataSize
	}
	maxPacketDataSize = int32(maxDataSize)
}

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
	return nil
}

func (p *NetPacket) Marshal() []byte {
	sendBuf := new(bytes.Buffer)
	_ = binary.Write(sendBuf, binary.LittleEndian, p.ProtoId)
	_ = binary.Write(sendBuf, binary.LittleEndian, p.Version)
	_ = binary.Write(sendBuf, binary.LittleEndian, p.DataSize)
	if p.Data != nil {
		_ = binary.Write(sendBuf, binary.LittleEndian, p.Data)
	}

	return sendBuf.Bytes()
}
