package zNet

import "net/http"

type httpSession struct {
	writer http.ResponseWriter
}

func NewHttpSession(writer http.ResponseWriter) *httpSession {
	return &httpSession{
		writer: writer,
	}
}

func (s *httpSession) Send(protoId int32, data []byte) error {
	_, _ = s.writer.Write(data)
	return nil
}

func (s *httpSession) GetSid() SessionIdType {
	return 0
}
