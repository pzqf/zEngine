// Package zInstance 提供「按需创建、空置自动回收的实例池」这一通用引擎能力——
// 房间 / 副本 / 战场 / 匹配对局 / 跨服临时实例 / 分线（layer）都是它的实例。
//
// 两类实例并存：
//   - Acquire：为"进入某逻辑组的占用者"选一个未满的实例，选不到则建新——用于分线/房间匹配这类
//     "填到 softCap 就开新、可空置回收"的场景。附带「在途预留额度」(reserved) 消除并发下的 TOCTOU
//     与"空置回收 vs 正在加入"竞态。
//   - Add：直接登记一个新实例（副本/跨服临时实例这类"显式建一个"的场景），可标记 pinned=永不回收。
//
// Pool 只管池化/回收；实例的创建与销毁副作用由调用方经 build 闭包与 onEvict 回调提供，因此 Pool
// 不必知道实例内部（地图/房间等）。
package zInstance

import (
	"math"
	"sync"
	"time"
)

// Instance 是被池管理的实例。Occupancy 报告当前占用数（如实例内玩家数）；Occupancy==0 且无在途
// 预留、且非 pinned 时成为空置回收候选。Close 释放实例自身资源。
type Instance interface {
	Occupancy() int
	Close()
}

type entry[K comparable, T Instance] struct {
	id         uint64
	key        K
	inst       T
	pinned     bool      // true=永不被 Reap 回收（如副本，由玩法生命周期显式销毁）
	reserved   int       // 在途分配额度：已选中/新建、占用者尚未加入完成的名额
	emptySince time.Time // 变空时刻（零值=当前非空/尚未计时）
}

// Pool 管理一组实例：K = 逻辑分组键（如逻辑地图ID/房间类型），T = 实例类型。并发安全。
// 空置回收由外部单 goroutine 周期调用 Reap 驱动。
type Pool[K comparable, T Instance] struct {
	mu      sync.Mutex
	nextID  uint64
	byID    map[uint64]*entry[K, T]
	byKey   map[K]map[uint64]*entry[K, T]
	onEvict func(id uint64, inst T) // 实例被 Reap/Destroy 摘除时回调（锁外，在 inst.Close 之前）；可为 nil
}

// NewPool 创建实例池。onEvict 在实例被回收/销毁、从池中摘除后、Close 之前回调（供调用方做联动清理，
// 如从另一张索引表删除）；不需要则传 nil。
func NewPool[K comparable, T Instance](onEvict func(id uint64, inst T)) *Pool[K, T] {
	return NewPoolWithBase[K, T](onEvict, 0)
}

// NewPoolWithBase 同 NewPool，但实例 id 从 idBase 起（首个分配的 id = idBase+1）。
// 用于让派生 id 落在与"逻辑/静态 id"不冲突的高段（如实例地图 id 从 100000 起，避开逻辑图 id）。
func NewPoolWithBase[K comparable, T Instance](onEvict func(id uint64, inst T), idBase uint64) *Pool[K, T] {
	return &Pool[K, T]{
		byID:    make(map[uint64]*entry[K, T]),
		byKey:   make(map[K]map[uint64]*entry[K, T]),
		onEvict: onEvict,
		nextID:  idBase,
	}
}

// Acquire 为进入 key 组的占用者选/建一个实例，并占 1 个在途预留额度（reserved++）。
// 选择顺序：①亲和：affinityID!=0 且该实例 effective<hardCap → 用它；②否则同组 effective 最少且
// <softCap 的实例；③都不满足 → build(id) 建新实例（reserved 从 1 起，非 pinned）。
// effective = Occupancy + reserved。选中/新建的实例同时清 emptySince。返回实例 id 与实例本身。
// ⚠ 调用方在占用者加入完成（成功或失败）后，必须 Release(id) 归还额度。
func (p *Pool[K, T]) Acquire(key K, affinityID uint64, softCap, hardCap int, build func(id uint64) T) (uint64, T) {
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

	// build 需要 id：先占 id 再 build（build 内可用 id 构造实例）。
	p.nextID++
	id := p.nextID
	inst := build(id)
	e := &entry[K, T]{id: id, key: key, inst: inst, reserved: 1}
	p.byID[id] = e
	if p.byKey[key] == nil {
		p.byKey[key] = make(map[uint64]*entry[K, T])
	}
	p.byKey[key][id] = e
	return id, inst
}

// Add 直接登记一个新实例（不走"选已有"逻辑，不占 reserved）。pinned=true 的永不被 Reap。
// build(id) 用分配到的 id 构造实例。用于副本/跨服临时实例这类"显式建一个"的场景。
func (p *Pool[K, T]) Add(key K, pinned bool, build func(id uint64) T) (uint64, T) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.nextID++
	id := p.nextID
	inst := build(id)
	e := &entry[K, T]{id: id, key: key, inst: inst, pinned: pinned}
	p.byID[id] = e
	if p.byKey[key] == nil {
		p.byKey[key] = make(map[uint64]*entry[K, T])
	}
	p.byKey[key][id] = e
	return id, inst
}

// Release 归还 1 个在途预留额度。对未知 id / reserved 已为 0 无副作用。
func (p *Pool[K, T]) Release(id uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if e, ok := p.byID[id]; ok && e.reserved > 0 {
		e.reserved--
	}
}

// Destroy 显式销毁一个实例（幂等）：摘除 → onEvict → Close。用于副本等由玩法生命周期显式回收的实例。
func (p *Pool[K, T]) Destroy(id uint64) {
	p.mu.Lock()
	e, ok := p.byID[id]
	if ok {
		p.remove(e)
	}
	p.mu.Unlock()
	if ok {
		if p.onEvict != nil {
			p.onEvict(e.id, e.inst)
		}
		e.inst.Close()
	}
}

// remove 在持锁下把 entry 从两张索引摘除。
func (p *Pool[K, T]) remove(e *entry[K, T]) {
	delete(p.byID, e.id)
	if g := p.byKey[e.key]; g != nil {
		delete(g, e.id)
		if len(g) == 0 {
			delete(p.byKey, e.key)
		}
	}
}

// Reap 回收「非 pinned、空置持续超过 grace（Occupancy==0 且 reserved==0）」的实例。返回回收数。
// ⚠ 必须由固定的单个 goroutine 周期调用（onEvict/Close 释放资源，与其它调用串行才安全）。
func (p *Pool[K, T]) Reap(grace time.Duration) int {
	now := time.Now()
	p.mu.Lock()
	var toReap []*entry[K, T]
	for _, e := range p.byID {
		if e.pinned {
			continue
		}
		if e.inst.Occupancy() > 0 || e.reserved > 0 {
			e.emptySince = time.Time{}
			continue
		}
		if e.emptySince.IsZero() {
			e.emptySince = now
			continue
		}
		if now.Sub(e.emptySince) >= grace {
			toReap = append(toReap, e)
		}
	}
	for _, e := range toReap {
		p.remove(e)
	}
	p.mu.Unlock()

	for _, e := range toReap {
		if p.onEvict != nil {
			p.onEvict(e.id, e.inst)
		}
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

// RangeByKey 遍历某逻辑键下的所有存活实例（fn 返回 false 停止）。
func (p *Pool[K, T]) RangeByKey(key K, fn func(id uint64, inst T) bool) {
	p.mu.Lock()
	entries := make([]*entry[K, T], 0, len(p.byKey[key]))
	for _, e := range p.byKey[key] {
		entries = append(entries, e)
	}
	p.mu.Unlock()
	for _, e := range entries {
		if !fn(e.id, e.inst) {
			return
		}
	}
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
