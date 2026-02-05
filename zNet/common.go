package zNet

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

// DefaultPacketDataSize 默认数据包数据大小
// 限制单个数据包的最大数据量为1MB
const DefaultPacketDataSize = int32(1024 * 1024)

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
