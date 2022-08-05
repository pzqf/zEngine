package zNet

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pzqf/zUtil/zMap"
)

type UdpServer struct {
	clientSIDAtomic  int64
	conn             *net.UDPConn
	clientSessionMap zMap.Map
	wg               sync.WaitGroup
	config           *UdpConfig
}

func NewUdpServer(cfg *UdpConfig, opts ...Options) *UdpServer {
	if cfg.ListenAddress == "" {
		cfg.ListenAddress = ":9160"
	}
	if cfg.ChanSize <= 0 {
		cfg.ChanSize = 2048
	}
	if cfg.MaxClientCount <= 0 {
		cfg.MaxClientCount = 10000
	}

	svr := &UdpServer{
		clientSIDAtomic:  10000,
		clientSessionMap: zMap.NewMap(),
		config:           cfg,
	}

	for _, opt := range opts {
		opt(svr)
	}

	return svr
}

func (svr *UdpServer) Start() error {
	udpAddr, err := net.ResolveUDPAddr("udp", svr.config.ListenAddress)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	svr.conn = conn

	LogPrint(fmt.Sprintf("Udp server listing on %s ", svr.config.ListenAddress))

	go func() {
		svr.wg.Add(1)
		defer svr.wg.Done()
		for {
			if svr.clientSessionMap.Len() >= int32(svr.config.MaxClientCount) {
				LogPrint(fmt.Sprintf("Maximum connections exceeded, max:%d", svr.config.MaxClientCount))
				time.Sleep(5 * time.Millisecond)
				continue
			}
			dataBUf := make([]byte, maxPacketDataSize)
			n, addr, err := svr.conn.ReadFromUDP(dataBUf)
			if err != nil {
				LogPrint(err)
				break
			}

			session, ok := svr.clientSessionMap.Get(addr.String())
			if !ok {
				sid := SessionIdType(atomic.AddInt64(&svr.clientSIDAtomic, 1))
				newSession := NewUdpServerSession(svr.config, conn, addr, sid)
				svr.clientSessionMap.Store(addr.String(), newSession)
				session = newSession
				newSession.Start()
			}

			go session.(*UdpServerSession).Receive(dataBUf[:n])
		}
	}()

	return nil
}

func (svr *UdpServer) Close() {
	LogPrint("Close tcp server, session count ", svr.clientSessionMap.Len())

	_ = svr.conn.Close()

	svr.clientSessionMap.Range(func(key, value interface{}) bool {
		session := value.(*UdpServerSession)
		session.Close()
		svr.clientSessionMap.Delete(session.sid)
		return true
	})

	svr.wg.Wait()
}

func (svr *UdpServer) RemoveSession(cli *UdpServerSession) {
	svr.clientSessionMap.Delete(cli.sid)
}

func (svr *UdpServer) GetSession(sid int64) *UdpServerSession {
	if client, ok := svr.clientSessionMap.Get(sid); ok {
		return client.(*UdpServerSession)
	}
	return nil
}

func (svr *UdpServer) GetAllSession() []*UdpServerSession {
	var sessionList []*UdpServerSession
	svr.clientSessionMap.Range(func(key, value interface{}) bool {
		sessionList = append(sessionList, value.(*UdpServerSession))
		return true
	})

	return sessionList
}
