// Package zInstance provides a shared pool for on-demand instances with idle reaping.
// Rooms, dungeons, battlegrounds, temporary cross-server maps, and layers all use the
// same capacity reservation and lifecycle mechanism.
package zInstance

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
)

var (
	ErrInvalidCapacity    = errors.New("instance capacity must satisfy 0 < soft <= hard")
	ErrNilInstanceBuilder = errors.New("instance builder is nil")
	ErrInstanceBuildPanic = errors.New("instance builder panicked")
	ErrInstanceCreating   = errors.New("instance is still being created")
	ErrInstanceReserved   = errors.New("instance has in-flight reservations")
)

// Instance is managed by Pool. Occupancy reports committed occupants; in-flight
// admission is tracked separately by Pool reservations. Close releases instance resources.
type Instance interface {
	Occupancy() int
	Close()
}

type entryState uint8

const (
	entryCreating entryState = iota + 1
	entryActive
	entryDestroying
)

type entry[K comparable, T Instance] struct {
	id         uint64
	generation uint64
	revision   uint64
	key        K
	inst       T
	state      entryState
	ready      chan struct{}
	pinned     bool
	reserved   int
	emptySince time.Time
}

type instanceSnapshot[K comparable, T Instance] struct {
	id         uint64
	generation uint64
	revision   uint64
	inst       T
	reserved   int
}

type occupancySnapshot[K comparable, T Instance] struct {
	instanceSnapshot[K, T]
	occupancy int
}

// Pool manages instances grouped by K. All internal indexes and state transitions are
// protected by mu. User callbacks are always invoked without mu held.
type Pool[K comparable, T Instance] struct {
	mu             sync.Mutex
	nextID         uint64
	nextGeneration uint64
	nextKeyVersion uint64
	byID           map[uint64]*entry[K, T]
	byKey          map[K]map[uint64]*entry[K, T]
	keyVersion     map[K]uint64
	onEvict        func(id uint64, inst T)
}

// NewPool creates a pool. onEvict runs after removal from both indexes and before Close.
func NewPool[K comparable, T Instance](onEvict func(id uint64, inst T)) *Pool[K, T] {
	return NewPoolWithBase[K, T](onEvict, 0)
}

// NewPoolWithBase is NewPool with the first allocated ID set to idBase+1.
func NewPoolWithBase[K comparable, T Instance](onEvict func(id uint64, inst T), idBase uint64) *Pool[K, T] {
	return &Pool[K, T]{
		byID:       make(map[uint64]*entry[K, T]),
		byKey:      make(map[K]map[uint64]*entry[K, T]),
		keyVersion: make(map[K]uint64),
		onEvict:    onEvict,
		nextID:     idBase,
	}
}

// AcquireChecked selects or builds an instance and reserves one admission slot.
// Affinity may fill an instance up to hardCap; normal selection uses the least occupied
// instance below softCap. Call Release after the occupant either commits or aborts entry.
func (p *Pool[K, T]) AcquireChecked(
	key K,
	affinityID uint64,
	softCap, hardCap int,
	build func(id uint64) (T, error),
) (uint64, T, error) {
	var zero T
	if softCap <= 0 || hardCap <= 0 || softCap > hardCap {
		return 0, zero, ErrInvalidCapacity
	}
	if build == nil {
		return 0, zero, ErrNilInstanceBuilder
	}

	for {
		snapshots, creating, keyVersion := p.snapshotGroup(key)
		measured := measureOccupancy(snapshots)
		candidate, limit := selectCandidate(measured, affinityID, softCap, hardCap)
		if candidate != nil {
			p.mu.Lock()
			current, ok := p.byID[candidate.id]
			if ok && current.state == entryActive &&
				current.generation == candidate.generation && current.revision == candidate.revision &&
				candidate.occupancy+current.reserved < limit {
				current.reserved++
				current.emptySince = time.Time{}
				current.revision++
				p.bumpKeyVersionLocked(key)
				id, inst := current.id, current.inst
				p.mu.Unlock()
				return id, inst, nil
			}
			p.mu.Unlock()
			continue
		}

		p.mu.Lock()
		if p.keyVersion[key] != keyVersion {
			p.mu.Unlock()
			continue
		}
		if creating != nil {
			p.mu.Unlock()
			<-creating
			continue
		}
		e := p.newEntryLocked(key, false, 1)
		p.mu.Unlock()

		inst, err := invokeBuild(build, e.id)
		if err != nil {
			p.failCreation(e)
			return 0, zero, err
		}
		p.finishCreation(e, inst)
		return e.id, inst, nil
	}
}

// Acquire preserves the original API for source compatibility.
// Deprecated: use AcquireChecked and handle build/admission errors.
func (p *Pool[K, T]) Acquire(key K, affinityID uint64, softCap, hardCap int, build func(id uint64) T) (uint64, T) {
	var checkedBuilder func(uint64) (T, error)
	if build != nil {
		checkedBuilder = func(id uint64) (T, error) { return build(id), nil }
	}
	id, inst, err := p.AcquireChecked(key, affinityID, softCap, hardCap, checkedBuilder)
	if err != nil {
		panic(err)
	}
	return id, inst
}

// AddChecked builds and registers an explicit instance. It does not reserve an admission
// slot; pinned instances are excluded from Reap and must be explicitly destroyed.
func (p *Pool[K, T]) AddChecked(key K, pinned bool, build func(id uint64) (T, error)) (uint64, T, error) {
	var zero T
	if build == nil {
		return 0, zero, ErrNilInstanceBuilder
	}
	p.mu.Lock()
	e := p.newEntryLocked(key, pinned, 0)
	p.mu.Unlock()

	inst, err := invokeBuild(build, e.id)
	if err != nil {
		p.failCreation(e)
		return 0, zero, err
	}
	p.finishCreation(e, inst)
	return e.id, inst, nil
}

// Add preserves the original API for source compatibility.
// Deprecated: use AddChecked and handle build errors.
func (p *Pool[K, T]) Add(key K, pinned bool, build func(id uint64) T) (uint64, T) {
	var checkedBuilder func(uint64) (T, error)
	if build != nil {
		checkedBuilder = func(id uint64) (T, error) { return build(id), nil }
	}
	id, inst, err := p.AddChecked(key, pinned, checkedBuilder)
	if err != nil {
		panic(err)
	}
	return id, inst
}

// Release returns one in-flight reservation. Unknown IDs and already released entries
// remain no-ops for compatibility.
func (p *Pool[K, T]) Release(id uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if e, ok := p.byID[id]; ok && e.state == entryActive && e.reserved > 0 {
		e.reserved--
		e.revision++
		p.bumpKeyVersionLocked(e.key)
	}
}

// DestroyChecked explicitly removes and closes an instance. It is idempotent for unknown
// IDs, but refuses to overtake creation or an admitted occupant that has not released its
// reservation yet.
func (p *Pool[K, T]) DestroyChecked(id uint64) error {
	p.mu.Lock()
	e, ok := p.byID[id]
	if !ok {
		p.mu.Unlock()
		return nil
	}
	if e.state == entryCreating {
		p.mu.Unlock()
		return fmt.Errorf("%w: %d", ErrInstanceCreating, id)
	}
	if e.reserved > 0 {
		p.mu.Unlock()
		return fmt.Errorf("%w: id=%d reserved=%d", ErrInstanceReserved, id, e.reserved)
	}
	e.state = entryDestroying
	p.removeLocked(e)
	p.bumpKeyVersionLocked(e.key)
	p.mu.Unlock()

	p.evict(e)
	return nil
}

// Destroy preserves the original no-result API while keeping the new safety boundary.
// Deprecated: use DestroyChecked to observe a reserved/creating rejection.
func (p *Pool[K, T]) Destroy(id uint64) {
	_ = p.DestroyChecked(id)
}

// Reap removes non-pinned instances that remain empty beyond grace. Occupancy, onEvict,
// and Close all run outside the pool lock. Entry revision validation prevents a stale
// occupancy observation from overtaking a concurrent Acquire/Release.
func (p *Pool[K, T]) Reap(grace time.Duration) int {
	now := time.Now()
	snapshots := p.snapshotReapCandidates()
	measured := measureOccupancy(snapshots)

	p.mu.Lock()
	toReap := make([]*entry[K, T], 0, len(measured))
	for _, snapshot := range measured {
		e, ok := p.byID[snapshot.id]
		if !ok || e.state != entryActive || e.generation != snapshot.generation || e.revision != snapshot.revision {
			continue
		}
		if snapshot.occupancy > 0 || e.reserved > 0 {
			if !e.emptySince.IsZero() {
				e.emptySince = time.Time{}
				e.revision++
				p.bumpKeyVersionLocked(e.key)
			}
			continue
		}
		if e.emptySince.IsZero() {
			e.emptySince = now
			e.revision++
			p.bumpKeyVersionLocked(e.key)
			continue
		}
		if now.Sub(e.emptySince) < grace {
			continue
		}
		e.state = entryDestroying
		p.removeLocked(e)
		p.bumpKeyVersionLocked(e.key)
		toReap = append(toReap, e)
	}
	p.mu.Unlock()

	for _, e := range toReap {
		p.evict(e)
	}
	return len(toReap)
}

// Get returns an active instance by ID. Creating entries are intentionally invisible.
func (p *Pool[K, T]) Get(id uint64) (T, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if e, ok := p.byID[id]; ok && e.state == entryActive {
		return e.inst, true
	}
	var zero T
	return zero, false
}

// RangeByKey visits a stable snapshot of active instances without holding the pool lock
// during fn.
func (p *Pool[K, T]) RangeByKey(key K, fn func(id uint64, inst T) bool) {
	p.mu.Lock()
	entries := make([]*entry[K, T], 0, len(p.byKey[key]))
	for _, e := range p.byKey[key] {
		if e.state == entryActive {
			entries = append(entries, e)
		}
	}
	p.mu.Unlock()
	for _, e := range entries {
		if !fn(e.id, e.inst) {
			return
		}
	}
}

// Count returns the number of active instances.
func (p *Pool[K, T]) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	count := 0
	for _, e := range p.byID {
		if e.state == entryActive {
			count++
		}
	}
	return count
}

// CountByKey returns the number of active instances for key.
func (p *Pool[K, T]) CountByKey(key K) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	count := 0
	for _, e := range p.byKey[key] {
		if e.state == entryActive {
			count++
		}
	}
	return count
}

func (p *Pool[K, T]) snapshotGroup(key K) ([]instanceSnapshot[K, T], <-chan struct{}, uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	group := p.byKey[key]
	snapshots := make([]instanceSnapshot[K, T], 0, len(group))
	var creating <-chan struct{}
	for _, e := range group {
		switch e.state {
		case entryCreating:
			if creating == nil {
				creating = e.ready
			}
		case entryActive:
			snapshots = append(snapshots, instanceSnapshot[K, T]{
				id: e.id, generation: e.generation, revision: e.revision, inst: e.inst, reserved: e.reserved,
			})
		}
	}
	return snapshots, creating, p.keyVersion[key]
}

func (p *Pool[K, T]) snapshotReapCandidates() []instanceSnapshot[K, T] {
	p.mu.Lock()
	defer p.mu.Unlock()
	snapshots := make([]instanceSnapshot[K, T], 0, len(p.byID))
	for _, e := range p.byID {
		if e.state != entryActive || e.pinned {
			continue
		}
		snapshots = append(snapshots, instanceSnapshot[K, T]{
			id: e.id, generation: e.generation, revision: e.revision, inst: e.inst, reserved: e.reserved,
		})
	}
	return snapshots
}

func measureOccupancy[K comparable, T Instance](snapshots []instanceSnapshot[K, T]) []occupancySnapshot[K, T] {
	measured := make([]occupancySnapshot[K, T], 0, len(snapshots))
	for _, snapshot := range snapshots {
		measured = append(measured, occupancySnapshot[K, T]{
			instanceSnapshot: snapshot,
			occupancy:        snapshot.inst.Occupancy(),
		})
	}
	return measured
}

func selectCandidate[K comparable, T Instance](
	snapshots []occupancySnapshot[K, T],
	affinityID uint64,
	softCap, hardCap int,
) (*occupancySnapshot[K, T], int) {
	if affinityID != 0 {
		for i := range snapshots {
			if snapshots[i].id == affinityID && snapshots[i].occupancy+snapshots[i].reserved < hardCap {
				return &snapshots[i], hardCap
			}
		}
	}
	best := -1
	bestEffective := math.MaxInt
	for i := range snapshots {
		effective := snapshots[i].occupancy + snapshots[i].reserved
		if effective < softCap && effective < bestEffective {
			best = i
			bestEffective = effective
		}
	}
	if best < 0 {
		return nil, softCap
	}
	return &snapshots[best], softCap
}

func (p *Pool[K, T]) newEntryLocked(key K, pinned bool, reserved int) *entry[K, T] {
	p.nextID++
	p.nextGeneration++
	e := &entry[K, T]{
		id:         p.nextID,
		generation: p.nextGeneration,
		key:        key,
		state:      entryCreating,
		ready:      make(chan struct{}),
		pinned:     pinned,
		reserved:   reserved,
	}
	p.byID[e.id] = e
	if p.byKey[key] == nil {
		p.byKey[key] = make(map[uint64]*entry[K, T])
	}
	p.byKey[key][e.id] = e
	p.bumpKeyVersionLocked(key)
	return e
}

func (p *Pool[K, T]) finishCreation(e *entry[K, T], inst T) {
	p.mu.Lock()
	current, ok := p.byID[e.id]
	if !ok || current != e || e.state != entryCreating {
		p.mu.Unlock()
		panic("zInstance: creation placeholder disappeared")
	}
	e.inst = inst
	e.state = entryActive
	e.revision++
	p.bumpKeyVersionLocked(e.key)
	close(e.ready)
	p.mu.Unlock()
}

func (p *Pool[K, T]) failCreation(e *entry[K, T]) {
	p.mu.Lock()
	current, ok := p.byID[e.id]
	if ok && current == e && e.state == entryCreating {
		e.state = entryDestroying
		p.removeLocked(e)
		p.bumpKeyVersionLocked(e.key)
		close(e.ready)
	}
	p.mu.Unlock()
}

func (p *Pool[K, T]) removeLocked(e *entry[K, T]) {
	delete(p.byID, e.id)
	if group := p.byKey[e.key]; group != nil {
		delete(group, e.id)
		if len(group) == 0 {
			delete(p.byKey, e.key)
		}
	}
}

func (p *Pool[K, T]) bumpKeyVersionLocked(key K) {
	p.nextKeyVersion++
	if len(p.byKey[key]) == 0 {
		delete(p.keyVersion, key)
		return
	}
	p.keyVersion[key] = p.nextKeyVersion
}

func (p *Pool[K, T]) evict(e *entry[K, T]) {
	defer e.inst.Close()
	if p.onEvict != nil {
		p.onEvict(e.id, e.inst)
	}
}

func invokeBuild[T Instance](build func(id uint64) (T, error), id uint64) (inst T, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			var zero T
			inst = zero
			err = fmt.Errorf("%w: %v", ErrInstanceBuildPanic, recovered)
		}
	}()
	return build(id)
}
