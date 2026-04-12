package zNet

import (
	"sync"
	"time"
)

// ConnectionLimiter 连接频率限制器
// 限制单个IP在指定时间窗口内的连接次数
type ConnectionLimiter struct {
	ipConnections map[string][]time.Time // IP连接时间记录
	mu            sync.Mutex             // 互斥锁
	maxConnPerIP  int                    // 每个IP最大连接数
	timeWindow    time.Duration          // 时间窗口
}

// NewConnectionLimiter 创建连接限制器
// 参数:
//   - maxConnPerIP: 每个IP最大连接数
//   - timeWindow: 时间窗口
//
// 返回:
//   - *ConnectionLimiter: 连接限制器实例
func NewConnectionLimiter(maxConnPerIP int, timeWindow time.Duration) *ConnectionLimiter {
	return &ConnectionLimiter{
		ipConnections: make(map[string][]time.Time),
		maxConnPerIP:  maxConnPerIP,
		timeWindow:    timeWindow,
	}
}

// AllowConnection 检查是否允许新连接
// 参数:
//   - ip: 客户端IP地址
//
// 返回:
//   - bool: 允许连接返回true
func (cl *ConnectionLimiter) AllowConnection(ip string) bool {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	// 清理过期的连接记录
	now := time.Now()
	var validConnections []time.Time
	for _, t := range cl.ipConnections[ip] {
		if now.Sub(t) < cl.timeWindow {
			validConnections = append(validConnections, t)
		}
	}
	cl.ipConnections[ip] = validConnections

	// 检查连接数是否超过限制
	if len(validConnections) >= cl.maxConnPerIP {
		return false
	}

	// 添加新连接记录
	cl.ipConnections[ip] = append(validConnections, now)
	return true
}

// PacketLimiter 数据包频率限制器
// 限制单个IP在指定时间窗口内的数据包数量
type PacketLimiter struct {
	ipPackets       map[string][]time.Time // IP数据包时间记录
	mu              sync.Mutex             // 互斥锁
	maxPacketsPerIP int                    // 每个IP最大数据包数
	timeWindow      time.Duration          // 时间窗口
}

// NewPacketLimiter 创建数据包限制器
// 参数:
//   - maxPacketsPerIP: 每个IP最大数据包数
//   - timeWindow: 时间窗口
//
// 返回:
//   - *PacketLimiter: 数据包限制器实例
func NewPacketLimiter(maxPacketsPerIP int, timeWindow time.Duration) *PacketLimiter {
	return &PacketLimiter{
		ipPackets:       make(map[string][]time.Time),
		maxPacketsPerIP: maxPacketsPerIP,
		timeWindow:      timeWindow,
	}
}

// AllowPacket 检查是否允许新数据包
// 参数:
//   - ip: 客户端IP地址
//
// 返回:
//   - bool: 允许数据包返回true
func (pl *PacketLimiter) AllowPacket(ip string) bool {
	pl.mu.Lock()
	defer pl.mu.Unlock()

	// 清理过期的数据包记录
	now := time.Now()
	var validPackets []time.Time
	for _, t := range pl.ipPackets[ip] {
		if now.Sub(t) < pl.timeWindow {
			validPackets = append(validPackets, t)
		}
	}
	pl.ipPackets[ip] = validPackets

	// 检查数据包数是否超过限制
	if len(validPackets) >= pl.maxPacketsPerIP {
		return false
	}

	// 添加新数据包记录
	pl.ipPackets[ip] = append(validPackets, now)
	return true
}

// IPBlacklist IP黑名单
// 管理被禁止的IP地址列表
type IPBlacklist struct {
	blacklistedIPs map[string]time.Time // 被禁止的IP及其禁止时间
	mu             sync.Mutex           // 互斥锁
	banDuration    time.Duration        // 禁止持续时间
}

// NewIPBlacklist 创建IP黑名单
// 参数:
//   - banDuration: 禁止持续时间
//
// 返回:
//   - *IPBlacklist: IP黑名单实例
func NewIPBlacklist(banDuration time.Duration) *IPBlacklist {
	return &IPBlacklist{
		blacklistedIPs: make(map[string]time.Time),
		banDuration:    banDuration,
	}
}

// IsBlacklisted 检查IP是否被黑名单
// 参数:
//   - ip: 要检查的IP地址
//
// 返回:
//   - bool: 在黑名单中返回true
func (ibl *IPBlacklist) IsBlacklisted(ip string) bool {
	ibl.mu.Lock()
	defer ibl.mu.Unlock()

	banTime, ok := ibl.blacklistedIPs[ip]
	if !ok {
		return false
	}

	// 检查黑名单是否过期
	if time.Since(banTime) > ibl.banDuration {
		delete(ibl.blacklistedIPs, ip)
		return false
	}

	return true
}

// BlacklistIP 将IP加入黑名单
// 参数:
//   - ip: 要加入黑名单的IP地址
func (ibl *IPBlacklist) BlacklistIP(ip string) {
	ibl.mu.Lock()
	defer ibl.mu.Unlock()

	ibl.blacklistedIPs[ip] = time.Now()
}

// TrafficLimiter 流量限制器
// 限制单个IP在指定时间窗口内的流量大小
type TrafficLimiter struct {
	ipTraffic     map[string]int64 // IP流量统计
	mu            sync.Mutex       // 互斥锁
	maxBytesPerIP int64            // 每个IP最大流量（字节）
	timeWindow    time.Duration    // 时间窗口
	lastReset     time.Time        // 上次重置时间
}

// NewTrafficLimiter 创建流量限制器
// 参数:
//   - maxBytesPerIP: 每个IP最大流量（字节）
//   - timeWindow: 时间窗口
//
// 返回:
//   - *TrafficLimiter: 流量限制器实例
func NewTrafficLimiter(maxBytesPerIP int64, timeWindow time.Duration) *TrafficLimiter {
	return &TrafficLimiter{
		ipTraffic:     make(map[string]int64),
		maxBytesPerIP: maxBytesPerIP,
		timeWindow:    timeWindow,
		lastReset:     time.Now(),
	}
}

// AllowTraffic 检查是否允许流量
// 参数:
//   - ip: 客户端IP地址
//   - bytes: 本次流量大小（字节）
//
// 返回:
//   - bool: 允许流量返回true
func (tl *TrafficLimiter) AllowTraffic(ip string, bytes int64) bool {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	// 定期重置流量统计
	if time.Since(tl.lastReset) > tl.timeWindow {
		tl.ipTraffic = make(map[string]int64)
		tl.lastReset = time.Now()
	}

	// 检查流量是否超过限制
	if tl.ipTraffic[ip]+bytes > tl.maxBytesPerIP {
		return false
	}

	// 更新流量统计
	tl.ipTraffic[ip] += bytes
	return true
}

// DDoSProtection DDoS保护管理器
type DDoSProtection struct {
	connectionLimiter *ConnectionLimiter
	packetLimiter     *PacketLimiter
	ipBlacklist       *IPBlacklist
	trafficLimiter    *TrafficLimiter
	whitelist         map[string]bool
}

func NewDDoSProtection(cfg ...*DDoSConfig) *DDoSProtection {
	ddosCfg := DefaultDDoSConfig()
	if len(cfg) > 0 && cfg[0] != nil {
		ddosCfg = cfg[0]
	}

	whitelist := make(map[string]bool)
	for _, ip := range ddosCfg.WhitelistIPs {
		whitelist[ip] = true
	}

	return &DDoSProtection{
		connectionLimiter: NewConnectionLimiter(ddosCfg.MaxConnPerIP, time.Duration(ddosCfg.ConnTimeWindow)*time.Second),
		packetLimiter:     NewPacketLimiter(ddosCfg.MaxPacketsPerIP, time.Duration(ddosCfg.PacketTimeWindow)*time.Second),
		ipBlacklist:       NewIPBlacklist(time.Duration(ddosCfg.BanDuration) * time.Second),
		trafficLimiter:    NewTrafficLimiter(ddosCfg.MaxBytesPerIP, time.Duration(ddosCfg.TrafficTimeWindow)*time.Second),
		whitelist:         whitelist,
	}
}

func (dp *DDoSProtection) AllowConnection(ip string) bool {
	if dp.whitelist[ip] {
		return true
	}

	if dp.ipBlacklist.IsBlacklisted(ip) {
		return false
	}

	if !dp.connectionLimiter.AllowConnection(ip) {
		dp.ipBlacklist.BlacklistIP(ip)
		return false
	}

	return true
}

// AllowPacket 检查是否允许新数据包
// 参数:
//   - ip: 客户端IP地址
//
// 返回:
//   - bool: 允许数据包返回true
func (dp *DDoSProtection) AllowPacket(ip string) bool {
	// 检查IP是否被黑名单
	if dp.ipBlacklist.IsBlacklisted(ip) {
		return false
	}

	// 检查数据包频率是否超过限制
	if !dp.packetLimiter.AllowPacket(ip) {
		// 将IP加入黑名单
		dp.ipBlacklist.BlacklistIP(ip)
		return false
	}

	return true
}

// AllowTraffic 检查是否允许流量
// 参数:
//   - ip: 客户端IP地址
//   - bytes: 本次流量大小（字节）
//
// 返回:
//   - bool: 允许流量返回true
func (dp *DDoSProtection) AllowTraffic(ip string, bytes int64) bool {
	// 检查IP是否被黑名单
	if dp.ipBlacklist.IsBlacklisted(ip) {
		return false
	}

	// 检查流量是否超过限制
	if !dp.trafficLimiter.AllowTraffic(ip, bytes) {
		// 将IP加入黑名单
		dp.ipBlacklist.BlacklistIP(ip)
		return false
	}

	return true
}

// IsBlacklisted 检查IP是否被黑名单
// 参数:
//   - ip: 要检查的IP地址
//
// 返回:
//   - bool: 在黑名单中返回true
func (dp *DDoSProtection) IsBlacklisted(ip string) bool {
	return dp.ipBlacklist.IsBlacklisted(ip)
}

// BlacklistIP 将IP加入黑名单
// 参数:
//   - ip: 要加入黑名单的IP地址
func (dp *DDoSProtection) BlacklistIP(ip string) {
	dp.ipBlacklist.BlacklistIP(ip)
}
