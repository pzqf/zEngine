package zNet

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
	"github.com/pzqf/zUtil/zMap"
)

type WebSocketServer struct {
	clientSIDAtomic SessionIdType

	clientSessionMap zMap.Map
	wg               sync.WaitGroup
	onAddSession     SessionCallBackFunc
	onRemoveSession  SessionCallBackFunc

	listener net.Listener
	addr     string
	upgrade  *websocket.Upgrader
}

func NewWebSocketServer(cfg *Config, addSessionFunc, removeSessionFunc SessionCallBackFunc) *WebSocketServer {
	svr := &WebSocketServer{
		clientSIDAtomic:  10000,
		clientSessionMap: zMap.NewMap(),
		addr:             cfg.ListenAddress,
		upgrade: &websocket.Upgrader{
			ReadBufferSize:  int(maxPacketDataSize),
			WriteBufferSize: int(maxPacketDataSize),
			CheckOrigin: func(r *http.Request) bool {
				if r.Method != "GET" {
					fmt.Println("method is not GET")
					return false
				}
				if r.URL.Path != "/ws" {
					fmt.Println("path error")
					return false
				}
				return true
			},
		},

		onAddSession:    addSessionFunc,
		onRemoveSession: removeSessionFunc,
	}

	GConfig = cfg

	return svr
}

func (svr *WebSocketServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/ws" {
		httpCode := http.StatusInternalServerError
		reusePhrase := http.StatusText(httpCode)
		fmt.Println("path error ", reusePhrase)
		http.Error(w, reusePhrase, httpCode)
		return
	}
	conn, err := svr.upgrade.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("websocket error:", err)
		return
	}
	fmt.Println("client connect :", conn.RemoteAddr())
	go svr.AddSession(conn)

}

func (svr *WebSocketServer) AddSession(conn *websocket.Conn) *WebSocketServerSession {
	sid := atomic.AddUint64(&svr.clientSIDAtomic, 1)

	newSession := NewWebSocketServerSession(conn, sid, GConfig.ChanSize, svr.RemoveSession)

	svr.clientSessionMap.Store(sid, newSession)

	if svr.onAddSession != nil {
		svr.onAddSession(newSession.sid)
	}

	newSession.Start()

	return newSession
}

func (svr *WebSocketServer) RemoveSession(cli *WebSocketServerSession) {
	if svr.onRemoveSession != nil {
		svr.onRemoveSession(cli.sid)
	}
	svr.clientSessionMap.Delete(cli.sid)
}

func (svr *WebSocketServer) Start() error {
	go func() {
		var err error
		svr.listener, err = net.Listen("tcp", svr.addr)
		if err != nil {
			fmt.Println("net listen error:", err)
			return
		}
		err = http.Serve(svr.listener, svr)
		if err != nil {
			fmt.Println("http serve error:", err)
			return
		}
	}()

	return nil
}

func (svr *WebSocketServer) Close() error {
	_ = svr.listener.Close()
	svr.clientSessionMap.Range(func(key, value interface{}) bool {
		session := value.(*WebSocketServerSession)
		session.Close()
		svr.clientSessionMap.Delete(session.sid)
		return true
	})

	svr.wg.Wait()

	return nil
}
