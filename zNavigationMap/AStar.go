package zNavigationMap

import (
	"container/heap"
	"errors"
	"math"
)

type Node struct {
	Grid
	father *Node
	f      int
}

func newNode(p *Grid, father *Node, end *Node) *Node {
	ap := &Node{*p, father, 0}
	if end != nil {
		ap.calcF(end)
	}
	return ap
}

func (asp *Node) calcF(end *Node) int {
	//g
	if asp.father != nil {
		deltaX := int(math.Abs(float64(asp.father.x - asp.x)))
		deltaY := int(math.Abs(float64(asp.father.y - asp.y)))
		if deltaX == 1 && deltaY == 0 {
			asp.f = asp.father.f + 10
		} else if deltaX == 0 && deltaY == 1 {
			asp.f = asp.father.f + 10
		} else if deltaX == 1 && deltaY == 1 {
			asp.f = asp.father.f + 14
		} else {
			//panic("father node is invalid!")
			asp.f = math.MaxInt
		}
	}
	//h
	asp.f += int(math.Abs(float64(end.x-asp.x))+math.Abs(float64(end.y-asp.y))) * 10

	return asp.f
}

type NodeQueue []*Node

func (nq NodeQueue) Len() int {
	return len(nq)
}

func (nq NodeQueue) Less(i, j int) bool {
	return nq[i].f < nq[j].f
}

func (nq NodeQueue) Swap(i, j int) {
	nq[i], nq[j] = nq[j], nq[i]
}

func (nq *NodeQueue) Push(x interface{}) {
	*nq = append(*nq, x.(*Node))
}

func (nq *NodeQueue) Pop() interface{} {
	old := *nq
	n := len(old)
	x := old[n-1]
	*nq = old[0 : n-1]
	return x
}

// FindPathByAStar A*寻路
func FindPathByAStar(start, end Grid, m *NavigationMap) ([]*Grid, error) {
	startNode := newNode(&start, nil, nil)
	endNode := newNode(&end, nil, nil)

	var road []*Grid

	var nq = NodeQueue{}
	heap.Init(&nq)
	heap.Push(&nq, startNode)

	openList := make(map[string]*Node)
	closeList := make(map[string]*Node)

	openList[startNode.ToUniqueKey()] = startNode

	for len(nq) > 0 {
		// 将节点从开放列表移到关闭列表当中。
		currNode := heap.Pop(&nq).(*Node)
		delete(openList, currNode.ToUniqueKey())
		closeList[currNode.ToUniqueKey()] = currNode

		//周围的节点
		neighborList := m.GetNeighborGrid(&currNode.Grid)
		for _, neighborGrid := range neighborList {
			//阻挡物
			/*if _, ok := m.blocks[neighborGrid.ToUniqueKey()]; ok {
				continue
			}*/
			//如果不可达到
			if !m.CanReachNeighborGrid(&currNode.Grid, neighborGrid) {
				continue
			}

			neighborNode := newNode(neighborGrid, currNode, endNode)
			if neighborNode.ToUniqueKey() == endNode.ToUniqueKey() {
				//已找到路径
				for neighborNode.father != nil {
					road = append(road, &neighborNode.Grid)
					neighborNode = neighborNode.father
				}
				road = append(road, &start)
				return road, nil
			}

			//如果邻居在关闭列表中，则继续搜索下一个邻居
			if _, ok := closeList[neighborGrid.ToUniqueKey()]; ok {
				continue
			}

			oldNode, ok := openList[neighborGrid.ToUniqueKey()]
			if !ok {
				//如果未在开放列表中，则加入开放列表,表示从示有其他路线到达过此点
				heap.Push(&nq, neighborNode)
				openList[neighborNode.ToUniqueKey()] = neighborNode
			} else {
				//如果在开放列表中，表示有从其他路径到达此节点
				tmpNode := newNode(&oldNode.Grid, currNode, endNode)
				if tmpNode.f < oldNode.f {
					oldNode.father = currNode
					oldNode.calcF(endNode)
				}
			}
		}
	}
	return nil, errors.New("can't find road")
}
