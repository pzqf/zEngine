package zNet

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"encoding/json"
)

const MaxNetPacketDataSize = 4096 * 2 * 2

type PacketCodeType int

const (
	PacketCodeByte = PacketCodeType(1)
	PacketCodeJson = PacketCodeType(2)
	PacketCodeGob  = PacketCodeType(3)
)

var packetCode = PacketCodeJson
var PacketDataSize = MaxNetPacketDataSize

func InitPacket(packetCodeType PacketCodeType, maxDataSize int) {
	if packetCodeType < PacketCodeByte && packetCodeType > PacketCodeGob {
		packetCodeType = PacketCodeJson
	}
	packetCode = packetCodeType

	if maxDataSize <= 0 {
		PacketDataSize = MaxNetPacketDataSize
	}
}

type NetPacket struct {
	ProtoId  int32  `json:"proto_id"`
	DataSize int32  `json:"data_size"`
	Data     []byte `json:"data"`
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

// if you want custom code type, please use PacketCodeByte, data type must []byte

func (p *NetPacket) EncodeData(data interface{}) error {
	if data == nil {
		p.DataSize = 0
		return nil
	}
	switch packetCode {
	case PacketCodeByte:
		err := p.ByteEncodeData(data.([]byte))
		if err != nil {
			return err
		}
	case PacketCodeJson:
		err := p.JsonEncodeData(data)
		if err != nil {
			return err
		}
	case PacketCodeGob:
		err := p.GobEncodeData(data)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *NetPacket) DecodeData(data interface{}) error {
	if p.DataSize == 0 {
		return nil
	}
	switch packetCode {
	case PacketCodeByte:
		err := p.ByteDecodeData(data.([]byte))
		if err != nil {
			return err
		}
	case PacketCodeJson:
		err := p.JsonDecodeData(data)
		if err != nil {
			return err
		}
	case PacketCodeGob:
		err := p.GobDecodeData(data)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *NetPacket) ByteDecodeData(data []byte) error {
	data = p.Data
	return nil
}

func (p *NetPacket) ByteEncodeData(data []byte) error {
	p.Data = append([]byte(nil), data...)
	p.DataSize = int32(len(p.Data))
	return nil
}

func (p *NetPacket) JsonDecodeData(data interface{}) error {
	err := json.Unmarshal(p.Data, data)
	if err != nil {
		return err
	}
	return nil
}

func (p *NetPacket) JsonEncodeData(data interface{}) error {
	marshal, err := json.Marshal(data)
	if err != nil {
		return err
	}
	p.Data = append([]byte(nil), marshal...)
	p.DataSize = int32(len(p.Data))
	return nil
}

func (p *NetPacket) GobDecodeData(data interface{}) error {
	buf := bytes.NewReader(p.Data)
	dec := gob.NewDecoder(buf)
	err := dec.Decode(data)
	if err != nil {
		return err
	}

	return nil
}

func (p *NetPacket) GobEncodeData(data interface{}) error {
	var buf bytes.Buffer
	err := gob.NewEncoder(&buf).Encode(data)
	if err != nil {
		return err
	}
	p.Data = append([]byte(nil), buf.Bytes()...)
	p.DataSize = int32(len(p.Data))
	return nil
}
