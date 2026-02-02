package zScript

import (
	"errors"
	"fmt"
	"reflect"
)

type ScriptHolder struct {
	script           *ScriptData
	currScriptNodeId string
	context          interface{}
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
	sh.currScriptNodeId = sh.script.GetEntry().Id
}

func (sh *ScriptHolder) Update(deltaTime int) {
	if sh.currScriptNodeId == "" {
		sh.ResetScript()
	}

	if sh.getNodeById(sh.currScriptNodeId) == nil {
		return
	}

	if sh.getNodeById(sh.currScriptNodeId).Content == "Exit" {
		return
	}

	newNodeId := sh.currScriptNodeId
	fmt.Println("===current node:", sh.currScriptNodeId)

	edges := sh.getEdgesFromNode(sh.currScriptNodeId)
	for _, edge := range edges {
		fmt.Print("    |check edge:", edge.Id, ", condition:", edge.Content)
		if edge.Stmt == nil {
			newNodeId = edge.Target
			break
		}
		ret := expressionEval(sh, edge.Stmt.X)
		fmt.Println(", return:", ret)
		if reflect.TypeOf(ret).String() == "bool" && ret.(bool) {
			newNodeId = edge.Target
			break
		}
	}

	if newNodeId != sh.currScriptNodeId {
		targetNode := sh.getNodeById(newNodeId)
		if targetNode != nil {
			fmt.Println("-->to node:", newNodeId, ", exec:", targetNode.Content)
			sh.currScriptNodeId = newNodeId
			if targetNode.Content != "Exit" && targetNode.Stmt != nil {
				expressionEval(sh, targetNode.Stmt)
			}
		}
	}
}

func (sh *ScriptHolder) getNodeById(id string) *Node {
	for i := range sh.script.Nodes {
		if sh.script.Nodes[i].Id == id {
			return &sh.script.Nodes[i]
		}
	}
	return nil
}

// GetContext 获取上下文
func (sh *ScriptHolder) GetContext() interface{} {
	return sh.context
}

// SetContext 设置上下文
func (sh *ScriptHolder) SetContext(ctx interface{}) {
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
