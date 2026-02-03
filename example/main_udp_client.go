package main

import (
	"fmt"
	"log"

	"github.com/pzqf/zEngine/zLog"
	"github.com/pzqf/zEngine/zNet"
)

func main() {
	// 创建UDP客户端
	client := &zNet.UdpClient{}

	// 设置日志
	logger := zLog.GetStandardLogger()
	client.SetLogger(logger)

	// 设置消息处理器
	client.SetDispatcher(func(session zNet.Session, packet *zNet.NetPacket) error {
		fmt.Printf("Received packet: ProtoId=%d, Data=%s\n", packet.ProtoId, string(packet.Data))
		return nil
	})

	// 连接服务器
	err := client.ConnectToServer("127.0.0.1", 8080, 30, 1024)
	if err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}

	// 发送测试消息
	testData := []byte("Hello UDP Server!")
	err = client.Send(1001, testData)
	if err != nil {
		log.Printf("Failed to send message: %v", err)
	} else {
		fmt.Println("Message sent successfully")
	}

	// 等待输入
	fmt.Println("UDP client started. Press Enter to exit.")
	fmt.Scanln()

	// 关闭客户端
	client.Close()
	fmt.Println("UDP client closed.")
}
