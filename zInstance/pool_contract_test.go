package zInstance

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type contractInst struct {
	occ           atomic.Int64
	closeCalls    atomic.Int64
	occupancyHook func() int
	closeHook     func()
}

func (i *contractInst) Occupancy() int {
	if i.occupancyHook != nil {
		return i.occupancyHook()
	}
	return int(i.occ.Load())
}

func (i *contractInst) Close() {
	i.closeCalls.Add(1)
	if i.closeHook != nil {
		i.closeHook()
	}
}

func TestPoolAcquireCheckedBuildRunsOutsideLock(t *testing.T) {
	p := NewPool[int, *contractInst](nil)
	started := make(chan struct{})
	unblock := make(chan struct{})
	result := make(chan error, 1)

	go func() {
		id, _, err := p.AcquireChecked(1, 0, 1, 1, func(uint64) (*contractInst, error) {
			close(started)
			<-unblock
			return &contractInst{}, nil
		})
		if err == nil {
			p.Release(id)
		}
		result <- err
	}()
	<-started

	assertReturnsPromptly(t, func() { _ = p.Count() }, "Count blocked behind build callback")
	close(unblock)
	if err := <-result; err != nil {
		t.Fatalf("AcquireChecked: %v", err)
	}
}

func TestPoolAddCheckedBuildRunsOutsideLock(t *testing.T) {
	p := NewPool[int, *contractInst](nil)
	started := make(chan struct{})
	unblock := make(chan struct{})
	result := make(chan error, 1)

	go func() {
		_, _, err := p.AddChecked(1, false, func(uint64) (*contractInst, error) {
			close(started)
			<-unblock
			return &contractInst{}, nil
		})
		result <- err
	}()
	<-started

	assertReturnsPromptly(t, func() { _ = p.Count() }, "Count blocked behind Add build callback")
	close(unblock)
	if err := <-result; err != nil {
		t.Fatalf("AddChecked: %v", err)
	}
}

func TestPoolAcquireCheckedBuildFailureRollsBack(t *testing.T) {
	p := NewPool[int, *contractInst](nil)
	buildErr := errors.New("build failed")
	id, inst, err := p.AcquireChecked(1, 0, 1, 1, func(uint64) (*contractInst, error) {
		return nil, buildErr
	})
	if !errors.Is(err, buildErr) {
		t.Fatalf("build error = %v, want %v", err, buildErr)
	}
	if id != 0 || inst != nil {
		t.Fatalf("failed build returned id=%d inst=%v", id, inst)
	}
	if got := p.Count(); got != 0 {
		t.Fatalf("failed build left %d instances", got)
	}
	if got := len(p.keyVersion); got != 0 {
		t.Fatalf("failed build left %d key versions", got)
	}
}

func TestPoolAddCheckedBuildFailureRollsBack(t *testing.T) {
	p := NewPool[int, *contractInst](nil)
	buildErr := errors.New("build failed")
	id, inst, err := p.AddChecked(1, false, func(uint64) (*contractInst, error) {
		return nil, buildErr
	})
	if !errors.Is(err, buildErr) {
		t.Fatalf("build error = %v, want %v", err, buildErr)
	}
	if id != 0 || inst != nil {
		t.Fatalf("failed build returned id=%d inst=%v", id, inst)
	}
	if got := p.Count(); got != 0 {
		t.Fatalf("failed build left %d instances", got)
	}
	if got := len(p.keyVersion); got != 0 {
		t.Fatalf("failed build left %d key versions", got)
	}
}

func TestPoolAddReservedCheckedBlocksDestroyUntilRelease(t *testing.T) {
	p := NewPool[int, *contractInst](nil)
	id, inst, err := p.AddReservedChecked(1, true, func(uint64) (*contractInst, error) {
		return &contractInst{}, nil
	})
	if err != nil {
		t.Fatalf("AddReservedChecked: %v", err)
	}

	if err := p.DestroyChecked(id); !errors.Is(err, ErrInstanceReserved) {
		t.Fatalf("DestroyChecked error = %v, want %v", err, ErrInstanceReserved)
	}
	if inst.closeCalls.Load() != 0 {
		t.Fatalf("reserved instance closed %d times", inst.closeCalls.Load())
	}

	p.Release(id)
	p.Reap(0)
	p.Reap(0)
	if _, ok := p.Get(id); !ok {
		t.Fatal("released pinned instance was reaped")
	}
	if err := p.DestroyChecked(id); err != nil {
		t.Fatalf("DestroyChecked after Release: %v", err)
	}
	if inst.closeCalls.Load() != 1 {
		t.Fatalf("close calls = %d, want 1", inst.closeCalls.Load())
	}
}

func TestPoolAddReservedCheckedBuildFailureRollsBack(t *testing.T) {
	p := NewPool[int, *contractInst](nil)
	wantErr := errors.New("reserved build failed")
	id, inst, err := p.AddReservedChecked(1, true, func(uint64) (*contractInst, error) {
		return nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("build error = %v, want %v", err, wantErr)
	}
	if id != 0 || inst != nil {
		t.Fatalf("failed build returned id=%d inst=%v", id, inst)
	}
	if p.Count() != 0 || p.CountByKey(1) != 0 {
		t.Fatalf("failed reserved build leaked pool entry: count=%d byKey=%d", p.Count(), p.CountByKey(1))
	}
}

func TestPoolAcquireCheckedBuildPanicRollsBack(t *testing.T) {
	p := NewPool[int, *contractInst](nil)
	_, _, err := p.AcquireChecked(1, 0, 1, 1, func(uint64) (*contractInst, error) {
		panic("boom")
	})
	if !errors.Is(err, ErrInstanceBuildPanic) {
		t.Fatalf("panic error = %v, want %v", err, ErrInstanceBuildPanic)
	}
	if got := p.Count(); got != 0 {
		t.Fatalf("panicking build left %d instances", got)
	}
}

func TestPoolOccupancyRunsOutsideLockAndMayReenter(t *testing.T) {
	p := NewPool[int, *contractInst](nil)
	_, inst, err := p.AddChecked(1, false, func(uint64) (*contractInst, error) {
		return &contractInst{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	inst.occupancyHook = func() int {
		_ = p.CountByKey(1)
		return 0
	}

	assertReturnsPromptly(t, func() {
		id, _, acquireErr := p.AcquireChecked(1, 0, 2, 2, func(uint64) (*contractInst, error) {
			t.Fatal("existing instance should have been selected")
			return nil, nil
		})
		if acquireErr != nil {
			t.Errorf("AcquireChecked: %v", acquireErr)
			return
		}
		p.Release(id)
	}, "AcquireChecked deadlocked in reentrant Occupancy")

	assertReturnsPromptly(t, func() { p.Reap(time.Hour) }, "Reap deadlocked in reentrant Occupancy")
}

func TestPoolDestroyCheckedRejectsReservedInstance(t *testing.T) {
	p := NewPool[int, *contractInst](nil)
	id, inst, err := p.AcquireChecked(1, 0, 2, 2, func(uint64) (*contractInst, error) {
		return &contractInst{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.DestroyChecked(id); !errors.Is(err, ErrInstanceReserved) {
		t.Fatalf("DestroyChecked error = %v, want %v", err, ErrInstanceReserved)
	}
	if _, ok := p.Get(id); !ok {
		t.Fatal("reserved instance was removed")
	}
	if got := inst.closeCalls.Load(); got != 0 {
		t.Fatalf("reserved instance closed %d times", got)
	}

	p.Release(id)
	if err := p.DestroyChecked(id); err != nil {
		t.Fatalf("DestroyChecked after Release: %v", err)
	}
	if err := p.DestroyChecked(id); err != nil {
		t.Fatalf("idempotent DestroyChecked: %v", err)
	}
	if got := inst.closeCalls.Load(); got != 1 {
		t.Fatalf("Close calls = %d, want 1", got)
	}
	if got := len(p.keyVersion); got != 0 {
		t.Fatalf("destroyed last instance left %d key versions", got)
	}
}

func TestPoolDestroyCheckedRejectsCreatingInstance(t *testing.T) {
	p := NewPool[int, *contractInst](nil)
	builderStarted := make(chan uint64, 1)
	unblockBuilder := make(chan struct{})
	created := make(chan *contractInst, 1)

	go func() {
		_, inst, err := p.AddChecked(1, false, func(id uint64) (*contractInst, error) {
			builderStarted <- id
			<-unblockBuilder
			return &contractInst{}, nil
		})
		if err != nil {
			created <- nil
			return
		}
		created <- inst
	}()

	id := <-builderStarted
	if err := p.DestroyChecked(id); !errors.Is(err, ErrInstanceCreating) {
		t.Fatalf("DestroyChecked error=%v, want ErrInstanceCreating", err)
	}
	close(unblockBuilder)
	inst := <-created
	if inst == nil {
		t.Fatal("AddChecked failed after creating rejection")
	}
	if err := p.DestroyChecked(id); err != nil {
		t.Fatalf("DestroyChecked after creation: %v", err)
	}
	if got := inst.closeCalls.Load(); got != 1 {
		t.Fatalf("Close calls=%d, want 1", got)
	}
}

func TestPoolEvictionCallbacksRunOutsideLock(t *testing.T) {
	var p *Pool[int, *contractInst]
	var evicted atomic.Int64
	p = NewPool[int, *contractInst](func(uint64, *contractInst) {
		_ = p.Count()
		evicted.Add(1)
	})
	_, inst, err := p.AddChecked(1, false, func(uint64) (*contractInst, error) {
		return &contractInst{closeHook: func() { _ = p.Count() }}, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	assertReturnsPromptly(t, func() {
		if destroyErr := p.DestroyChecked(1); destroyErr != nil {
			t.Errorf("DestroyChecked: %v", destroyErr)
		}
	}, "DestroyChecked deadlocked in eviction callback")
	if got := evicted.Load(); got != 1 {
		t.Fatalf("onEvict calls = %d, want 1", got)
	}
	if got := inst.closeCalls.Load(); got != 1 {
		t.Fatalf("Close calls = %d, want 1", got)
	}
}

func TestPoolConcurrentAcquireDoesNotDuplicateBuildWithinCapacity(t *testing.T) {
	p := NewPool[int, *contractInst](nil)
	const n = 8
	var buildCalls atomic.Int64
	buildStarted := make(chan struct{})
	unblockBuild := make(chan struct{})
	releaseReservations := make(chan struct{})
	type acquireResult struct {
		id  uint64
		err error
	}
	results := make(chan acquireResult, n)
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, _, err := p.AcquireChecked(1, 0, n, n, func(uint64) (*contractInst, error) {
				if buildCalls.Add(1) == 1 {
					close(buildStarted)
					<-unblockBuild
				}
				return &contractInst{}, nil
			})
			results <- acquireResult{id: id, err: err}
			if err == nil {
				<-releaseReservations
				p.Release(id)
			}
		}()
	}
	<-buildStarted
	close(unblockBuild)

	var firstID uint64
	for range n {
		result := <-results
		if result.err != nil {
			t.Fatalf("AcquireChecked: %v", result.err)
		}
		if firstID == 0 {
			firstID = result.id
		} else if result.id != firstID {
			t.Fatalf("concurrent acquire returned id=%d, want %d", result.id, firstID)
		}
	}
	if got := buildCalls.Load(); got != 1 {
		t.Fatalf("build calls = %d, want 1", got)
	}
	close(releaseReservations)
	wg.Wait()
}

func TestPoolReapDoesNotOvertakeConcurrentAcquire(t *testing.T) {
	p := NewPool[int, *contractInst](nil)
	_, inst, err := p.AddChecked(1, false, func(uint64) (*contractInst, error) {
		return &contractInst{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	p.Reap(0) // Start the empty grace period.

	reapInOccupancy := make(chan struct{})
	unblockReap := make(chan struct{})
	var calls atomic.Int64
	inst.occupancyHook = func() int {
		if calls.Add(1) == 1 {
			close(reapInOccupancy)
			<-unblockReap
		}
		return 0
	}
	reapDone := make(chan struct{})
	go func() {
		p.Reap(0)
		close(reapDone)
	}()
	<-reapInOccupancy

	id, _, err := p.AcquireChecked(1, 1, 2, 2, func(uint64) (*contractInst, error) {
		t.Fatal("existing instance should have been selected")
		return nil, nil
	})
	if err != nil {
		t.Fatalf("AcquireChecked: %v", err)
	}
	close(unblockReap)
	select {
	case <-reapDone:
	case <-time.After(time.Second):
		t.Fatal("Reap did not finish")
	}
	if _, ok := p.Get(id); !ok {
		t.Fatal("Reap removed an instance with a concurrent reservation")
	}
	if got := inst.closeCalls.Load(); got != 0 {
		t.Fatalf("reserved instance closed %d times", got)
	}
	p.Release(id)
}

func TestPoolReapDoesNotOvertakeConcurrentExactReserve(t *testing.T) {
	p := NewPool[int, *contractInst](nil)
	id, inst, err := p.AddChecked(1, false, func(uint64) (*contractInst, error) {
		return &contractInst{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	p.Reap(0)

	reapInOccupancy := make(chan struct{})
	unblockReap := make(chan struct{})
	var calls atomic.Int64
	inst.occupancyHook = func() int {
		if calls.Add(1) == 1 {
			close(reapInOccupancy)
			<-unblockReap
		}
		return 0
	}
	reapDone := make(chan struct{})
	go func() {
		p.Reap(0)
		close(reapDone)
	}()
	<-reapInOccupancy

	if err := p.ReserveChecked(id); err != nil {
		t.Fatalf("ReserveChecked: %v", err)
	}
	close(unblockReap)
	select {
	case <-reapDone:
	case <-time.After(time.Second):
		t.Fatal("Reap did not finish")
	}
	if _, ok := p.Get(id); !ok {
		t.Fatal("Reap removed an exactly reserved instance")
	}
	if err := p.DestroyChecked(id); !errors.Is(err, ErrInstanceReserved) {
		t.Fatalf("DestroyChecked error=%v, want ErrInstanceReserved", err)
	}
	p.Release(id)
	if err := p.DestroyChecked(id); err != nil {
		t.Fatalf("DestroyChecked after Release: %v", err)
	}
	if err := p.ReserveChecked(id); !errors.Is(err, ErrInstanceNotFound) {
		t.Fatalf("ReserveChecked removed error=%v, want ErrInstanceNotFound", err)
	}
}

func TestPoolKeyVersionDoesNotABAWhenGroupIsRecreated(t *testing.T) {
	p := NewPool[int, *contractInst](nil)
	oldID, oldInst, err := p.AddChecked(1, false, func(uint64) (*contractInst, error) {
		return &contractInst{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	occupancyStarted := make(chan struct{})
	unblockOccupancy := make(chan struct{})
	oldInst.occupancyHook = func() int {
		close(occupancyStarted)
		<-unblockOccupancy
		return 1
	}
	type acquireResult struct {
		id  uint64
		err error
	}
	result := make(chan acquireResult, 1)
	var buildCalls atomic.Int64
	go func() {
		id, _, acquireErr := p.AcquireChecked(1, 0, 1, 1, func(uint64) (*contractInst, error) {
			buildCalls.Add(1)
			return &contractInst{}, nil
		})
		result <- acquireResult{id: id, err: acquireErr}
	}()
	<-occupancyStarted

	if err := p.DestroyChecked(oldID); err != nil {
		t.Fatalf("DestroyChecked old group: %v", err)
	}
	newID, _, err := p.AddChecked(1, false, func(uint64) (*contractInst, error) {
		return &contractInst{}, nil
	})
	if err != nil {
		t.Fatalf("AddChecked recreated group: %v", err)
	}
	close(unblockOccupancy)

	acquired := <-result
	if acquired.err != nil {
		t.Fatalf("AcquireChecked: %v", acquired.err)
	}
	if acquired.id != newID {
		t.Fatalf("AcquireChecked id=%d, want recreated id=%d", acquired.id, newID)
	}
	if got := buildCalls.Load(); got != 0 {
		t.Fatalf("stale key version triggered %d extra builds", got)
	}
	p.Release(newID)
	if err := p.DestroyChecked(newID); err != nil {
		t.Fatalf("DestroyChecked recreated group: %v", err)
	}
	if got := len(p.keyVersion); got != 0 {
		t.Fatalf("destroyed recreated group left %d key versions", got)
	}
}

func TestPoolConcurrentReapAndDestroyCloseOnce(t *testing.T) {
	p := NewPool[int, *contractInst](nil)
	id, inst, err := p.AddChecked(1, false, func(uint64) (*contractInst, error) {
		return &contractInst{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	p.Reap(0)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				p.Reap(0)
				return
			}
			_ = p.DestroyChecked(id)
		}(i)
	}
	wg.Wait()
	if got := inst.closeCalls.Load(); got != 1 {
		t.Fatalf("Close calls = %d, want 1", got)
	}
}

func assertReturnsPromptly(t *testing.T, fn func(), message string) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal(message)
	}
}
