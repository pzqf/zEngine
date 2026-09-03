package zInstance

import (
	"sync"
	"sync/atomic"
	"testing"
)

// fakeInst 测试用实例：占用数与关闭标志均原子，供并发用例安全读写。
type fakeInst struct {
	occ    atomic.Int64
	closed atomic.Bool
}

func (f *fakeInst) Occupancy() int { return int(f.occ.Load()) }
func (f *fakeInst) Close()         { f.closed.Store(true) }

func newFake(uint64) *fakeInst { return &fakeInst{} }

func newFakeChecked(id uint64) (*fakeInst, error) { return newFake(id), nil }

func newTestPool() *Pool[int, *fakeInst] {
	return NewPool[int, *fakeInst](nil)
}

// enter 模拟"一个占用者进入 key 组"：选/建实例 → 加入(occ++) → 归还预留。返回选中实例 id。
func enter(p *Pool[int, *fakeInst], key int, affinity uint64, soft, hard int) (uint64, error) {
	id, inst, err := p.AcquireChecked(key, affinity, soft, hard, newFakeChecked)
	if err != nil {
		return 0, err
	}
	inst.occ.Add(1)
	p.Release(id)
	return id, nil
}

func mustEnter(t *testing.T, p *Pool[int, *fakeInst], key int, affinity uint64, soft, hard int) uint64 {
	t.Helper()
	id, err := enter(p, key, affinity, soft, hard)
	if err != nil {
		t.Fatalf("AcquireChecked: %v", err)
	}
	return id
}

// TestPool_FillThenNewInstance 填满 softCap 才开新实例。
func TestPool_FillThenNewInstance(t *testing.T) {
	p := newTestPool()
	id1 := mustEnter(t, p, 1001, 0, 2, 3)
	id2 := mustEnter(t, p, 1001, 0, 2, 3)
	if id1 != id2 {
		t.Fatalf("前 2 人应同实例: %d vs %d", id1, id2)
	}
	if p.Count() != 1 {
		t.Fatalf("此时应只有 1 个实例, got %d", p.Count())
	}
	id3 := mustEnter(t, p, 1001, 0, 2, 3)
	if id3 == id1 {
		t.Fatalf("第 3 人应进新实例")
	}
	if p.Count() != 2 {
		t.Fatalf("应有 2 个实例, got %d", p.Count())
	}
}

// TestPool_Affinity 亲和：未到 hardCap 时优先进指定实例（可超 softCap）。
func TestPool_Affinity(t *testing.T) {
	p := newTestPool()
	id1 := mustEnter(t, p, 1002, 0, 1, 3)
	id2 := mustEnter(t, p, 1002, id1, 1, 3)
	if id2 != id1 {
		t.Fatalf("亲和应进同实例")
	}
	id3 := mustEnter(t, p, 1002, id1, 1, 3)
	if id3 != id1 {
		t.Fatalf("亲和(未到 hardCap)应进同实例")
	}
	if p.CountByKey(1002) != 1 {
		t.Fatalf("亲和挤同实例, 应仍 1 个, got %d", p.CountByKey(1002))
	}
	id4 := mustEnter(t, p, 1002, id1, 1, 3)
	if id4 == id1 {
		t.Fatalf("实例1 已满 hardCap, 第 4 人应进新实例")
	}
	if p.CountByKey(1002) != 2 {
		t.Fatalf("应有 2 个实例, got %d", p.CountByKey(1002))
	}
}

// TestPool_ReservedBlocksReap 在途预留(reserved>0)的实例不被 Reap 回收。
func TestPool_ReservedBlocksReap(t *testing.T) {
	p := newTestPool()
	id, inst, err := p.AcquireChecked(1003, 0, 5, 8, newFakeChecked) // reserved=1, occ=0
	if err != nil {
		t.Fatalf("AcquireChecked: %v", err)
	}
	p.Reap(0)
	p.Reap(0)
	if _, ok := p.Get(id); !ok {
		t.Fatalf("在途预留的实例不应被回收")
	}
	if inst.closed.Load() {
		t.Fatalf("预留中的实例不应被 Close")
	}
	inst.occ.Add(1)
	p.Release(id)
	inst.occ.Add(-1)
	p.Reap(0)
	p.Reap(0)
	if _, ok := p.Get(id); ok {
		t.Fatalf("归还预留且空置后应被回收")
	}
	if !inst.closed.Load() {
		t.Fatalf("回收时应调用实例 Close")
	}
}

// TestPool_AddPinnedNotReaped AddChecked 的 pinned 实例永不被 Reap，只能显式 DestroyChecked。
func TestPool_AddPinnedNotReaped(t *testing.T) {
	p := newTestPool()
	id, inst, err := p.AddChecked(1004, true, newFakeChecked) // pinned, occ=0
	if err != nil {
		t.Fatalf("AddChecked: %v", err)
	}
	p.Reap(0)
	p.Reap(0)
	if _, ok := p.Get(id); !ok {
		t.Fatalf("pinned 实例不应被 Reap 回收")
	}
	if inst.closed.Load() {
		t.Fatalf("pinned 实例不应被 Close")
	}
	if err := p.DestroyChecked(id); err != nil {
		t.Fatalf("DestroyChecked: %v", err)
	}
	if _, ok := p.Get(id); ok {
		t.Fatalf("Destroy 后应摘除")
	}
	if !inst.closed.Load() {
		t.Fatalf("Destroy 应调用 Close")
	}
}

// TestPool_DestroyCallsOnEvict DestroyChecked/Reap 摘除时回调 onEvict（供联动清理）。
func TestPool_DestroyCallsOnEvict(t *testing.T) {
	var mu sync.Mutex
	var evicted []uint64
	p := NewPool[int, *fakeInst](func(id uint64, _ *fakeInst) {
		mu.Lock()
		evicted = append(evicted, id)
		mu.Unlock()
	})
	id, inst, err := p.AddChecked(1, false, newFakeChecked)
	if err != nil {
		t.Fatalf("AddChecked: %v", err)
	}
	if err := p.DestroyChecked(id); err != nil {
		t.Fatalf("DestroyChecked: %v", err)
	}
	if len(evicted) != 1 || evicted[0] != id {
		t.Fatalf("onEvict 应被调用一次且携带正确 id, got %v", evicted)
	}
	if !inst.closed.Load() {
		t.Fatalf("Destroy 应 Close")
	}
}

// TestPool_ConcurrentAcquire_NoCapBreach 并发进入不击穿 softCap/hardCap。
func TestPool_ConcurrentAcquire_NoCapBreach(t *testing.T) {
	p := newTestPool()
	const n, cap = 30, 3
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := enter(p, 1010, 0, cap, cap); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("AcquireChecked: %v", err)
	}

	total := 0
	p.mu.Lock()
	insts := make([]*fakeInst, 0, len(p.byID))
	for _, e := range p.byID {
		insts = append(insts, e.inst)
	}
	p.mu.Unlock()
	for _, inst := range insts {
		o := inst.Occupancy()
		if o > cap {
			t.Fatalf("实例超 cap(%d): %d 人（容量被击穿）", cap, o)
		}
		total += o
	}
	if total != n {
		t.Fatalf("总占用应=%d, got %d", n, total)
	}
	if p.Count() < n/cap {
		t.Fatalf("实例数应 >= %d, got %d", n/cap, p.Count())
	}
}
