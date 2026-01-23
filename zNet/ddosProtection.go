package zNet

import (
	"sync"
	"time"
)

// ConnectionLimiter 连接频率限制器
type ConnectionLimiter struct {
	ipConnections map[string][]time.Time
	mu            sync.Mutex
	maxConnPerIP  int
	timeWindow    time.Duration
}

// NewConnectionLimiter 创建连接限制器
func NewConnectionLimiter(maxConnPerIP int, timeWindow time.Duration) *ConnectionLimiter {
	return &ConnectionLimiter{
		ipConnections: make(map[string][]time.Time),
		maxConnPerIP:  maxConnPerIP,
		timeWindow:    timeWindow,
	}
}

// AllowConnection 检查是否允许新连接
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
type PacketLimiter struct {
	ipPackets       map[string][]time.Time
	mu              sync.Mutex
	maxPacketsPerIP int
	timeWindow      time.Duration
}

// NewPacketLimiter 创建数据包限制器
func NewPacketLimiter(maxPacketsPerIP int, timeWindow time.Duration) *PacketLimiter {
	return &PacketLimiter{
		ipPackets:       make(map[string][]time.Time),
		maxPacketsPerIP: maxPacketsPerIP,
		timeWindow:      timeWindow,
	}
}

// AllowPacket 检查是否允许新数据包
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
type IPBlacklist struct {
	blacklistedIPs map[string]time.Time
	mu             sync.Mutex
	banDuration    time.Duration
}

// NewIPBlacklist 创建IP黑名单
func NewIPBlacklist(banDuration time.Duration) *IPBlacklist {
	return &IPBlacklist{
		blacklistedIPs: make(map[string]time.Time),
		banDuration:    banDuration,
	}
}

// IsBlacklisted 检查IP是否被黑名单
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
func (ibl *IPBlacklist) BlacklistIP(ip string) {
	ibl.mu.Lock()
	defer ibl.mu.Unlock()

	ibl.blacklistedIPs[ip] = time.Now()
}

// TrafficLimiter 流量限制器
type TrafficLimiter struct {
	ipTraffic     map[string]int64
	mu            sync.Mutex
	maxBytesPerIP int64
	timeWindow    time.Duration
	lastReset     time.Time
}

// NewTrafficLimiter 创建流量限制器
func NewTrafficLimiter(maxBytesPerIP int64, timeWindow time.Duration) *TrafficLimiter {
	return &TrafficLimiter{
		ipTraffic:     make(map[string]int64),
		maxBytesPerIP: maxBytesPerIP,
		timeWindow:    timeWindow,
		lastReset:     time.Now(),
	}
}

// AllowTraffic 检查是否允许流量
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
}

// NewDDoSProtection 创建DDoS保护管理器
func NewDDoSProtection(cfg ...*DDoSConfig) *DDoSProtection {
	// 使用默认配置
	ddosCfg := DefaultDDoSConfig()
	if len(cfg) > 0 && cfg[0] != nil {
		ddosCfg = cfg[0]
	}

	return &DDoSProtection{
		connectionLimiter: NewConnectionLimiter(ddosCfg.MaxConnPerIP, time.Duration(ddosCfg.ConnTimeWindow)*time.Second),
		packetLimiter:     NewPacketLimiter(ddosCfg.MaxPacketsPerIP, time.Duration(ddosCfg.PacketTimeWindow)*time.Second),
		ipBlacklist:       NewIPBlacklist(time.Duration(ddosCfg.BanDuration) * time.Second),
		trafficLimiter:    NewTrafficLimiter(ddosCfg.MaxBytesPerIP, time.Duration(ddosCfg.TrafficTimeWindow)*time.Second),
	}
}

// AllowConnection 检查是否允许新连接
func (dp *DDoSProtection) AllowConnection(ip string) bool {
	// 检查IP是否被黑名单
	if dp.ipBlacklist.IsBlacklisted(ip) {
		return false
	}

	// 检查连接频率是否超过限制
	if !dp.connectionLimiter.AllowConnection(ip) {
		// 将IP加入黑名单
		dp.ipBlacklist.BlacklistIP(ip)
		return false
	}

	return true
}

// AllowPacket 检查是否允许新数据包
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
func (dp *DDoSProtection) IsBlacklisted(ip string) bool {
	return dp.ipBlacklist.IsBlacklisted(ip)
}

// BlacklistIP 将IP加入黑名单
func (dp *DDoSProtection) BlacklistIP(ip string) {
	dp.ipBlacklist.BlacklistIP(ip)
}
