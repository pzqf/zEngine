package zNet

import (
	"bytes"
	"encoding/binary"
)

const DefaultPacketDataSize = int32(1024 * 1024)

var maxPacketDataSize = DefaultPacketDataSize

type NetPacket struct {
	ProtoId  int32  `json:"proto_id"`
	DataSize int32  `json:"data_size"`
	Data     []byte `json:"data"`
}

func InitPacket(maxDataSize int32) {
	if maxDataSize <= 0 {
		maxDataSize = DefaultPacketDataSize
	}
	maxPacketDataSize = maxDataSize
}

func (p *NetPacket) UnmarshalHead(data []byte) error {
	buf := bytes.NewReader(data)
	if err := binary.Read(buf, binary.LittleEndian, &p.ProtoId); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.LittleEndian, &p.DataSize); err != nil {
		return err
	}
	return nil
}
