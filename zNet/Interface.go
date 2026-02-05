package zNet

import "github.com/pzqf/zEngine/zLog"

// Logger 日志接口
// 类型别名，使用zLog的Logger接口
type Logger zLog.Logger

// Session 会话接口
// 定义网络会话的基本操作方法
type Session interface {
	Start()              // 启动Session
	Close()              // 关闭Session
	Send(protoId int32, data []byte) error // 发送数据
	GetSid() SessionIdType // 获取Session ID
}

// Server 服务器接口
// 定义网络服务器的基本操作方法
type Server interface {
	Start() error // 启动服务器
	Close()       // 关闭服务器
}
