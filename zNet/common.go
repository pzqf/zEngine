package zNet

type SessionCallBackFunc func(sid SessionIdType)

type SessionIdType = uint64

const DefaultChanSize = 1024

const DefaultPacketDataSize = int32(1024 * 1024)

type HandlerFun func(session Session, netPacket *NetPacket) error

const DefaultWorkerPoolSize = 1000000
