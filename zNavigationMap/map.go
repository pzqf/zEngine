package zNavigationMap

import (
	"strconv"
)

// Grid 导航地图上的格子， z代表高度， 一定高度差是可以上去的。
type Grid struct {
	x int
	y int
	z float64
}

// ToUniqueKey 唯一key
func (g Grid) ToUniqueKey() string {
	return strconv.Itoa(g.x) + "-" + strconv.Itoa(g.y)
}

// NavigationMap 导航地图
type NavigationMap struct {
	grids [][]Grid //地图上的块
	//blocks map[string]*Grid //地图上的阻挡，(永远不可到达)
	maxX                     int
	maxY                     int
	canReachHeightDifference float64
}

// NewNavigationMap 初始化地图，
// maxX, maxY 地块最大长度和宽度，
// canReachHeightDifference 可攀爬的高度差
func NewNavigationMap(maxX, maxY int, canReachHeightDifference float64) *NavigationMap {
	m := NavigationMap{
		maxX:                     maxX,
		maxY:                     maxY,
		canReachHeightDifference: canReachHeightDifference,
	}
	m.grids = make([][]Grid, maxX)
	for x := 0; x < maxX; x++ {
		m.grids[x] = make([]Grid, maxY)
		for y := 0; y < maxY; y++ {
			m.grids[x][y] = Grid{x, y, 9999}
		}
	}

	return &m
}

func (m *NavigationMap) AddGrid(g Grid) {
	m.grids[g.x][g.y] = g
}

// GetNeighborGrid 获取相邻点,包含不可到达的点
func (m *NavigationMap) GetNeighborGrid(currGrid *Grid) []*Grid {
	var listGrid []*Grid

	for x := currGrid.x - 1; x <= currGrid.x+1; x++ {
		if x < 0 || x >= m.maxX {
			continue
		}

		for y := currGrid.y - 1; y <= currGrid.y+1; y++ {
			if y < 0 || y >= m.maxY {
				continue
			}

			if x == currGrid.x && y == currGrid.y {
				continue
			}

			listGrid = append(listGrid, &m.grids[x][y])
		}
	}

	return listGrid
}

// CanReachNeighborGrid 是否可以到达，假设可以攀爬高度差为1
func (m *NavigationMap) CanReachNeighborGrid(from, to *Grid) bool {
	if to.z <= from.z+m.canReachHeightDifference {
		return true
	}
	return false
}
