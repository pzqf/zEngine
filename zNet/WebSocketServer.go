package zNet

import (
	"net"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
	"github.com/pzqf/zUtil/zMap"
)

type WebSocketServer struct {
	Dispatcher
	clientSIDAtomic  SessionIdType
	clientSessionMap zMap.Map
	wg               sync.WaitGroup
	onAddSession     SessionCallBackFunc
	onRemoveSession  SessionCallBackFunc
	listener         net.Listener
	addr             string
	upgrade          *websocket.Upgrader
	config           *WebSocketConfig
}

func NewWebSocketServer(cfg *WebSocketConfig, opts ...Options) *WebSocketServer {
	svr := &WebSocketServer{
		clientSIDAtomic:  10000,
		clientSessionMap: zMap.NewMap(),
		addr:             cfg.ListenAddress,
		upgrade: &websocket.Upgrader{
			ReadBufferSize:  int(maxPacketDataSize),
			WriteBufferSize: int(maxPacketDataSize),
			CheckOrigin: func(r *http.Request) bool {
				if r.Method != "GET" {
					LogPrint("method is not GET")
					return false
				}
				return true
			},
		},
		config: cfg,
	}

	for _, opt := range opts {
		opt(svr)
	}

	return svr
}

func (svr *WebSocketServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := svr.upgrade.Upgrade(w, r, nil)
	if err != nil {
		LogPrint("websocket Upgrade error:", err)
		return
	}
	//fmt.Println("client connect :", conn.RemoteAddr())
	go svr.AddSession(conn)

}

func (svr *WebSocketServer) AddSession(conn *websocket.Conn) *WebSocketServerSession {
	sid := atomic.AddUint64(&svr.clientSIDAtomic, 1)

	newSession := NewWebSocketServerSession(svr.config, conn, sid, svr.RemoveSession, svr.DispatcherFun)

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
			LogPrint("net listen error:", err)
			return
		}

		err = http.Serve(svr.listener, svr)
		if err != nil {
			LogPrint("http serve error:", err)
			return
		}
	}()

	return nil
}

func (svr *WebSocketServer) Close() {
	_ = svr.listener.Close()
	svr.clientSessionMap.Range(func(key, value interface{}) bool {
		session := value.(*WebSocketServerSession)
		session.Close()
		svr.clientSessionMap.Delete(session.sid)
		return true
	})

	svr.wg.Wait()

	return
}
