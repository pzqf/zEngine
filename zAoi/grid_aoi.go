package zAoi

import (
	"github.com/pzqf/zEngine/zLog"
	"github.com/pzqf/zUtil/zMap"
	"go.uber.org/zap"
)

type Coord struct {
	X float64
	Y float64
}

type AOIEventType int

const (
	AOIEventEnter AOIEventType = iota
	AOIEventLeave
	AOIEventMove
)

type AOIEvent struct {
	Type     AOIEventType
	Watcher  int64
	Target   int64
	OldCoord Coord
	NewCoord Coord
}

type AOIListener func(event AOIEvent)

type Grid struct {
	entityID int64
	entities *zMap.TypedMap[int64, Coord]
}

func NewGrid(entityID int64) *Grid {
	return &Grid{
		entityID: entityID,
		entities: zMap.NewTypedMap[int64, Coord](),
	}
}

func (g *Grid) Add(entityID int64, coord Coord) {
	g.entities.Store(entityID, coord)
}

func (g *Grid) Remove(entityID int64) {
	g.entities.Delete(entityID)
}

func (g *Grid) Update(entityID int64, coord Coord) {
	g.entities.Store(entityID, coord)
}

func (g *Grid) GetAll() map[int64]Coord {
	result := make(map[int64]Coord)
	g.entities.Range(func(id int64, coord Coord) bool {
		result[id] = coord
		return true
	})
	return result
}

func (g *Grid) Count() int {
	return int(g.entities.Len())
}

type GridManager struct {
	gridWidth  float64
	gridHeight float64
	mapWidth   float64
	mapHeight  float64
	grids      *zMap.TypedMap[int64, *Grid]
	listener   AOIListener
}

func NewGridManager(mapWidth, mapHeight, gridWidth, gridHeight float64) *GridManager {
	return &GridManager{
		gridWidth:  gridWidth,
		gridHeight: gridHeight,
		mapWidth:   mapWidth,
		mapHeight:  mapHeight,
		grids:      zMap.NewTypedMap[int64, *Grid](),
	}
}

func (gm *GridManager) SetListener(listener AOIListener) {
	gm.listener = listener
}

func (gm *GridManager) getGridID(coord Coord) int64 {
	gx := int(coord.X / gm.gridWidth)
	gy := int(coord.Y / gm.gridHeight)
	if gx < 0 {
		gx = 0
	}
	if gy < 0 {
		gy = 0
	}
	maxGX := int(gm.mapWidth / gm.gridWidth)
	maxGY := int(gm.mapHeight / gm.gridHeight)
	if gx >= maxGX {
		gx = maxGX - 1
	}
	if gy >= maxGY {
		gy = maxGY - 1
	}
	return int64(gy*maxGX + gx)
}

func (gm *GridManager) getSurroundingGridIDs(gridID int64) []int64 {
	maxGX := int(gm.mapWidth / gm.gridWidth)
	maxGY := int(gm.mapHeight / gm.gridHeight)
	gx := int(gridID) % maxGX
	gy := int(gridID) / maxGX

	var ids []int64
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			nx := gx + dx
			ny := gy + dy
			if nx >= 0 && nx < maxGX && ny >= 0 && ny < maxGY {
				ids = append(ids, int64(ny*maxGX+nx))
			}
		}
	}
	return ids
}

func (gm *GridManager) getOrCreateGrid(gridID int64) *Grid {
	if grid, ok := gm.grids.Load(gridID); ok {
		return grid
	}
	grid := NewGrid(gridID)
	gm.grids.Store(gridID, grid)
	return grid
}

func (gm *GridManager) AddEntity(entityID int64, coord Coord) {
	gridID := gm.getGridID(coord)
	grid := gm.getOrCreateGrid(gridID)

	// 先对既有实体（不含自身，此时尚未加入自身格）触发双向进入视野事件，再加入自身：
	//  ① 既有实体看见新实体进入（Watcher=既有, Target=新）
	//  ② 新实体也看见既有实体（Watcher=新, Target=既有）——此前缺失，导致新玩家进区
	//     收不到"已在场对象"，AOI 对新进入者无用。修复后进入者能获知周围已有实体。
	if gm.listener != nil {
		surroundingGrids := gm.getSurroundingGridIDs(gridID)
		for _, sid := range surroundingGrids {
			sgrid := gm.getOrCreateGrid(sid)
			for otherID, otherCoord := range sgrid.GetAll() {
				if otherID == entityID {
					continue
				}
				gm.listener(AOIEvent{Type: AOIEventEnter, Watcher: otherID, Target: entityID, NewCoord: coord})
				gm.listener(AOIEvent{Type: AOIEventEnter, Watcher: entityID, Target: otherID, NewCoord: otherCoord})
			}
		}
	}

	grid.Add(entityID, coord)

	zLog.Debug("Entity added to AOI",
		zap.Int64("entity_id", entityID),
		zap.Int64("grid_id", gridID))
}

func (gm *GridManager) RemoveEntity(entityID int64, coord Coord) {
	oldGridID := gm.getGridID(coord)

	surroundingGrids := gm.getSurroundingGridIDs(oldGridID)
	for _, sid := range surroundingGrids {
		sgrid := gm.getOrCreateGrid(sid)
		for watcherID := range sgrid.GetAll() {
			if watcherID != entityID && gm.listener != nil {
				gm.listener(AOIEvent{
					Type:     AOIEventLeave,
					Watcher:  watcherID,
					Target:   entityID,
					OldCoord: coord,
				})
			}
		}
	}

	if grid, ok := gm.grids.Load(oldGridID); ok {
		grid.Remove(entityID)
	}

	zLog.Debug("Entity removed from AOI",
		zap.Int64("entity_id", entityID),
		zap.Int64("grid_id", oldGridID))
}

func (gm *GridManager) MoveEntity(entityID int64, oldCoord, newCoord Coord) {
	oldGridID := gm.getGridID(oldCoord)
	newGridID := gm.getGridID(newCoord)

	if oldGridID == newGridID {
		if grid, ok := gm.grids.Load(oldGridID); ok {
			grid.Update(entityID, newCoord)
		}

		if gm.listener != nil {
			gm.listener(AOIEvent{
				Type:     AOIEventMove,
				Target:   entityID,
				OldCoord: oldCoord,
				NewCoord: newCoord,
			})
		}
		return
	}

	gm.RemoveEntity(entityID, oldCoord)
	gm.AddEntity(entityID, newCoord)
}

func (gm *GridManager) GetSurroundingEntities(coord Coord) map[int64]Coord {
	gridID := gm.getGridID(coord)
	surroundingGrids := gm.getSurroundingGridIDs(gridID)

	result := make(map[int64]Coord)
	for _, sid := range surroundingGrids {
		if grid, ok := gm.grids.Load(sid); ok {
			for id, c := range grid.GetAll() {
				result[id] = c
			}
		}
	}
	return result
}

func (gm *GridManager) GetEntitiesInGrid(coord Coord) map[int64]Coord {
	gridID := gm.getGridID(coord)
	if grid, ok := gm.grids.Load(gridID); ok {
		return grid.GetAll()
	}
	return make(map[int64]Coord)
}

func (gm *GridManager) GetGridID(coord Coord) int64 {
	return gm.getGridID(coord)
}
