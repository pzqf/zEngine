package zAoi

import (
	"errors"
	"math"
	"sort"
	"sync"

	"github.com/pzqf/zEngine/zLog"
	"github.com/pzqf/zUtil/zMap"
	"go.uber.org/zap"
)

var (
	ErrInvalidMapSize  = errors.New("zAoi: invalid map size")
	ErrInvalidGridSize = errors.New("zAoi: invalid grid size")
	ErrDuplicateEntity = errors.New("zAoi: duplicate entity")
	ErrEntityNotFound  = errors.New("zAoi: entity not found")
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
	mu         sync.RWMutex
	gridWidth  float64
	gridHeight float64
	mapWidth   float64
	mapHeight  float64
	gridCols   int64
	gridRows   int64
	grids      *zMap.TypedMap[int64, *Grid]
	entities   map[int64]entityLocation
	listener   AOIListener
}

type entityLocation struct {
	gridID int64
	coord  Coord
}

// NewGridManager keeps the original constructor signature for source compatibility.
// Deprecated: use NewGridManagerChecked when dimensions are not compile-time constants.
func NewGridManager(mapWidth, mapHeight, gridWidth, gridHeight float64) *GridManager {
	gm, err := NewGridManagerChecked(mapWidth, mapHeight, gridWidth, gridHeight)
	if err != nil {
		panic(err)
	}
	return gm
}

func NewGridManagerChecked(mapWidth, mapHeight, gridWidth, gridHeight float64) (*GridManager, error) {
	if !validDimension(mapWidth) || !validDimension(mapHeight) {
		return nil, ErrInvalidMapSize
	}
	if !validDimension(gridWidth) || !validDimension(gridHeight) {
		return nil, ErrInvalidGridSize
	}

	cols := math.Ceil(mapWidth / gridWidth)
	rows := math.Ceil(mapHeight / gridHeight)
	const maxGridAxisCount = float64(1<<31 - 1)
	if !validDimension(cols) || !validDimension(rows) || cols > maxGridAxisCount || rows > maxGridAxisCount {
		return nil, ErrInvalidGridSize
	}

	return &GridManager{
		gridWidth:  gridWidth,
		gridHeight: gridHeight,
		mapWidth:   mapWidth,
		mapHeight:  mapHeight,
		gridCols:   int64(cols),
		gridRows:   int64(rows),
		grids:      zMap.NewTypedMap[int64, *Grid](),
		entities:   make(map[int64]entityLocation),
	}, nil
}

func validDimension(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func (gm *GridManager) SetListener(listener AOIListener) {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	gm.listener = listener
}

func (gm *GridManager) getGridID(coord Coord) int64 {
	gx := gridAxis(coord.X, gm.mapWidth, gm.gridWidth, gm.gridCols)
	gy := gridAxis(coord.Y, gm.mapHeight, gm.gridHeight, gm.gridRows)
	return gy*gm.gridCols + gx
}

func gridAxis(value, mapSize, gridSize float64, count int64) int64 {
	if math.IsNaN(value) || value <= 0 {
		return 0
	}
	if math.IsInf(value, 1) || value >= mapSize {
		return count - 1
	}
	return int64(value / gridSize)
}

func (gm *GridManager) getSurroundingGridIDs(gridID int64) []int64 {
	gx := gridID % gm.gridCols
	gy := gridID / gm.gridCols

	var ids []int64
	for dy := int64(-1); dy <= 1; dy++ {
		for dx := int64(-1); dx <= 1; dx++ {
			nx := gx + dx
			ny := gy + dy
			if nx >= 0 && nx < gm.gridCols && ny >= 0 && ny < gm.gridRows {
				ids = append(ids, ny*gm.gridCols+nx)
			}
		}
	}
	return ids
}

func (gm *GridManager) getOrCreateGridLocked(gridID int64) *Grid {
	if grid, ok := gm.grids.Load(gridID); ok {
		return grid
	}
	grid := NewGrid(gridID)
	gm.grids.Store(gridID, grid)
	return grid
}

func (gm *GridManager) visibleEntitiesLocked(gridID int64) map[int64]Coord {
	result := make(map[int64]Coord)
	for _, surroundingID := range gm.getSurroundingGridIDs(gridID) {
		grid, ok := gm.grids.Load(surroundingID)
		if !ok {
			continue
		}
		for entityID, coord := range grid.GetAll() {
			result[entityID] = coord
		}
	}
	return result
}

func (gm *GridManager) AddEntity(entityID int64, coord Coord) error {
	gm.mu.Lock()
	if _, exists := gm.entities[entityID]; exists {
		gm.mu.Unlock()
		return ErrDuplicateEntity
	}

	gridID := gm.getGridID(coord)
	grid := gm.getOrCreateGridLocked(gridID)
	grid.Add(entityID, coord)
	gm.entities[entityID] = entityLocation{gridID: gridID, coord: coord}

	visible := gm.visibleEntitiesLocked(gridID)
	delete(visible, entityID)
	events := make([]AOIEvent, 0, len(visible)*2)
	for _, otherID := range sortedEntityIDs(visible) {
		otherCoord := visible[otherID]
		events = append(events,
			AOIEvent{Type: AOIEventEnter, Watcher: otherID, Target: entityID, NewCoord: coord},
			AOIEvent{Type: AOIEventEnter, Watcher: entityID, Target: otherID, NewCoord: otherCoord},
		)
	}
	listener := gm.listener
	gm.mu.Unlock()

	emitAOIEvents(listener, events)

	zLog.Debug("Entity added to AOI",
		zap.Int64("entity_id", entityID),
		zap.Int64("grid_id", gridID))
	return nil
}

// RemoveEntity uses the manager's authoritative entity location. The optional
// coordinate is accepted only so existing callers compile; it is intentionally ignored.
func (gm *GridManager) RemoveEntity(entityID int64, _ ...Coord) error {
	gm.mu.Lock()
	location, exists := gm.entities[entityID]
	if !exists {
		gm.mu.Unlock()
		return ErrEntityNotFound
	}

	visible := gm.visibleEntitiesLocked(location.gridID)
	delete(visible, entityID)
	if grid, ok := gm.grids.Load(location.gridID); ok {
		grid.Remove(entityID)
		if grid.Count() == 0 {
			gm.grids.Delete(location.gridID)
		}
	}
	delete(gm.entities, entityID)

	events := make([]AOIEvent, 0, len(visible)*2)
	for _, otherID := range sortedEntityIDs(visible) {
		otherCoord := visible[otherID]
		events = append(events,
			AOIEvent{Type: AOIEventLeave, Watcher: entityID, Target: otherID, OldCoord: otherCoord},
			AOIEvent{Type: AOIEventLeave, Watcher: otherID, Target: entityID, OldCoord: location.coord},
		)
	}
	listener := gm.listener
	gm.mu.Unlock()

	emitAOIEvents(listener, events)

	zLog.Debug("Entity removed from AOI",
		zap.Int64("entity_id", entityID),
		zap.Int64("grid_id", location.gridID))
	return nil
}

// MoveEntity keeps the old signature for source compatibility. oldCoord is no
// longer trusted; MoveEntityTo uses the manager's authoritative location.
// Deprecated: use MoveEntityTo.
func (gm *GridManager) MoveEntity(entityID int64, _ Coord, newCoord Coord) error {
	return gm.MoveEntityTo(entityID, newCoord)
}

func (gm *GridManager) MoveEntityTo(entityID int64, newCoord Coord) error {
	gm.mu.Lock()
	location, exists := gm.entities[entityID]
	if !exists {
		gm.mu.Unlock()
		return ErrEntityNotFound
	}

	newGridID := gm.getGridID(newCoord)
	oldVisible := gm.visibleEntitiesLocked(location.gridID)
	newVisible := gm.visibleEntitiesLocked(newGridID)
	delete(oldVisible, entityID)
	delete(newVisible, entityID)

	if location.gridID == newGridID {
		grid, ok := gm.grids.Load(location.gridID)
		if !ok {
			gm.mu.Unlock()
			return ErrEntityNotFound
		}
		grid.Update(entityID, newCoord)
	} else {
		oldGrid, ok := gm.grids.Load(location.gridID)
		if !ok {
			gm.mu.Unlock()
			return ErrEntityNotFound
		}
		oldGrid.Remove(entityID)
		if oldGrid.Count() == 0 {
			gm.grids.Delete(location.gridID)
		}
		gm.getOrCreateGridLocked(newGridID).Add(entityID, newCoord)
	}
	gm.entities[entityID] = entityLocation{gridID: newGridID, coord: newCoord}

	events := buildMoveEvents(entityID, location.coord, newCoord, oldVisible, newVisible)
	listener := gm.listener
	gm.mu.Unlock()

	emitAOIEvents(listener, events)
	zLog.Debug("Entity moved in AOI",
		zap.Int64("entity_id", entityID),
		zap.Int64("old_grid_id", location.gridID),
		zap.Int64("new_grid_id", newGridID))
	return nil
}

func buildMoveEvents(entityID int64, oldCoord, newCoord Coord, oldVisible, newVisible map[int64]Coord) []AOIEvent {
	events := make([]AOIEvent, 0, len(oldVisible)*2+len(newVisible)*2)

	for _, otherID := range sortedEntityIDs(oldVisible) {
		otherCoord := oldVisible[otherID]
		if _, remainsVisible := newVisible[otherID]; remainsVisible {
			continue
		}
		events = append(events,
			AOIEvent{Type: AOIEventLeave, Watcher: entityID, Target: otherID, OldCoord: otherCoord},
			AOIEvent{Type: AOIEventLeave, Watcher: otherID, Target: entityID, OldCoord: oldCoord},
		)
	}

	for _, otherID := range sortedEntityIDs(newVisible) {
		otherCoord := newVisible[otherID]
		if _, wasVisible := oldVisible[otherID]; wasVisible {
			continue
		}
		events = append(events,
			AOIEvent{Type: AOIEventEnter, Watcher: entityID, Target: otherID, NewCoord: otherCoord},
			AOIEvent{Type: AOIEventEnter, Watcher: otherID, Target: entityID, NewCoord: newCoord},
		)
	}

	for _, otherID := range sortedEntityIDs(oldVisible) {
		if _, remainsVisible := newVisible[otherID]; !remainsVisible {
			continue
		}
		events = append(events, AOIEvent{
			Type:     AOIEventMove,
			Watcher:  otherID,
			Target:   entityID,
			OldCoord: oldCoord,
			NewCoord: newCoord,
		})
	}
	return events
}

func sortedEntityIDs(entities map[int64]Coord) []int64 {
	ids := make([]int64, 0, len(entities))
	for entityID := range entities {
		ids = append(ids, entityID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func emitAOIEvents(listener AOIListener, events []AOIEvent) {
	if listener == nil {
		return
	}
	for _, event := range events {
		listener(event)
	}
}

func (gm *GridManager) GetSurroundingEntities(coord Coord) map[int64]Coord {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	gridID := gm.getGridID(coord)
	return gm.visibleEntitiesLocked(gridID)
}

func (gm *GridManager) GetEntitiesInGrid(coord Coord) map[int64]Coord {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	gridID := gm.getGridID(coord)
	if grid, ok := gm.grids.Load(gridID); ok {
		return grid.GetAll()
	}
	return make(map[int64]Coord)
}

func (gm *GridManager) GetGridID(coord Coord) int64 {
	return gm.getGridID(coord)
}
