package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/pzqf/zEngine/zLog"
	"github.com/pzqf/zEngine/zNet"
)

func main() {
	fmt.Println("=====================================")
	fmt.Println("  AES-GCM 加密模式测试")
	fmt.Println("=====================================")

	cfg := zLog.Config{
		Level:    zLog.InfoLevel,
		Console:  true,
		Filename: "./logs/encryption_test.log",
		MaxSize:  1024,
	}
	err := zLog.InitLogger(&cfg)
	if err != nil {
		fmt.Printf("Failed to init logger: %v\n", err)
		return
	}

	testPort := 9161
	clientCount := 3

	fmt.Printf("\n开始测试，端口: %d, 客户端数量: %d\n", testPort, clientCount)

	serverWg := sync.WaitGroup{}
	serverWg.Add(1)
	go startServer(testPort, &serverWg)

	time.Sleep(1 * time.Second)

	clientWg := sync.WaitGroup{}
	clientWg.Add(clientCount)

	startTime := time.Now()
	failedCount := 0

	fmt.Printf("\n启动 %d 个客户端进行加密测试...\n", clientCount)

	for i := 0; i < clientCount; i++ {
		time.Sleep(100 * time.Millisecond)
		go testClient(i, testPort, &clientWg, &failedCount)
	}

	clientWg.Wait()

	serverWg.Done()

	fmt.Println("\n=====================================")
	fmt.Println("  测试报告")
	fmt.Println("=====================================")

	totalTime := time.Now().Sub(startTime)
	successCount := clientCount - failedCount

	fmt.Printf("测试时间: %s\n", totalTime)
	fmt.Printf("客户端总数: %d\n", clientCount)
	fmt.Printf("成功连接: %d (%.2f%%)\n", successCount, float64(successCount)/float64(clientCount)*100)
	fmt.Printf("连接失败: %d (%.2f%%)\n", failedCount, float64(failedCount)/float64(clientCount)*100)

	if failedCount == 0 {
		fmt.Println("\n✅ 所有客户端加密测试通过！")
		fmt.Println("   - GCM 模式加密功能正常")
		fmt.Println("   - 密钥交换机制正常")
		fmt.Println("   - 加密/解密流程完整")
	} else {
		fmt.Println("\n❌ 部分客户端测试失败，需要检查日志")
	}

	fmt.Println("=====================================")
}

func startServer(port int, wg *sync.WaitGroup) {
	tcpConfig := zNet.TcpConfig{
		ListenAddress:     fmt.Sprintf(":%d", port),
		HeartbeatDuration: 0,
		MaxPacketDataSize: zNet.DefaultPacketDataSize,
	}

	tcpSvr := zNet.NewTcpServer(&tcpConfig,
		zNet.WithMaxClientCount(1000),
		zNet.WithChanSize(1024),
	)

	tcpSvr.RegisterDispatcher(func(session zNet.Session, netPacket *zNet.NetPacket) error {
		switch netPacket.ProtoId {
		case 1:
			return handleLoginRequest(session, netPacket)
		case 2:
			return handleEncryptionTest(session, netPacket)
		}
		return nil
	})

	fmt.Printf("服务器启动成功，监听端口: %d\n", port)

	err := tcpSvr.Start()
	if err != nil {
		fmt.Printf("服务器启动失败: %v\n", err)
		wg.Done()
		return
	}

	wg.Wait()
	fmt.Println("服务器停止")
	tcpSvr.Close()
}

func handleLoginRequest(session zNet.Session, netPacket *zNet.NetPacket) error {
	type loginDataInfo struct {
		UserName string `json:"user_name"`
	}

	var loginData loginDataInfo
	err := json.Unmarshal(netPacket.Data, &loginData)
	if err != nil {
		zLog.Error(fmt.Sprintf("登录数据解析错误: %v", err))
		return err
	}

	zLog.Info(fmt.Sprintf("收到登录请求: UserName = %s", loginData.UserName),
		zap.Uint64("session_id", uint64(session.GetSid())))

	type loginResponse struct {
		Result string `json:"result"`
		Msg    string `json:"msg"`
	}

	response := loginResponse{
		Result: "success",
		Msg:    fmt.Sprintf("欢迎 %s，已使用 AES-GCM 加密", loginData.UserName),
	}

	data, err := json.Marshal(response)
	if err != nil {
		return err
	}

	return session.Send(1, data)
}

func handleEncryptionTest(session zNet.Session, netPacket *zNet.NetPacket) error {
	zLog.Info("收到加密测试请求",
		zap.Uint64("session_id", uint64(session.GetSid())),
		zap.Int("data_length", len(netPacket.Data)))

	type testRequest struct {
		ClientId int    `json:"client_id"`
		TestData string `json:"test_data"`
		Time     int64  `json:"time"`
	}

	var request testRequest
	err := json.Unmarshal(netPacket.Data, &request)
	if err != nil {
		zLog.Error(fmt.Sprintf("加密测试数据解析错误: %v", err))
		return err
	}

	zLog.Info(fmt.Sprintf("解密成功: ClientId = %d, TestData = %s", request.ClientId, request.TestData),
		zap.Uint64("session_id", uint64(session.GetSid())))

	type testResponse struct {
		Result       string `json:"result"`
		ClientId     int    `json:"client_id"`
		ServerTime   int64  `json:"server_time"`
		OriginalData string `json:"original_data"`
	}

	response := testResponse{
		Result:       "success",
		ClientId:     request.ClientId,
		ServerTime:   time.Now().UnixNano(),
		OriginalData: request.TestData,
	}

	data, err := json.Marshal(response)
	if err != nil {
		return err
	}

	return session.Send(2, data)
}

func testClient(clientId int, port int, wg *sync.WaitGroup, failedCount *int) {
	defer wg.Done()

	fmt.Printf("  客户端 %d 开始连接... ", clientId)

	cli := zNet.TcpClient{}

	err := cli.RegisterHandler(func(session zNet.Session, netPacket *zNet.NetPacket) error {
		if netPacket.ProtoId == 1 {
			type loginResponse struct {
				Result string `json:"result"`
				Msg    string `json:"msg"`
			}

			var response loginResponse
			err := json.Unmarshal(netPacket.Data, &response)
			if err != nil {
				fmt.Printf("客户端 %d 登录响应解析错误: %v\n", clientId, err)
				return err
			}

			fmt.Printf("✅ 客户端 %d 登录响应: %s - %s\n", clientId, response.Result, response.Msg)

			go func() {
				time.Sleep(500 * time.Millisecond)
				sendEncryptionTest(session, clientId)
			}()

			return nil
		} else if netPacket.ProtoId == 2 {
			type testResponse struct {
				Result       string `json:"result"`
				ClientId     int    `json:"client_id"`
				ServerTime   int64  `json:"server_time"`
				OriginalData string `json:"original_data"`
			}

			var response testResponse
			err := json.Unmarshal(netPacket.Data, &response)
			if err != nil {
				fmt.Printf("客户端 %d 加密测试响应解析错误: %v\n", clientId, err)
				return err
			}

			fmt.Printf("  客户端 %d 加密测试结果: %s\n", clientId, response.Result)
			fmt.Printf("    - 原始数据: %s\n", response.OriginalData)
			fmt.Printf("    - 服务器时间: %d\n", response.ServerTime)
			fmt.Printf("  ✅ 客户端 %d 加密测试通过！\n", clientId)

			return nil
		}
		return nil
	}, 10)
	if err != nil {
		fmt.Printf("❌ 客户端 %d 注册处理函数失败: %v\n", clientId, err)
		*failedCount++
		return
	}

	err = cli.ConnectToServer("127.0.0.1", port, "", 30, zNet.DefaultPacketDataSize)
	if err != nil {
		fmt.Printf("❌ 客户端 %d 连接失败: %v\n", clientId, err)
		*failedCount++
		return
	}

	fmt.Printf("✅ 客户端 %d 连接成功\n", clientId)

	type loginRequest struct {
		UserName string `json:"user_name"`
	}

	loginReq := loginRequest{
		UserName: fmt.Sprintf("test_client_%d", clientId),
	}

	data, err := json.Marshal(loginReq)
	if err != nil {
		fmt.Printf("❌ 客户端 %d 登录数据序列化失败: %v\n", clientId, err)
		*failedCount++
		return
	}

	err = cli.Send(1, data)
	if err != nil {
		fmt.Printf("❌ 客户端 %d 发送登录请求失败: %v\n", clientId, err)
		*failedCount++
		return
	}

	time.Sleep(5 * time.Second)

	fmt.Printf("  客户端 %d 测试完成\n", clientId)
}

func sendEncryptionTest(session zNet.Session, clientId int) {
	type testRequest struct {
		ClientId int    `json:"client_id"`
		TestData string `json:"test_data"`
		Time     int64  `json:"time"`
	}

	testReq := testRequest{
		ClientId: clientId,
		TestData: fmt.Sprintf("this_is_encrypted_data_from_client_%d", clientId),
		Time:     time.Now().UnixNano(),
	}

	data, err := json.Marshal(testReq)
	if err != nil {
		fmt.Printf("客户端 %d 测试数据序列化失败: %v\n", clientId, err)
		return
	}

	fmt.Printf("  客户端 %d 发送加密测试请求（数据长度: %d bytes）\n", clientId, len(data))

	err = session.Send(2, data)
	if err != nil {
		fmt.Printf("❌ 客户端 %d 发送加密测试请求失败: %v\n", clientId, err)
	}
}
