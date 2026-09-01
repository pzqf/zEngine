package zNet

import (
	"encoding/binary"
	"fmt"
	"sync/atomic"
)

// WireByteOrder 是端点使用的网络包字节序。空值保持兼容：端点构造时快照当前全局默认值。
type WireByteOrder string

const (
	WireByteOrderDefault WireByteOrder = ""
	WireByteOrderLittle  WireByteOrder = "little"
	WireByteOrderBig     WireByteOrder = "big"
)

func resolveWireByteOrder(order WireByteOrder) (binary.ByteOrder, error) {
	switch order {
	case WireByteOrderDefault:
		return GetByteOrder(), nil
	case WireByteOrderLittle:
		return binary.LittleEndian, nil
	case WireByteOrderBig:
		return binary.BigEndian, nil
	default:
		return nil, fmt.Errorf("unsupported wire byte order %q", order)
	}
}

type byteOrderWrapper struct {
	order binary.ByteOrder
}

var (
	byteOrder atomic.Value // 存储字节序，默认为小端序
)

func init() {
	byteOrder.Store(&byteOrderWrapper{order: binary.LittleEndian})
}

// SetByteOrder 设置旧 API 和后续新建端点使用的默认字节序。
// 已构造端点会保留自己的字节序快照，不受后续调用影响。
// 参数:
//   - order: 字节序，binary.LittleEndian 或 binary.BigEndian
func SetByteOrder(order binary.ByteOrder) {
	if order == nil {
		order = binary.LittleEndian
	}
	byteOrder.Store(&byteOrderWrapper{order: order})
}

// GetByteOrder 获取当前网络数据包的字节序
// 返回:
//   - binary.ByteOrder: 当前字节序
func GetByteOrder() binary.ByteOrder {
	return byteOrder.Load().(*byteOrderWrapper).order
}

// SessionCallBackFunc Session回调函数类型
// 用于Session添加/移除时的通知回调
//
// 参数:
//   - sid: Session唯一标识
type SessionCallBackFunc func(sid SessionIdType)

// SessionIdType Session标识类型
// 使用uint64作为Session的唯一标识符
type SessionIdType = uint64

// DefaultChanSize 默认通道大小
// 用于Session的收发通道缓冲区
const DefaultChanSize = 1024

const (
	// DefaultWirePacketSize 限制单个线包 payload 的默认最大数据量为 1 MiB。
	DefaultWirePacketSize = int32(1024 * 1024)
	// DefaultDecodedPacketSize 独立限制解密和解压后的 payload，零值配置采用此默认值。
	DefaultDecodedPacketSize = int32(1024 * 1024)
	// DefaultPacketDataSize 保留旧名称兼容；新代码应明确使用 DefaultWirePacketSize。
	DefaultPacketDataSize = DefaultWirePacketSize
)

// HandlerFun 消息处理函数类型
// 定义处理网络数据包的函数签名
//
// 参数:
//   - session: 发送数据包的Session
//   - netPacket: 接收到的网络数据包
//
// 返回:
//   - error: 处理失败时返回错误
type HandlerFun func(session Session, netPacket *NetPacket) error
