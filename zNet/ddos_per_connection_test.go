package zNet

import "testing"

// 这些用例钉死一条产品级要求：**共享出口 IP 的玩家之间不得互相连坐**。
//
// 背景：原实现把包频率/流量都按 IP 聚合，且一超限就 BlacklistIP（默认封 24 小时）。
// NAT/CGNAT 下网吧、公司、乃至整个运营商城域网的玩家共用一个出口 IP，
// 任何一个人（甚至只是重传突发）越限，整段 IP 的人一起掉线且连不回来。
// 现在包/流量按**连接**限，只掐越限的那条连接；只有连接洪水才按 IP 拦并拉黑。

func perConnTestConfig() *DDoSConfig {
	return &DDoSConfig{
		MaxConnPerIP:      100,
		ConnTimeWindow:    60,
		MaxPacketsPerIP:   3, // 故意设小，便于触发
		PacketTimeWindow:  60,
		MaxBytesPerIP:     100,
		TrafficTimeWindow: 60,
		BanDuration:       3600,
	}
}

// TestDDoS_PacketLimitIsPerConnection 同一 IP 上的两条连接各限各的：
// 一条刷爆不影响另一条。
func TestDDoS_PacketLimitIsPerConnection(t *testing.T) {
	dp := NewDDoSProtection(perConnTestConfig())

	const ip = "203.0.113.7"
	noisy, quiet := ip+"#1", ip+"#2"

	// 吵闹的那条把自己的额度用光。
	for i := 0; i < 3; i++ {
		if !dp.AllowPacketFrom(noisy, ip) {
			t.Fatalf("额度内的第 %d 个包不应被拒", i+1)
		}
	}
	if dp.AllowPacketFrom(noisy, ip) {
		t.Fatalf("超额后该连接应被拒")
	}

	// 同 IP 的另一条连接必须完全不受影响。
	for i := 0; i < 3; i++ {
		if !dp.AllowPacketFrom(quiet, ip) {
			t.Fatalf("同 IP 的另一条连接被连坐了（第 %d 个包被拒）", i+1)
		}
	}
}

// TestDDoS_PacketLimitDoesNotBlacklistIP 包频率超限**不得**拉黑 IP，
// 否则该 IP 后面所有人（含尚未建立的新连接）全被封。
func TestDDoS_PacketLimitDoesNotBlacklistIP(t *testing.T) {
	dp := NewDDoSProtection(perConnTestConfig())

	const ip = "203.0.113.8"
	noisy := ip + "#1"

	for i := 0; i < 10; i++ {
		dp.AllowPacketFrom(noisy, ip)
	}

	if dp.IsBlacklisted(ip) {
		t.Fatalf("包频率超限不应拉黑整个 IP（NAT 下会误伤大量无关玩家）")
	}
	if !dp.AllowConnection(ip) {
		t.Fatalf("该 IP 的新连接不应被拒")
	}
	if !dp.AllowPacketFrom(ip+"#2", ip) {
		t.Fatalf("该 IP 上的新连接应有自己的额度")
	}
}

// TestDDoS_TrafficLimitIsPerConnection 流量维度同理。
func TestDDoS_TrafficLimitIsPerConnection(t *testing.T) {
	dp := NewDDoSProtection(perConnTestConfig())

	const ip = "203.0.113.9"
	noisy, quiet := ip+"#1", ip+"#2"

	if !dp.AllowTrafficFrom(noisy, ip, 100) {
		t.Fatalf("额度内的流量不应被拒")
	}
	if dp.AllowTrafficFrom(noisy, ip, 100) {
		t.Fatalf("超额后该连接应被拒")
	}
	if !dp.AllowTrafficFrom(quiet, ip, 100) {
		t.Fatalf("同 IP 的另一条连接被连坐了")
	}
	if dp.IsBlacklisted(ip) {
		t.Fatalf("流量超限不应拉黑整个 IP")
	}
}

// TestDDoS_ConnectionFloodStillBansIP 连接洪水**仍**按 IP 拦并拉黑
// ——那是唯一只能按 IP 判断的维度（建连时还没有任何账号/会话身份）。
func TestDDoS_ConnectionFloodStillBansIP(t *testing.T) {
	cfg := perConnTestConfig()
	cfg.MaxConnPerIP = 2
	dp := NewDDoSProtection(cfg)

	const ip = "203.0.113.10"
	if !dp.AllowConnection(ip) || !dp.AllowConnection(ip) {
		t.Fatalf("额度内的连接不应被拒")
	}
	if dp.AllowConnection(ip) {
		t.Fatalf("超额连接应被拒")
	}
	if !dp.IsBlacklisted(ip) {
		t.Fatalf("连接洪水仍应拉黑该 IP")
	}
}

// TestDDoS_BlacklistedIPStillBlocksPackets 已因连接洪水被拉黑的 IP，其包也应被拒。
func TestDDoS_BlacklistedIPStillBlocksPackets(t *testing.T) {
	cfg := perConnTestConfig()
	cfg.MaxConnPerIP = 1
	dp := NewDDoSProtection(cfg)

	const ip = "203.0.113.11"
	dp.AllowConnection(ip)
	dp.AllowConnection(ip) // 触发拉黑

	if !dp.IsBlacklisted(ip) {
		t.Fatalf("前置条件：该 IP 应已被拉黑")
	}
	if dp.AllowPacketFrom(ip+"#1", ip) {
		t.Fatalf("已拉黑 IP 的包应被拒")
	}
}
