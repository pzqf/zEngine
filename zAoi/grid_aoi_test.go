package zAoi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewGridManager(t *testing.T) {
	gm := NewGridManager(100, 100, 50, 50)

	assert.NotNil(t, gm)
	assert.Equal(t, 50.0, gm.gridWidth)
	assert.Equal(t, 50.0, gm.gridHeight)
}

func TestGridManagerGetGridID(t *testing.T) {
	gm := NewGridManager(100, 100, 50, 50)

	assert.Equal(t, int64(0), gm.GetGridID(Coord{X: 10, Y: 10}))
	assert.Equal(t, int64(1), gm.GetGridID(Coord{X: 60, Y: 10}))
	assert.Equal(t, int64(2), gm.GetGridID(Coord{X: 10, Y: 60}))
	assert.Equal(t, int64(3), gm.GetGridID(Coord{X: 60, Y: 60}))
}

func TestGridManagerAddEntity(t *testing.T) {
	gm := NewGridManager(100, 100, 50, 50)

	gm.AddEntity(1, Coord{X: 10, Y: 10})

	entities := gm.GetSurroundingEntities(Coord{X: 10, Y: 10})
	assert.Contains(t, entities, int64(1))
}

func TestGridManagerRemoveEntity(t *testing.T) {
	gm := NewGridManager(100, 100, 50, 50)

	gm.AddEntity(1, Coord{X: 10, Y: 10})
	gm.RemoveEntity(1, Coord{X: 10, Y: 10})

	entities := gm.GetSurroundingEntities(Coord{X: 10, Y: 10})
	assert.NotContains(t, entities, int64(1))
}

func TestGridManagerMoveEntitySameGrid(t *testing.T) {
	gm := NewGridManager(100, 100, 50, 50)

	gm.AddEntity(1, Coord{X: 10, Y: 10})
	gm.MoveEntity(1, Coord{X: 10, Y: 10}, Coord{X: 20, Y: 20})

	entities := gm.GetSurroundingEntities(Coord{X: 20, Y: 20})
	assert.Contains(t, entities, int64(1))
}

func TestGridManagerMoveEntityCrossGrid(t *testing.T) {
	gm := NewGridManager(300, 300, 50, 50)

	gm.AddEntity(1, Coord{X: 10, Y: 10})
	gm.MoveEntity(1, Coord{X: 10, Y: 10}, Coord{X: 250, Y: 250})

	entities := gm.GetSurroundingEntities(Coord{X: 250, Y: 250})
	assert.Contains(t, entities, int64(1))

	oldEntities := gm.GetSurroundingEntities(Coord{X: 10, Y: 10})
	assert.NotContains(t, oldEntities, int64(1))
}

func TestGridManagerAOIEnterEvent(t *testing.T) {
	gm := NewGridManager(100, 100, 50, 50)

	var events []AOIEvent
	gm.SetListener(func(evt AOIEvent) {
		events = append(events, evt)
	})

	gm.AddEntity(1, Coord{X: 10, Y: 10})
	gm.AddEntity(2, Coord{X: 15, Y: 15})

	// AddEntity 双向触发（Phase 2.3 修复：此前只触发"既有实体看见新实体"，导致进入者
	// 收不到已在场对象）：实体 2 进入实体 1 视野，实体 1 也进入实体 2 视野 → 2 个事件。
	assert.Len(t, events, 2)
	foundW1, foundW2 := false, false
	for _, evt := range events {
		assert.Equal(t, AOIEventEnter, evt.Type)
		if evt.Watcher == 1 && evt.Target == 2 {
			foundW1 = true
		}
		if evt.Watcher == 2 && evt.Target == 1 {
			foundW2 = true
		}
	}
	assert.True(t, foundW1, "entity 1 should see entity 2 enter")
	assert.True(t, foundW2, "entity 2 should see entity 1 enter")
}

func TestGridManagerAOILeaveEvent(t *testing.T) {
	gm := NewGridManager(100, 100, 50, 50)

	var events []AOIEvent
	gm.SetListener(func(evt AOIEvent) {
		events = append(events, evt)
	})

	gm.AddEntity(1, Coord{X: 10, Y: 10})
	gm.AddEntity(2, Coord{X: 15, Y: 15})

	enterCount := len(events)
	gm.RemoveEntity(2, Coord{X: 15, Y: 15})

	leaveEvents := events[enterCount:]
	assert.True(t, len(leaveEvents) >= 1)
	found := false
	for _, evt := range leaveEvents {
		if evt.Type == AOIEventLeave && evt.Target == 2 {
			found = true
			break
		}
	}
	assert.True(t, found)
}

func TestGridManagerGetSurroundingEntities(t *testing.T) {
	gm := NewGridManager(200, 200, 50, 50)

	gm.AddEntity(1, Coord{X: 10, Y: 10})
	gm.AddEntity(2, Coord{X: 60, Y: 10})
	gm.AddEntity(3, Coord{X: 150, Y: 150})

	nearby := gm.GetSurroundingEntities(Coord{X: 10, Y: 10})
	assert.Contains(t, nearby, int64(1))
	assert.Contains(t, nearby, int64(2))
	assert.NotContains(t, nearby, int64(3))
}

func TestGridManagerGetEntitiesInGrid(t *testing.T) {
	gm := NewGridManager(100, 100, 50, 50)

	gm.AddEntity(1, Coord{X: 10, Y: 10})
	gm.AddEntity(2, Coord{X: 20, Y: 20})

	inGrid := gm.GetEntitiesInGrid(Coord{X: 10, Y: 10})
	assert.Len(t, inGrid, 2)
}

func TestGridManagerMultipleEntities(t *testing.T) {
	gm := NewGridManager(100, 100, 50, 50)

	for i := int64(1); i <= 10; i++ {
		gm.AddEntity(i, Coord{X: float64(i * 5), Y: float64(i * 5)})
	}

	nearby := gm.GetSurroundingEntities(Coord{X: 25, Y: 25})
	assert.True(t, len(nearby) > 0)
}
