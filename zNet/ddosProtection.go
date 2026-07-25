package zNet

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/pzqf/zUtil/zMap"
)

type ipRecord struct {
	times []time.Time
	mu    sync.Mutex
}

func newIPRecord() *ipRecord {
	return &ipRecord{
		times: make([]time.Time, 0, 8),
	}
}

func (r *ipRecord) addAndCheck(now time.Time, window time.Duration, maxCount int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	valid := r.times[:0]
	for _, t := range r.times {
		if now.Sub(t) < window {
			valid = append(valid, t)
		}
	}

	if len(valid) >= maxCount {
		r.times = valid
		return false
	}

	r.times = append(valid, now)
	return true
}

// cleanupIPRecords 移除 window 内已无有效记录的 IP 条目（INF-11）。此前 records 每见一个 IP 就
// LoadOrStore 一个 *ipRecord 且从不删除→随独立 IP 数无界增长（ipRecord.times 会自裁剪但 map 条目常驻）。
// best-effort：与并发 Allow 存在极小竞争（可能误删刚被重新填充的条目使该 IP 少量放宽），对空闲 IP 无害。
func cleanupIPRecords(records *zMap.TypedMap[string, *ipRecord], now time.Time, window time.Duration) {
	records.Range(func(ip string, r *ipRecord) bool {
		r.mu.Lock()
		empty := true
		for _, t := range r.times {
			if now.Sub(t) < window {
				empty = false
				break
			}
		}
		r.mu.Unlock()
		if empty {
			records.Delete(ip)
		}
		return true
	})
}

// maybeCleanupIPRecords 每过一个 window 至多清理一次（CAS 防并发重复清理），无需独立 goroutine/生命周期。
func maybeCleanupIPRecords(records *zMap.TypedMap[string, *ipRecord], lastCleanup *atomic.Int64, window time.Duration) {
	now := time.Now()
	last := lastCleanup.Load()
	if now.UnixNano()-last > window.Nanoseconds() {
		if lastCleanup.CompareAndSwap(last, now.UnixNano()) {
			cleanupIPRecords(records, now, window)
		}
	}
}

type ConnectionLimiter struct {
	records      *zMap.TypedMap[string, *ipRecord]
	maxConnPerIP int
	timeWindow   time.Duration
	lastCleanup  atomic.Int64
}

func NewConnectionLimiter(maxConnPerIP int, timeWindow time.Duration) *ConnectionLimiter {
	return &ConnectionLimiter{
		records:      zMap.NewTypedMap[string, *ipRecord](),
		maxConnPerIP: maxConnPerIP,
		timeWindow:   timeWindow,
	}
}

func (cl *ConnectionLimiter) AllowConnection(ip string) bool {
	maybeCleanupIPRecords(cl.records, &cl.lastCleanup, cl.timeWindow) // INF-11
	record, _ := cl.records.LoadOrStore(ip, newIPRecord())
	return record.addAndCheck(time.Now(), cl.timeWindow, cl.maxConnPerIP)
}

type PacketLimiter struct {
	records         *zMap.TypedMap[string, *ipRecord]
	maxPacketsPerIP int
	timeWindow      time.Duration
	lastCleanup     atomic.Int64
}

func NewPacketLimiter(maxPacketsPerIP int, timeWindow time.Duration) *PacketLimiter {
	return &PacketLimiter{
		records:         zMap.NewTypedMap[string, *ipRecord](),
		maxPacketsPerIP: maxPacketsPerIP,
		timeWindow:      timeWindow,
	}
}

func (pl *PacketLimiter) AllowPacket(ip string) bool {
	maybeCleanupIPRecords(pl.records, &pl.lastCleanup, pl.timeWindow) // INF-11
	record, _ := pl.records.LoadOrStore(ip, newIPRecord())
	return record.addAndCheck(time.Now(), pl.timeWindow, pl.maxPacketsPerIP)
}

type IPBlacklist struct {
	blacklistedIPs *zMap.TypedMap[string, time.Time]
	banDuration    time.Duration
}

func NewIPBlacklist(banDuration time.Duration) *IPBlacklist {
	return &IPBlacklist{
		blacklistedIPs: zMap.NewTypedMap[string, time.Time](),
		banDuration:    banDuration,
	}
}

func (ibl *IPBlacklist) IsBlacklisted(ip string) bool {
	banTime, ok := ibl.blacklistedIPs.Load(ip)
	if !ok {
		return false
	}

	if time.Since(banTime) > ibl.banDuration {
		ibl.blacklistedIPs.Delete(ip)
		return false
	}

	return true
}

func (ibl *IPBlacklist) BlacklistIP(ip string) {
	ibl.blacklistedIPs.Store(ip, time.Now())
}

type atomicTraffic struct {
	bytes atomic.Int64
}

type TrafficLimiter struct {
	ipTraffic     *zMap.TypedMap[string, *atomicTraffic]
	maxBytesPerIP int64
	timeWindow    time.Duration
	lastReset     atomic.Int64
}

func NewTrafficLimiter(maxBytesPerIP int64, timeWindow time.Duration) *TrafficLimiter {
	tl := &TrafficLimiter{
		ipTraffic:     zMap.NewTypedMap[string, *atomicTraffic](),
		maxBytesPerIP: maxBytesPerIP,
		timeWindow:    timeWindow,
	}
	tl.lastReset.Store(time.Now().UnixNano())
	return tl
}

func (tl *TrafficLimiter) AllowTraffic(ip string, bytes int64) bool {
	now := time.Now()
	lastResetNano := tl.lastReset.Load()
	lastResetTime := time.Unix(0, lastResetNano)

	if now.Sub(lastResetTime) > tl.timeWindow {
		if tl.lastReset.CompareAndSwap(lastResetNano, now.UnixNano()) {
			tl.ipTraffic.Range(func(key string, _ *atomicTraffic) bool {
				tl.ipTraffic.Delete(key)
				return true
			})
		}
	}

	traffic, _ := tl.ipTraffic.LoadOrStore(ip, &atomicTraffic{})
	// 循环 CAS：CAS 失败意味着有并发更新（同 IP 的其他会话在同时上报流量），应重试，
	// 而非把 CAS 失败当成"流量超限"返回 false——后者是 bug，会在同 IP 并发（NAT/多连接）
	// 下随机误判超限并断连（server_test.go 实测：8 个同 IP 会话并发时随机丢消息+断连）。
	for {
		current := traffic.bytes.Load()
		if current+bytes > tl.maxBytesPerIP {
			return false
		}
		if traffic.bytes.CompareAndSwap(current, current+bytes) {
			return true
		}
	}
}

type DDoSProtection struct {
	connectionLimiter *ConnectionLimiter
	packetLimiter     *PacketLimiter
	ipBlacklist       *IPBlacklist
	trafficLimiter    *TrafficLimiter
	whitelist         *zMap.TypedMap[string, struct{}]
}

func NewDDoSProtection(cfg ...*DDoSConfig) *DDoSProtection {
	ddosCfg := DefaultDDoSConfig()
	if len(cfg) > 0 && cfg[0] != nil {
		ddosCfg = cfg[0]
	}

	whitelist := zMap.NewTypedMap[string, struct{}]()
	for _, ip := range ddosCfg.WhitelistIPs {
		whitelist.Store(ip, struct{}{})
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
	if _, ok := dp.whitelist.Load(ip); ok {
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

func (dp *DDoSProtection) AllowPacket(ip string) bool {
	if dp.ipBlacklist.IsBlacklisted(ip) {
		return false
	}

	if !dp.packetLimiter.AllowPacket(ip) {
		dp.ipBlacklist.BlacklistIP(ip)
		return false
	}

	return true
}

func (dp *DDoSProtection) AllowTraffic(ip string, bytes int64) bool {
	if dp.ipBlacklist.IsBlacklisted(ip) {
		return false
	}

	if !dp.trafficLimiter.AllowTraffic(ip, bytes) {
		dp.ipBlacklist.BlacklistIP(ip)
		return false
	}

	return true
}

func (dp *DDoSProtection) IsBlacklisted(ip string) bool {
	return dp.ipBlacklist.IsBlacklisted(ip)
}

func (dp *DDoSProtection) BlacklistIP(ip string) {
	dp.ipBlacklist.BlacklistIP(ip)
}
