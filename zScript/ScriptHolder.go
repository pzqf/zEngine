package zScript

import (
	"errors"
	"sync"
)

type ScriptHolder struct {
	mu              sync.RWMutex
	script          *ScriptData
	currScriptNodeId string
	context         interface{}
}

func (sh *ScriptHolder) BindScript(scriptFilename string) error {
	var err error

	sh.script, err = GetScriptData(scriptFilename)
	if err != nil {
		err = LoadScriptFile(scriptFilename)
		if err != nil {
			return err
		}
	}

	sh.script, _ = GetScriptData(scriptFilename)

	if sh.script.GetEntry() == nil {
		return errors.New("the script " + scriptFilename + " no entry node")
	}

	sh.ResetScript()

	return nil
}

func (sh *ScriptHolder) ResetScript() {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.currScriptNodeId = sh.script.GetEntry().Id
}

func (sh *ScriptHolder) Update(deltaTime int) {
	sh.mu.Lock()
	if sh.currScriptNodeId == "" {
		sh.currScriptNodeId = sh.script.GetEntry().Id
	}

	if sh.getNodeById(sh.currScriptNodeId) == nil {
		sh.mu.Unlock()
		return
	}

	if sh.getNodeById(sh.currScriptNodeId).Content == "Exit" {
		sh.mu.Unlock()
		return
	}

	newNodeId := sh.currScriptNodeId
	currentNodeId := sh.currScriptNodeId

	edges := sh.getEdgesFromNode(currentNodeId)
	sh.mu.Unlock()

	for _, edge := range edges {
		if edge.Stmt == nil {
			newNodeId = edge.Target
			break
		}

		ret := expressionEval(sh, edge.Stmt.X)

		// 条件求值结果可能是 nil（不支持的表达式 / 函数返回 nil / 操作数类型不匹配）。
		// 用类型断言而不是 reflect.TypeOf(ret).String()——后者在 ret 为 nil 时返回 nil *rtype，
		// 再调 .String() 直接空指针崩溃（本包自带脚本第一次跑到复合条件边就会炸）。
		// 条件求不出布尔真值 = 这条边不通，继续看下一条。
		if b, ok := ret.(bool); ok && b {
			newNodeId = edge.Target
			break
		}
	}

	sh.mu.Lock()
	if newNodeId != sh.currScriptNodeId {
		targetNode := sh.getNodeById(newNodeId)
		if targetNode != nil {
			sh.currScriptNodeId = newNodeId

			if targetNode.Content != "Exit" && targetNode.Stmt != nil {
				sh.mu.Unlock()
				expressionEval(sh, targetNode.Stmt)
				return
			}
		}
	}
	sh.mu.Unlock()
}

func (sh *ScriptHolder) getNodeById(id string) *Node {
	for i := range sh.script.Nodes {
		if sh.script.Nodes[i].Id == id {
			return &sh.script.Nodes[i]
		}
	}
	return nil
}

func (sh *ScriptHolder) GetContext() interface{} {
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	return sh.context
}

func (sh *ScriptHolder) SetContext(ctx interface{}) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.context = ctx
}

func (sh *ScriptHolder) getEdgesFromNode(nodeId string) []Edge {
	var edges []Edge
	for i := range sh.script.Edges {
		if sh.script.Edges[i].Source == nodeId {
			edges = append(edges, sh.script.Edges[i])
		}
	}
	return edges
}

func (sh *ScriptHolder) GetCurrentNodeId() string {
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	return sh.currScriptNodeId
}
