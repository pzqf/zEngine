package zActor

import (
	"fmt"
	"time"
)

// 示例：如何使用Actor模型和消息队列

// ========== 示例1：创建和使用基础Actor ==========

// CustomActor 自定义Actor类型
type CustomActor struct {
	*BaseActor
	counter int
}

// CustomMessage 自定义消息类型
type CustomMessage struct {
	BaseActorMessage
	Value int
}

// ProcessMessage 实现ProcessMessage方法
func (ca *CustomActor) ProcessMessage(msg ActorMessage) {
	switch msg := msg.(type) {
	case *CustomMessage:
		ca.counter += msg.Value
		fmt.Printf("CustomActor %d received message with value %d, counter is now %d\n", ca.ID(), msg.Value, ca.counter)
	default:
		fmt.Printf("CustomActor %d received unknown message\n", ca.ID())
	}
}

// ActorExample1 创建和使用基础Actor
func ActorExample1() {
	// 创建Actor系统
	actorSystem := NewActorSystem()

	// 启动Actor系统
	if err := actorSystem.Start(); err != nil {
		fmt.Printf("Failed to start actor system: %v\n", err)
		return
	}

	// 创建自定义Actor实例
	customActor := &CustomActor{
		BaseActor: NewBaseActor(1, 10),
		counter:   0,
	}

	// 注册Actor到系统
	if err := actorSystem.RegisterActor(customActor); err != nil {
		fmt.Printf("Failed to register actor: %v\n", err)
		return
	}

	// 发送消息给Actor
	for i := 0; i < 5; i++ {
		msg := &CustomMessage{
			BaseActorMessage: BaseActorMessage{ActorID: 1},
			Value:            i * 10,
		}
		actorSystem.SendMessage(1, msg)
	}

	// 等待消息处理完成
	time.Sleep(500 * time.Millisecond)

	// 从系统中注销Actor
	if err := actorSystem.UnregisterActor(1); err != nil {
		fmt.Printf("Failed to unregister actor: %v\n", err)
		return
	}

	// 停止Actor系统
	if err := actorSystem.Stop(); err != nil {
		fmt.Printf("Failed to stop actor system: %v\n", err)
		return
	}

	fmt.Println("Actor example 1 completed")
}

// ========== 示例2：使用事件总线和Actor的组合 ==========

// EventListenerActor 事件监听Actor（简化版，不使用zEvent）
type EventListenerActor struct {
	*BaseActor
}

// ProcessMessage 实现ProcessMessage方法
func (ela *EventListenerActor) ProcessMessage(msg ActorMessage) {
	fmt.Printf("EventListenerActor %d received message: %T\n", ela.ID(), msg)
}

// ActorExample2 使用多个Actor的组合（简化版，不使用zEvent）
func ActorExample2() {
	// 创建Actor系统
	actorSystem := NewActorSystem()

	// 启动Actor系统
	if err := actorSystem.Start(); err != nil {
		fmt.Printf("Failed to start actor system: %v\n", err)
		return
	}

	// 创建并注册Actor
	listenerActor := &EventListenerActor{
		BaseActor: NewBaseActor(2, 10),
	}

	if err := actorSystem.RegisterActor(listenerActor); err != nil {
		fmt.Printf("Failed to register listener actor: %v\n", err)
		return
	}

	// 这里可以添加向listenerActor发送消息的代码示例
	fmt.Println("Actor example 2: Listener actor created and registered")

	// 等待处理完成
	time.Sleep(500 * time.Millisecond)

	// 从系统中注销Actor
	if err := actorSystem.UnregisterActor(2); err != nil {
		fmt.Printf("Failed to unregister listener actor: %v\n", err)
		return
	}

	// 停止Actor系统
	if err := actorSystem.Stop(); err != nil {
		fmt.Printf("Failed to stop actor system: %v\n", err)
		return
	}

	fmt.Println("Actor example 2 completed")
}

// ========== 示例3：展示如何使用PlayerActor ==========

// PlayerActorExample 展示如何使用PlayerActor
// 注意：此示例需要导入player包，这里仅作为伪代码示例
func PlayerActorExample() {
	// 获取全局Actor系统
	// actorSystem := GetGlobalActorSystem()

	// 创建玩家会话（假设已实现）
	// session := &zNet.TcpServerSession{}

	// 获取日志记录器
	// logger := zLog.GetLogger()

	// 创建PlayerActor实例
	// playerActor := player.NewPlayerActor(12345, "Player1", session, logger)

	// 注册PlayerActor到系统
	// if err := actorSystem.RegisterActor(playerActor); err != nil {
	//     logger.Error("Failed to register player actor", zap.Int64("playerId", playerActor.GetPlayerId()), zap.Error(err))
	//     return
	// }

	// 发送连接消息
	// connectMsg := &player.PlayerActorConnectMessage{
	//     BaseActorMessage: event.BaseActorMessage{ActorID: playerActor.GetPlayerId()},
	//     Session: session,
	// }
	// actorSystem.SendMessage(playerActor.GetPlayerId(), connectMsg)

	// 发送增加经验消息
	// addExpMsg := &player.PlayerActorAddExpMessage{
	//     BaseActorMessage: event.BaseActorMessage{ActorID: playerActor.GetPlayerId()},
	//     Exp: 100,
	// }
	// actorSystem.SendMessage(playerActor.GetPlayerId(), addExpMsg)

	// 发送增加金币消息
	// addGoldMsg := &player.PlayerActorAddGoldMessage{
	//     BaseActorMessage: event.BaseActorMessage{ActorID: playerActor.GetPlayerId()},
	//     Gold: 50,
	// }
	// actorSystem.SendMessage(playerActor.GetPlayerId(), addGoldMsg)

	// 发送使用物品消息
	// useItemMsg := &player.PlayerActorUseItemMessage{
	//     BaseActorMessage: event.BaseActorMessage{ActorID: playerActor.GetPlayerId()},
	//     ItemID: 1001,
	//     Slot: 1,
	// }
	// actorSystem.SendMessage(playerActor.GetPlayerId(), useItemMsg)

	// 发送断开连接消息
	// disconnectMsg := &player.PlayerActorDisconnectMessage{
	//     BaseActorMessage: event.BaseActorMessage{ActorID: playerActor.GetPlayerId()},
	// }
	// actorSystem.SendMessage(playerActor.GetPlayerId(), disconnectMsg)

	// 从系统中注销PlayerActor
	// if err := actorSystem.UnregisterActor(playerActor.GetPlayerId()); err != nil {
	//     logger.Error("Failed to unregister player actor", zap.Int64("playerId", playerActor.GetPlayerId()), zap.Error(err))
	//     return
	// }

	fmt.Println("PlayerActor example completed (pseudocode)")
}

// ========== 初始化函数，用于演示 ==========

// init 初始化函数，用于演示
func init() {
	// 只在测试时启用示例
	// fmt.Println("Running Actor examples...")
	// ActorExample1()
	// ActorExample2()
	// PlayerActorExample()
	// fmt.Println("All Actor examples completed")
}
