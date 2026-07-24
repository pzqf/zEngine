package zNet

import (
	"bytes"
	"crypto/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// 本文件是旧 server_test.go 的重写（成熟化改造收尾）。旧版问题：① 固定端口 8081-8099
// 并行/占用即冲突；② 断言薄弱且自相矛盾（`t.Errorf(...received 0)` 后又 `t.Log("passed")`），
// 多条实际处于 FAIL。重写为：临时端口（`:0` + `GetListenAddress`，现为原子安全）、
// 强断言（原子计数 + 轮询等待）、`t.Cleanup` 保证释放，杜绝端口冲突与资源泄漏，且 `-race` 干净。

// startTestServer 在临时端口启动一台服务器（dispatcher 由调用方给定），返回其监听地址。
// 服务器经 t.Cleanup 自动关闭。
func startTestServer(t *testing.T, dispatcher HandlerFun, opts ...Options) (string, *TcpServer) {
	t.Helper()
	cfg := &TcpConfig{
		ListenAddress:     "127.0.0.1:0",
		MaxClientCount:    100,
		ChanSize:          1024,
		HeartbeatDuration: 30,
		MaxPacketDataSize: 1024 * 1024,
		DisableEncryption: true,
	}
	srv := NewTcpServer(cfg, opts...)
	if dispatcher != nil {
		srv.RegisterDispatcher(dispatcher)
	}
	go func() { _ = srv.Start() }()
	t.Cleanup(func() { srv.Close() })

	var addr string
	if !waitFor(t, 2*time.Second, func() bool {
		addr = srv.GetListenAddress()
		return addr != ""
	}) {
		t.Fatal("server did not report a listen address in time")
	}
	return addr, srv
}

// dialTestClient 连接 addr（默认 no-op dispatcher，除非给定），经 t.Cleanup 自动关闭。
func dialTestClient(t *testing.T, addr string, dispatcher HandlerFun) *TcpClient {
	t.Helper()
	host, port := parseAddr(addr)
	cli := NewTcpClient(&TcpClientConfig{DisableEncryption: true, ServerAddr: host, ServerPort: port})
	if dispatcher == nil {
		dispatcher = func(Session, *NetPacket) error { return nil }
	}
	cli.RegisterDispatcher(dispatcher)
	if err := cli.Connect(); err != nil {
		t.Fatalf("client connect to %s: %v", addr, err)
	}
	t.Cleanup(func() { cli.Close() })
	return cli
}

// waitFor 轮询直到 cond 为真或超时，返回最终结果。
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

// TestServer_ReceivesClientMessage 断言服务器确实收到客户端消息（强断言，非仅打印）。
func TestServer_ReceivesClientMessage(t *testing.T) {
	var got atomic.Int64
	addr, _ := startTestServer(t, func(session Session, p *NetPacket) error {
		got.Add(1)
		return nil
	})
	cli := dialTestClient(t, addr, nil)

	if err := cli.Send(1000, []byte("hello")); err != nil {
		t.Fatalf("send: %v", err)
	}
	if !waitFor(t, 2*time.Second, func() bool { return got.Load() == 1 }) {
		t.Fatalf("server must receive exactly 1 message, got %d", got.Load())
	}
}

// TestServer_MultipleMessages 断言 N 条消息全部被服务器接收。
func TestServer_MultipleMessages(t *testing.T) {
	const n = 50
	var got atomic.Int64
	addr, _ := startTestServer(t, func(session Session, p *NetPacket) error {
		got.Add(1)
		return nil
	})
	cli := dialTestClient(t, addr, nil)

	for i := 0; i < n; i++ {
		if err := cli.Send(ProtoIdType(1000+i), []byte("msg")); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	if !waitFor(t, 3*time.Second, func() bool { return got.Load() == n }) {
		t.Fatalf("server must receive %d messages, got %d", n, got.Load())
	}
}

// TestServer_ConcurrentClients 断言多客户端并发发送时服务器收齐全部消息。
func TestServer_ConcurrentClients(t *testing.T) {
	const clients = 8
	const perClient = 25
	var got atomic.Int64
	// 回归守护：8 个客户端从同一 IP(127.0.0.1) 并发发送。此前 TrafficLimiter.AllowTraffic
	// 用单次 CAS，同 IP 并发时 CAS 失败被误判为"流量超限"→随机断连丢消息（本测试实测 ~30% 失败）。
	// 修复（循环 CAS 重试）后应稳定收齐全部消息。
	addr, _ := startTestServer(t, func(session Session, p *NetPacket) error {
		got.Add(1)
		return nil
	})

	// 在主 goroutine 建立所有客户端（dialTestClient 用到 t.Fatalf/t.Cleanup，不可在子 goroutine 调），
	// 再并发发送（Send 不涉及 testing.T）。
	conns := make([]*TcpClient, clients)
	for c := 0; c < clients; c++ {
		conns[c] = dialTestClient(t, addr, nil)
	}
	var wg sync.WaitGroup
	for c := 0; c < clients; c++ {
		wg.Add(1)
		go func(cli *TcpClient) {
			defer wg.Done()
			for i := 0; i < perClient; i++ {
				_ = cli.Send(2000, []byte("x"))
			}
		}(conns[c])
	}
	wg.Wait()

	want := int64(clients * perClient)
	if !waitFor(t, 5*time.Second, func() bool { return got.Load() == want }) {
		t.Fatalf("server must receive %d messages from %d concurrent clients, got %d", want, clients, got.Load())
	}
}

// TestServer_SessionCallbacks 断言会话建立/移除回调各触发一次，且会话数随之增减。
func TestServer_SessionCallbacks(t *testing.T) {
	var added, removed atomic.Int64
	addr, srv := startTestServer(t, nil,
		WithAddSessionCallBack(func(SessionIdType) { added.Add(1) }),
		WithRemoveSessionCallBack(func(SessionIdType) { removed.Add(1) }),
	)

	cli := dialTestClient(t, addr, nil)
	if !waitFor(t, 2*time.Second, func() bool { return added.Load() == 1 }) {
		t.Fatalf("onAddSession must fire once, got %d", added.Load())
	}
	if !waitFor(t, 2*time.Second, func() bool { return srv.clientSessionMap.Len() == 1 }) {
		t.Fatalf("server must track 1 session, got %d", srv.clientSessionMap.Len())
	}

	cli.Close()
	if !waitFor(t, 3*time.Second, func() bool { return removed.Load() == 1 }) {
		t.Fatalf("onRemoveSession must fire once after client close, got %d", removed.Load())
	}
}

// TestServer_LargeDataRoundTrip 断言大数据包（256KB）被完整接收且内容一致。
func TestServer_LargeDataRoundTrip(t *testing.T) {
	payload := make([]byte, 256*1024)
	if _, err := rand.Read(payload); err != nil {
		t.Fatalf("rand: %v", err)
	}

	var okContent atomic.Bool
	done := make(chan struct{}, 1)
	addr, _ := startTestServer(t, func(session Session, p *NetPacket) error {
		if bytes.Equal(p.Data, payload) {
			okContent.Store(true)
		}
		select {
		case done <- struct{}{}:
		default:
		}
		return nil
	})
	cli := dialTestClient(t, addr, nil)

	if err := cli.Send(3000, payload); err != nil {
		t.Fatalf("send large: %v", err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("large data packet was not received in time")
	}
	if !okContent.Load() {
		t.Fatal("received large data did not match sent content")
	}
}

// TestServer_TwoServersIndependent 断言两台服务器各自独立收发（取代旧的固定端口 server-to-server）。
func TestServer_TwoServersIndependent(t *testing.T) {
	var got1, got2 atomic.Int64
	addr1, _ := startTestServer(t, func(session Session, p *NetPacket) error { got1.Add(1); return nil })
	addr2, _ := startTestServer(t, func(session Session, p *NetPacket) error { got2.Add(1); return nil })

	c1 := dialTestClient(t, addr1, nil)
	c2 := dialTestClient(t, addr2, nil)
	_ = c1.Send(1, []byte("to-1"))
	_ = c2.Send(2, []byte("to-2"))
	_ = c2.Send(2, []byte("to-2"))

	if !waitFor(t, 2*time.Second, func() bool { return got1.Load() == 1 && got2.Load() == 2 }) {
		t.Fatalf("independent servers: got1=%d(want 1) got2=%d(want 2)", got1.Load(), got2.Load())
	}
}

// TestClientSession_ClosedOnRealNetworkDrop 回归守护：**真实网络掉线**（服务端关闭连接，
// 而非客户端显式 Close）后，客户端会话必须报告 IsClosed()=true。否则 TcpClient.monitorConnection
// 靠 IsClosed() 判定掉线将永远检测不到、AutoReconnect 永不触发（此前 receive 读错误退出时
// 不设 closed，是真实的重连失效 bug）。
func TestClientSession_ClosedOnRealNetworkDrop(t *testing.T) {
	addr, srv := startTestServer(t, nil)
	client := dialTestClient(t, addr, nil) // AutoReconnect 默认 false：只验证 closed 标志，不掺入重连
	session := client.GetSession()
	if session == nil || session.IsClosed() {
		t.Fatal("precondition: connected session should exist and not be closed")
	}

	// 服务端关闭 → 客户端 receive 读到 EOF/连接错误而退出 → 应标记 closed。
	srv.Close()

	if !waitFor(t, 6*time.Second, func() bool { return session.IsClosed() }) {
		t.Fatal("client session must report IsClosed()=true after real network drop (server closed)")
	}
}
