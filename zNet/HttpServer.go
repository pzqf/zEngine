package zNet

import (
	"net/http"
)

type HttpServer struct {
	server *http.Server
	mux    *http.ServeMux
	addr   string
}

func NewHttpServer(addr string) *HttpServer {
	svr := &HttpServer{
		mux:  http.NewServeMux(),
		addr: addr,
	}

	return svr
}

func (svr *HttpServer) Start() error {
	svr.server = &http.Server{
		Addr:    svr.addr,
		Handler: svr.mux,
	}

	go func() {
		err := svr.server.ListenAndServe()
		if err != nil {
			if err == http.ErrServerClosed {
				LogPrint("Server closed under request")
			} else {
				LogPrint("Server closed unexpected", err)
			}
		}
	}()

	return nil
}

func (svr *HttpServer) Close() {
	LogPrint("Close http server")

	_ = svr.server.Close()
}

func (svr *HttpServer) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	//LogPrint("http register", zap.String("route", pattern), zap.String("func", runtime.FuncForPC(reflect.ValueOf(handler).Pointer()).Name()))
	svr.mux.HandleFunc(pattern, handler)
}
