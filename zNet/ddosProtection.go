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

type ConnectionLimiter struct {
	records      *zMap.TypedMap[string, *ipRecord]
	maxConnPerIP int
	timeWindow   time.Duration
}

func NewConnectionLimiter(maxConnPerIP int, timeWindow time.Duration) *ConnectionLimiter {
	return &ConnectionLimiter{
		records:      zMap.NewTypedMap[string, *ipRecord](),
		maxConnPerIP: maxConnPerIP,
		timeWindow:   timeWindow,
	}
}

func (cl *ConnectionLimiter) AllowConnection(ip string) bool {
	record, _ := cl.records.LoadOrStore(ip, newIPRecord())
	return record.addAndCheck(time.Now(), cl.timeWindow, cl.maxConnPerIP)
}

type PacketLimiter struct {
	records         *zMap.TypedMap[string, *ipRecord]
	maxPacketsPerIP int
	timeWindow      time.Duration
}

func NewPacketLimiter(maxPacketsPerIP int, timeWindow time.Duration) *PacketLimiter {
	return &PacketLimiter{
		records:         zMap.NewTypedMap[string, *ipRecord](),
		maxPacketsPerIP: maxPacketsPerIP,
		timeWindow:      timeWindow,
	}
}

func (pl *PacketLimiter) AllowPacket(ip string) bool {
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
	current := traffic.bytes.Load()
	if current+bytes > tl.maxBytesPerIP {
		return false
	}

	return traffic.bytes.CompareAndSwap(current, current+bytes)
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
