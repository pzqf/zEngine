package NetServer

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
)

const MaxNetPacketDataSize = 4096 * 2 * 2

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

// if you want use json or other, change the function or add other decode, encode function
// PS: struct size, gob > json,

func (p *NetPacket) DecodeData(data interface{}) error {
	err := json.Unmarshal(p.Data, data)
	if err != nil {
		return err
	}
	return nil
}

func (p *NetPacket) EncodeData(data interface{}) error {
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
	if p.DataSize > MaxNetPacketDataSize {
		return errors.New(fmt.Sprintf("Data size over max size, data size :%d, max size: %d", p.DataSize, MaxNetPacketDataSize))
	}

	return nil
}
