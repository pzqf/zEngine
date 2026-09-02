package zInstance

import (
	"testing"
	"time"
)

func TestPoolSnapshotReportsCreatingAndActiveReservation(t *testing.T) {
	pool := NewPool[string, *fakeInst](nil)
	buildStarted := make(chan struct{})
	releaseBuild := make(chan struct{})
	created := make(chan *fakeInst, 1)

	go func() {
		_, instance, err := pool.AddReservedChecked("dungeon", true, func(uint64) (*fakeInst, error) {
			close(buildStarted)
			<-releaseBuild
			return &fakeInst{}, nil
		})
		if err != nil {
			created <- nil
			return
		}
		created <- instance
	}()
	<-buildStarted

	creating := pool.Snapshot()
	if len(creating) != 1 || creating[0].State != LifecycleCreating || creating[0].Reserved != 1 ||
		!creating[0].Pinned || creating[0].Key != "dungeon" || creating[0].Generation == 0 {
		t.Fatalf("creating snapshot = %+v", creating)
	}

	close(releaseBuild)
	select {
	case instance := <-created:
		if instance == nil {
			t.Fatal("reserved instance creation failed")
		}
	case <-time.After(time.Second):
		t.Fatal("reserved instance creation did not finish")
	}

	active := pool.Snapshot()
	if len(active) != 1 || active[0].State != LifecycleActive || active[0].Reserved != 1 ||
		active[0].ID != creating[0].ID || active[0].Generation != creating[0].Generation ||
		active[0].Revision <= creating[0].Revision {
		t.Fatalf("active snapshot = %+v; creating = %+v", active, creating)
	}

	pool.Release(active[0].ID)
	released := pool.Snapshot()
	if len(released) != 1 || released[0].Reserved != 0 || released[0].Revision <= active[0].Revision {
		t.Fatalf("released snapshot = %+v; active = %+v", released, active)
	}
}

func TestPoolSnapshotIsDetachedFromPoolState(t *testing.T) {
	pool := NewPool[string, *fakeInst](nil)
	id, _, err := pool.AddChecked("layer", false, func(uint64) (*fakeInst, error) {
		return &fakeInst{}, nil
	})
	if err != nil {
		t.Fatalf("AddChecked: %v", err)
	}

	snapshot := pool.Snapshot()
	if len(snapshot) != 1 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	snapshot[0].Reserved = 99
	if current := pool.Snapshot(); len(current) != 1 || current[0].Reserved != 0 {
		t.Fatalf("pool snapshot was mutable through caller copy: %+v", current)
	}

	if err := pool.DestroyChecked(id); err != nil {
		t.Fatalf("DestroyChecked: %v", err)
	}
	if current := pool.Snapshot(); len(current) != 0 {
		t.Fatalf("destroyed entry remains in snapshot: %+v", current)
	}
}
