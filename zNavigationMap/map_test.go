package zNavigationMap

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pzqf/zUtil/zColor"

	"github.com/pzqf/zUtil/zDataConv"
)

func StringToMap(charMap []string) *NavigationMap {
	grids := make([][]Grid, len(charMap))
	maxX := len(grids)
	maxY := 0
	for x, row := range charMap {
		cols := strings.Split(row, " ")
		grids[x] = make([]Grid, len(cols))
		if maxY < len(cols) {
			maxY = len(cols)
		}
		for y, view := range cols {
			grids[x][y] = Grid{x, y, Vector3d{0, 0, 0}}
			if view != "-" {
				n, _ := zDataConv.String2Float64(view)
				grids[x][y].Pos.Z = n

			}
		} // end of cols
	} // end of row

	m := NewNavigationMap(maxX, maxY, 1)
	for _, x := range grids {
		for _, v := range x {
			_ = m.AddGrid(v)
		}
	}

	return &m
}

func PrintMap(m *NavigationMap, road []*Grid) {
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

func Test(t *testing.T) {
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

	road, err := FindPathByAStar(Grid{0, 0, Vector3d{0, 0, 0}}, Grid{18, 14, Vector3d{18, 14, 0}}, m)
	if err != nil {
		fmt.Println(err)
		return
	}
	PrintMap(m, road)
	fmt.Println("cost:", time.Now().Sub(begin).String())
}
