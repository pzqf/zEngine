package zNavMap

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pzqf/zUtil/zColor"

	"github.com/pzqf/zUtil/zDataConv"
)

func StringToMap(charMap []string) *NavMap {
	maxX := len(charMap)
	maxY := 0

	// 计算maxY
	for _, row := range charMap {
		cols := strings.Split(row, " ")
		if len(cols) > maxY {
			maxY = len(cols)
		}
	}

	m := NewNavMap(maxX, maxY, 1)

	for x, row := range charMap {
		cols := strings.Split(row, " ")
		for y, view := range cols {
			grid := Grid{X: x, Y: y, Pos: Vector3d{Z: 0}}
			if view != "-" {
				n, _ := zDataConv.String2Float64(view)
				grid.Pos.Z = n
			}
			m.AddGrid(grid)
		} // end of cols
	} // end of row

	return &m
}

func PrintMap(m *NavMap, road []*Grid) {
	for x := 0; x < m.maxX; x++ {
		for y := 0; y < m.maxY; y++ {
			for i := 0; i < len(road); i++ {
				if road[i].X == x && road[i].Y == y {
					switch i {
					case 0:
						fmt.Print(" " + zColor.LightGreen("E"))
					case len(road) - 1:
						fmt.Print(" " + zColor.LightGreen("S"))
					default:
						fmt.Print(" " + zColor.LightGreen("*"))
					}
					goto NEXT
				}
			}
			if m.grids[x][y].Pos.Z > 0 {
				if m.grids[x][y].Pos.Z > 9 {
					fmt.Print(" " + zColor.LightRed("X"))
				} else {
					fmt.Print(" " + zColor.LightRed(zDataConv.Float642String(m.grids[x][y].Pos.Z)))
				}

			} else {
				fmt.Print(" " + zColor.LightCyan("-"))
			}
		NEXT:
		}
		fmt.Println()
	}
}

func TestAStar(t *testing.T) {
	strMap := []string{
		"- - - - - - - - - - - - - - - - - - - - - - - - - - -",
		"5 5 5 5 5 5 5 5 5 5 5 - 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5",
		"- - - - - - - - - - - - - - - - - - - - - - - - - - -",
		"5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 1",
		"- - - - - - - - - - - - - - - - - - - - - - - - - - -",
		"- - 5 5 1 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5",
		"- - - - - - - - - - - - - - - - - - 5 - - - - - - - -",
		"- - - - - - 5 - - - - - - - - - - - 5 - - - - 5 - - -",
		"- - - - - - 5 - - - - - - - - 5 - - 5 - - - - 5 - - -",
		"- - - - - - 5 - - - - - - - - 5 - - 5 - - - - 5 - - -",
		"- - - - - - - - - - - - - - - 5 - - - - - - - 5 - - -",
		"5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 - 5 5",
		"- - - - - - - - - - - - - - - - - - - - - - 5 - - - -",
		"- - - - - - - 5 - - - - - - - - - - - 5 - - 5 - - - -",
		"- - - - - - - - 5 5 5 5 5 5 5 5 5 5 5 5 - - 5 - - - -",
		"- - - - - - - - - - - - - - - - - - - 5 - - 5 - - - -",
		"5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 5 - - - 5 - - 5 - - - -",
		"- - - - - - - - - - - - - - - - - - - 5 - - - - - - -",
		"- - - - - - - - - - - - - - - - - - - 5 - - - - - - -",
	}

	begin := time.Now()
	m := StringToMap(strMap)
	//PrintMap(m, nil)

	road, err := FindPathByAStar(Grid{X: 0, Y: 0, Pos: Vector3d{}}, Grid{X: 18, Y: 14, Pos: Vector3d{}}, m)
	if err != nil {
		t.Logf("Error finding path: %v", err)
		return
	}
	PrintMap(m, road)
	t.Logf("cost: %v", time.Now().Sub(begin).String())
}
