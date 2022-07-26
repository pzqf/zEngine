package zScript

import (
	"errors"
	"fmt"
	"reflect"
)

type ScriptHolder struct {
	script           *ScriptData
	currScriptNodeId string
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

	if sh.script.getEntry() == nil {
		return errors.New("the script " + scriptFilename + " no entry node")
	}

	sh.ResetScript()

	return nil
}

func (sh *ScriptHolder) ResetScript() {
	sh.currScriptNodeId = sh.script.getEntry().Id
}

func (sh *ScriptHolder) Update(deltaTime int) {
	if sh.currScriptNodeId == "" {
		sh.ResetScript()
	}

	script := *sh.script

	if script[sh.currScriptNodeId].Content == "Exit" {
		return
	}

	newNodeId := sh.currScriptNodeId
	fmt.Println("===current node:", sh.currScriptNodeId)
	for _, edge := range script[sh.currScriptNodeId].Edges {
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
		fmt.Println("-->to node:", newNodeId, ", exec:", script[newNodeId].Content)
		sh.currScriptNodeId = newNodeId
		if script[newNodeId].Content != "Exit" && script[newNodeId].Stmt != nil {
			expressionEval(sh, script[newNodeId].Stmt)
		}
	}
}
