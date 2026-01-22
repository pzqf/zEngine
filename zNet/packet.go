package zNet

import (
	"bytes"
	"encoding/binary"
	"errors"
)

//const DefaultPacketDataSize = 1024 * 1024

//var maxPacketDataSize = int32(DefaultPacketDataSize)

const HeartbeatProtoId = int32(0)
const NetPacketHeadSize = 16

type NetPacket struct {
	ProtoId      int32
	DataSize     int32
	Version      int32
	IsCompressed bool
	Data         []byte
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
	if err := binary.Read(buf, binary.LittleEndian, &p.IsCompressed); err != nil {
		return errors.New("NetPacket head field IsCompressed error:" + err.Error())
	}
	return nil
}

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
