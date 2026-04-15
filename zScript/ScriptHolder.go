package zScript

import (
	"errors"
	"reflect"
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

		if reflect.TypeOf(ret).String() == "bool" && ret.(bool) {
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
