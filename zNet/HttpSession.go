package zNet

import "net/http"

type HttpSession struct {
	writer   http.ResponseWriter
	sid      SessionIdType
	clientIP string
}

func NewHttpSession(writer http.ResponseWriter, sid SessionIdType, clientIP string) *HttpSession {
	return &HttpSession{
		writer:   writer,
		sid:      sid,
		clientIP: clientIP,
	}
}

func (s *HttpSession) Send(protoId ProtoIdType, data []byte) error {
	_, _ = s.writer.Write(data)
	return nil
}

func (s *HttpSession) GetSid() SessionIdType {
	return s.sid
}

func (s *HttpSession) GetWriter() http.ResponseWriter {
	return s.writer
}

func (s *HttpSession) Start() {
	// HTTP会话不需要特殊的启动逻辑
}

func (s *HttpSession) Close() {
	// HTTP会话不需要特殊的关闭逻辑，因为HTTP是无状态的
}

// GetClientIP 获取客户端IP地址
func (s *HttpSession) GetClientIP() string {
	return s.clientIP
}
