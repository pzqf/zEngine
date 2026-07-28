// Package zInstance 提供「按需创建、空置自动回收的实例池」这一通用引擎能力——
// 房间 / 副本 / 战场 / 匹配对局 / 跨服临时实例 / 分线（layer）都是它的实例。
//
// 核心价值是封装了一个通用且极易写错的并发点：「在途预留额度」（reserved）。
// 没有它时，"读占用数 → 选中实例 → 加入" 之间存在 TOCTOU：多个并发进入看到同一份滞后
// 占用数、一起选中同一实例（击穿容量上限），空置回收又会与"正在加入"竞态（把占用者加进
// 已销毁的实例）。Pool 在同一把锁内原子完成"读占用+预留"，并把 reserved>0 视为非空、
// 不予回收，从根上消除这两个竞态。
package zInstance

import (
	"math"
	"sync"
	"time"
)

// Instance 是被池管理的实例。Occupancy 报告当前占用数（如实例内玩家数）；
// Occupancy==0 且无在途预留时该实例成为空置回收候选。Close 释放实例资源（幂等由实现保证）。
type Instance interface {
	Occupancy() int
	Close()
}

type entry[K comparable, T Instance] struct {
	id         uint64
	key        K
	inst       T
	reserved   int       // 在途预留额度：已选中/新建、占用者尚未加入完成的名额
	emptySince time.Time // 变空时刻（零值=当前非空/尚未计时）
}

// Pool 管理一组实例：K = 逻辑分组键（如逻辑地图ID/房间类型），T = 实例类型。
// 并发安全。空置回收由外部单 goroutine 周期调用 Reap 驱动。
type Pool[K comparable, T Instance] struct {
	mu      sync.Mutex
	nextID  uint64
	byID    map[uint64]*entry[K, T]
	byKey   map[K]map[uint64]*entry[K, T]
	factory func(key K) T
}

// NewPool 创建实例池。factory 按逻辑键创建一个新实例（Pool 只负责池化与回收，不关心实例内部）。
func NewPool[K comparable, T Instance](factory func(key K) T) *Pool[K, T] {
	return &Pool[K, T]{
		byID:    make(map[uint64]*entry[K, T]),
		byKey:   make(map[K]map[uint64]*entry[K, T]),
		factory: factory,
	}
}

// Acquire 为进入 key 组的一个占用者选/建一个实例，并占 1 个在途预留额度（reserved++）。
// 选择顺序：
//  1. 亲和：affinityID!=0 且该实例 effective < hardCap → 用它（同队/组队同实例）。
//  2. 否则同组内 effective 最少且 < softCap 的实例。
//  3. 都不满足 → 新建实例（reserved 从 1 起）。
//
// effective = Occupancy + reserved（把"在途"名额计入容量判定，防并发击穿 cap）。选中/新建的
// 实例同时清 emptySince（防被 Reap 回收）。返回实例 id 与实例本身。
//
// ⚠ 调用方在占用者真正加入完成（成功或失败）后，必须调用 Release(id) 归还额度，否则该实例的
// reserved 永不归零：容量被永久占用、且永不被 Reap 回收。
func (p *Pool[K, T]) Acquire(key K, affinityID uint64, softCap, hardCap int) (uint64, T) {
	p.mu.Lock()
	defer p.mu.Unlock()

	group := p.byKey[key]

	if affinityID != 0 && group != nil {
		if e, ok := group[affinityID]; ok {
			if e.inst.Occupancy()+e.reserved < hardCap {
				e.reserved++
				e.emptySince = time.Time{}
				return e.id, e.inst
			}
		}
	}

	var best *entry[K, T]
	bestEff := math.MaxInt
	for _, e := range group {
		eff := e.inst.Occupancy() + e.reserved
		if eff < softCap && eff < bestEff {
			best = e
			bestEff = eff
		}
	}
	if best != nil {
		best.reserved++
		best.emptySince = time.Time{}
		return best.id, best.inst
	}

	p.nextID++
	id := p.nextID
	e := &entry[K, T]{id: id, key: key, inst: p.factory(key), reserved: 1}
	p.byID[id] = e
	if p.byKey[key] == nil {
		p.byKey[key] = make(map[uint64]*entry[K, T])
	}
	p.byKey[key][id] = e
	return id, e.inst
}

// Release 归还 1 个在途预留额度（占用者已加入完成或加入失败后调用）。对未知 id / reserved 已为 0 无副作用。
func (p *Pool[K, T]) Release(id uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if e, ok := p.byID[id]; ok && e.reserved > 0 {
		e.reserved--
	}
}

// Reap 回收「空置持续超过 grace」的实例（Occupancy==0 且 reserved==0）。返回回收的实例数。
// ⚠ 必须由固定的单个 goroutine 周期调用（Close 会释放实例资源，与其它调用串行才安全）。
func (p *Pool[K, T]) Reap(grace time.Duration) int {
	now := time.Now()
	p.mu.Lock()
	var toReap []*entry[K, T]
	for _, e := range p.byID {
		if e.inst.Occupancy() > 0 || e.reserved > 0 {
			e.emptySince = time.Time{} // 非空（含在途预留），清计时
			continue
		}
		if e.emptySince.IsZero() {
			e.emptySince = now // 刚变空，开始计时
			continue
		}
		if now.Sub(e.emptySince) >= grace {
			toReap = append(toReap, e)
		}
	}
	for _, e := range toReap {
		delete(p.byID, e.id)
		if g := p.byKey[e.key]; g != nil {
			delete(g, e.id)
			if len(g) == 0 {
				delete(p.byKey, e.key)
			}
		}
	}
	p.mu.Unlock()

	// Close 在锁外，避免实例的 Close 回调重入 Pool 造成死锁。
	for _, e := range toReap {
		e.inst.Close()
	}
	return len(toReap)
}

// Get 按 id 取实例。
func (p *Pool[K, T]) Get(id uint64) (T, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if e, ok := p.byID[id]; ok {
		return e.inst, true
	}
	var zero T
	return zero, false
}

// Count 返回当前存活实例总数。
func (p *Pool[K, T]) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.byID)
}

// CountByKey 返回某逻辑键下的存活实例数。
func (p *Pool[K, T]) CountByKey(key K) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.byKey[key])
}
