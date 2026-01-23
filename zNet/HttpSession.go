package zNet

import "net/http"

type HttpSession struct {
	writer http.ResponseWriter
	sid    SessionIdType
}

func NewHttpSession(writer http.ResponseWriter, sid SessionIdType) *HttpSession {
	return &HttpSession{
		writer: writer,
		sid:    sid,
	}
}

func (s *HttpSession) Send(protoId int32, data []byte) error {
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
