package zAoi

import (
	"errors"
	"math"
	"testing"
	"time"
)

type aoiEventKey struct {
	typ     AOIEventType
	watcher int64
	target  int64
}

func TestNewGridManagerCheckedRejectsInvalidDimensions(t *testing.T) {
	tests := []struct {
		name                  string
		mapWidth, mapHeight   float64
		gridWidth, gridHeight float64
		want                  error
	}{
		{name: "zero map width", mapWidth: 0, mapHeight: 100, gridWidth: 50, gridHeight: 50, want: ErrInvalidMapSize},
		{name: "negative map height", mapWidth: 100, mapHeight: -1, gridWidth: 50, gridHeight: 50, want: ErrInvalidMapSize},
		{name: "nan map width", mapWidth: math.NaN(), mapHeight: 100, gridWidth: 50, gridHeight: 50, want: ErrInvalidMapSize},
		{name: "infinite map height", mapWidth: 100, mapHeight: math.Inf(1), gridWidth: 50, gridHeight: 50, want: ErrInvalidMapSize},
		{name: "zero grid width", mapWidth: 100, mapHeight: 100, gridWidth: 0, gridHeight: 50, want: ErrInvalidGridSize},
		{name: "negative grid height", mapWidth: 100, mapHeight: 100, gridWidth: 50, gridHeight: -1, want: ErrInvalidGridSize},
		{name: "nan grid width", mapWidth: 100, mapHeight: 100, gridWidth: math.NaN(), gridHeight: 50, want: ErrInvalidGridSize},
		{name: "infinite grid height", mapWidth: 100, mapHeight: 100, gridWidth: 50, gridHeight: math.Inf(1), want: ErrInvalidGridSize},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gm, err := NewGridManagerChecked(tt.mapWidth, tt.mapHeight, tt.gridWidth, tt.gridHeight)
			if gm != nil {
				t.Fatalf("manager = %#v, want nil", gm)
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestLegacyNewGridManagerFailsFastOnInvalidDimensions(t *testing.T) {
	defer func() {
		recovered := recover()
		err, ok := recovered.(error)
		if !ok || !errors.Is(err, ErrInvalidMapSize) {
			t.Fatalf("panic = %#v, want ErrInvalidMapSize", recovered)
		}
	}()
	NewGridManager(0, 100, 50, 50)
}

func TestGridManagerCoversPartialBoundaryGrid(t *testing.T) {
	gm := newCheckedGridManager(t, 101, 101, 50, 50)
	coord := Coord{X: 100.5, Y: 100.5}
	if got := gm.GetGridID(coord); got != 8 {
		t.Fatalf("partial boundary grid ID = %d, want 8", got)
	}
	if err := gm.AddEntity(1, coord); err != nil {
		t.Fatalf("AddEntity: %v", err)
	}
	if _, ok := gm.GetEntitiesInGrid(coord)[1]; !ok {
		t.Fatal("entity missing from partial boundary grid")
	}
}

func TestGridManagerRejectsDuplicateAndMissingEntity(t *testing.T) {
	gm := newCheckedGridManager(t, 300, 300, 50, 50)
	if err := gm.AddEntity(1, Coord{X: 10, Y: 10}); err != nil {
		t.Fatalf("first AddEntity: %v", err)
	}

	if err := gm.AddEntity(1, Coord{X: 250, Y: 250}); !errors.Is(err, ErrDuplicateEntity) {
		t.Fatalf("duplicate AddEntity error = %v, want ErrDuplicateEntity", err)
	}
	if _, ok := gm.GetEntitiesInGrid(Coord{X: 10, Y: 10})[1]; !ok {
		t.Fatal("duplicate add moved the authoritative entity")
	}
	if _, ok := gm.GetEntitiesInGrid(Coord{X: 250, Y: 250})[1]; ok {
		t.Fatal("duplicate add left an entity in a second grid")
	}

	if err := gm.MoveEntityTo(999, Coord{X: 20, Y: 20}); !errors.Is(err, ErrEntityNotFound) {
		t.Fatalf("missing MoveEntityTo error = %v, want ErrEntityNotFound", err)
	}
	if err := gm.RemoveEntity(999); !errors.Is(err, ErrEntityNotFound) {
		t.Fatalf("missing RemoveEntity error = %v, want ErrEntityNotFound", err)
	}
}

func TestGridManagerRemoveUsesAuthoritativeLocationAndEmitsBothDirections(t *testing.T) {
	gm := newCheckedGridManager(t, 300, 300, 50, 50)
	events := collectAOIEvents(gm)
	mustAddEntity(t, gm, 1, Coord{X: 10, Y: 10})
	mustAddEntity(t, gm, 2, Coord{X: 15, Y: 15})
	*events = nil

	if err := gm.RemoveEntity(1, Coord{X: 250, Y: 250}); err != nil {
		t.Fatalf("RemoveEntity with stale legacy coordinate: %v", err)
	}
	assertAOIEventKeys(t, *events, []aoiEventKey{
		{typ: AOIEventLeave, watcher: 1, target: 2},
		{typ: AOIEventLeave, watcher: 2, target: 1},
	})
	if _, ok := gm.GetSurroundingEntities(Coord{X: 10, Y: 10})[1]; ok {
		t.Fatal("entity remained at its authoritative location after remove")
	}
}

func TestGridManagerSameGridMoveUsesRealWatcher(t *testing.T) {
	gm := newCheckedGridManager(t, 300, 300, 50, 50)
	events := collectAOIEvents(gm)
	mustAddEntity(t, gm, 1, Coord{X: 10, Y: 10})
	mustAddEntity(t, gm, 2, Coord{X: 15, Y: 15})
	*events = nil

	if err := gm.MoveEntityTo(1, Coord{X: 20, Y: 20}); err != nil {
		t.Fatalf("MoveEntityTo: %v", err)
	}
	assertAOIEventKeys(t, *events, []aoiEventKey{
		{typ: AOIEventMove, watcher: 2, target: 1},
	})
	assertNoZeroWatcher(t, *events)
}

func TestGridManagerAdjacentMoveEmitsOldNewAndIntersectionDelta(t *testing.T) {
	gm := newCheckedGridManager(t, 300, 300, 50, 50)
	events := collectAOIEvents(gm)
	mustAddEntity(t, gm, 1, Coord{X: 60, Y: 10})  // moving: grid 1 -> grid 2
	mustAddEntity(t, gm, 2, Coord{X: 10, Y: 10})  // old visible only: grid 0
	mustAddEntity(t, gm, 3, Coord{X: 70, Y: 10})  // intersection: grid 1
	mustAddEntity(t, gm, 4, Coord{X: 160, Y: 10}) // new visible only: grid 3
	*events = nil

	if err := gm.MoveEntityTo(1, Coord{X: 110, Y: 10}); err != nil {
		t.Fatalf("MoveEntityTo: %v", err)
	}
	assertAOIEventKeys(t, *events, []aoiEventKey{
		{typ: AOIEventLeave, watcher: 1, target: 2},
		{typ: AOIEventLeave, watcher: 2, target: 1},
		{typ: AOIEventEnter, watcher: 1, target: 4},
		{typ: AOIEventEnter, watcher: 4, target: 1},
		{typ: AOIEventMove, watcher: 3, target: 1},
	})
	assertNoZeroWatcher(t, *events)
}

func TestGridManagerDistantMoveEmitsBidirectionalLeaveAndEnter(t *testing.T) {
	gm := newCheckedGridManager(t, 400, 400, 50, 50)
	events := collectAOIEvents(gm)
	mustAddEntity(t, gm, 1, Coord{X: 10, Y: 10})
	mustAddEntity(t, gm, 2, Coord{X: 20, Y: 20})
	mustAddEntity(t, gm, 3, Coord{X: 310, Y: 310})
	*events = nil

	if err := gm.MoveEntityTo(1, Coord{X: 320, Y: 320}); err != nil {
		t.Fatalf("MoveEntityTo: %v", err)
	}
	assertAOIEventKeys(t, *events, []aoiEventKey{
		{typ: AOIEventLeave, watcher: 1, target: 2},
		{typ: AOIEventLeave, watcher: 2, target: 1},
		{typ: AOIEventEnter, watcher: 1, target: 3},
		{typ: AOIEventEnter, watcher: 3, target: 1},
	})
	assertNoZeroWatcher(t, *events)
}

func TestGridManagerListenerObservesCommittedStateOutsideManagerLock(t *testing.T) {
	gm := newCheckedGridManager(t, 300, 300, 50, 50)
	mustAddEntity(t, gm, 1, Coord{X: 10, Y: 10})
	mustAddEntity(t, gm, 2, Coord{X: 15, Y: 15})

	listenerResult := make(chan error, 1)
	gm.SetListener(func(event AOIEvent) {
		if event.Type != AOIEventMove {
			return
		}
		coord, ok := gm.GetEntitiesInGrid(event.NewCoord)[event.Target]
		if !ok {
			listenerResult <- errors.New("listener could not observe moved entity")
			return
		}
		if coord != event.NewCoord {
			listenerResult <- errors.New("listener observed stale entity coordinate")
			return
		}
		listenerResult <- nil
	})

	moveDone := make(chan error, 1)
	go func() {
		moveDone <- gm.MoveEntityTo(1, Coord{X: 20, Y: 20})
	}()
	select {
	case err := <-moveDone:
		if err != nil {
			t.Fatalf("MoveEntityTo: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("MoveEntityTo deadlocked while listener re-entered GridManager")
	}
	select {
	case err := <-listenerResult:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("move listener was not called")
	}
}

func newCheckedGridManager(t *testing.T, mapWidth, mapHeight, gridWidth, gridHeight float64) *GridManager {
	t.Helper()
	gm, err := NewGridManagerChecked(mapWidth, mapHeight, gridWidth, gridHeight)
	if err != nil {
		t.Fatalf("NewGridManagerChecked: %v", err)
	}
	return gm
}

func collectAOIEvents(gm *GridManager) *[]AOIEvent {
	events := make([]AOIEvent, 0)
	gm.SetListener(func(event AOIEvent) {
		events = append(events, event)
	})
	return &events
}

func mustAddEntity(t *testing.T, gm *GridManager, entityID int64, coord Coord) {
	t.Helper()
	if err := gm.AddEntity(entityID, coord); err != nil {
		t.Fatalf("AddEntity(%d): %v", entityID, err)
	}
}

func assertAOIEventKeys(t *testing.T, got []AOIEvent, want []aoiEventKey) {
	t.Helper()
	gotCounts := make(map[aoiEventKey]int, len(got))
	for _, event := range got {
		gotCounts[aoiEventKey{typ: event.Type, watcher: event.Watcher, target: event.Target}]++
	}
	wantCounts := make(map[aoiEventKey]int, len(want))
	for _, event := range want {
		wantCounts[event]++
	}
	if len(got) != len(want) {
		t.Fatalf("event count = %d, want %d; got=%v", len(got), len(want), gotCounts)
	}
	for key, count := range wantCounts {
		if gotCounts[key] != count {
			t.Fatalf("event %v count = %d, want %d; all=%v", key, gotCounts[key], count, gotCounts)
		}
	}
}

func assertNoZeroWatcher(t *testing.T, events []AOIEvent) {
	t.Helper()
	for _, event := range events {
		if event.Watcher == 0 {
			t.Fatalf("event has zero watcher: %+v", event)
		}
	}
}
