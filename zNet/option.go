package zNet

func WithMaxClientCount(maxClientCount int32) Options {
	return func(svr *TcpServer) {
		svr.maxClientCount = maxClientCount
	}
}
func WithSidInitio(sidInitio int64) Options {
	return func(svr *TcpServer) {
		svr.clientSIDAtomic = sidInitio
	}
}

func WithPacketCodeType(codeType PacketCodeType) Options {
	return func(svr *TcpServer) {
		packetCode = codeType
	}
}

func WithMaxPacketDataSize(size int32) Options {
	return func(svr *TcpServer) {
		maxPacketDataSize = size
	}
}

func WithDispatcherPoolSize(size int) Options {
	return func(svr *TcpServer) {
		defaultPoolSize = size
	}
}
