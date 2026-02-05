package zScript

import (
	"errors"
	"fmt"
	"reflect"
)

// ScriptHolder 脚本持有者
// 管理脚本的执行状态、当前节点位置和上下文数据
type ScriptHolder struct {
	script           *ScriptData // 绑定的脚本数据
	currScriptNodeId string      // 当前节点ID
	context          interface{} // 用户上下文数据
}

// BindScript 绑定脚本文件
// 如果脚本未加载则自动加载，然后绑定到当前持有者
// 参数:
//   - scriptFilename: 脚本文件路径
//
// 返回:
//   - error: 绑定失败返回错误
func (sh *ScriptHolder) BindScript(scriptFilename string) error {
	var err error

	// 尝试获取已加载的脚本
	sh.script, err = GetScriptData(scriptFilename)
	if err != nil {
		// 未加载则加载脚本
		err = LoadScriptFile(scriptFilename)
		if err != nil {
			return err
		}
	}

	// 重新获取脚本数据
	sh.script, _ = GetScriptData(scriptFilename)

	// 检查是否有入口节点
	if sh.script.GetEntry() == nil {
		return errors.New("the script " + scriptFilename + " no entry node")
	}

	// 重置脚本状态
	sh.ResetScript()

	return nil
}

// ResetScript 重置脚本状态
// 将当前节点重置为入口节点
func (sh *ScriptHolder) ResetScript() {
	sh.currScriptNodeId = sh.script.GetEntry().Id
}

// Update 更新脚本执行状态
// 根据边的条件表达式判断并转移到下一个节点
// 参数:
//   - deltaTime: 时间增量（毫秒），当前未使用
func (sh *ScriptHolder) Update(deltaTime int) {
	// 检查当前节点有效性
	if sh.currScriptNodeId == "" {
		sh.ResetScript()
	}

	if sh.getNodeById(sh.currScriptNodeId) == nil {
		return
	}

	// 检查是否到达退出节点
	if sh.getNodeById(sh.currScriptNodeId).Content == "Exit" {
		return
	}

	newNodeId := sh.currScriptNodeId
	fmt.Println("===current node:", sh.currScriptNodeId)

	// 获取从当前节点出发的所有边
	edges := sh.getEdgesFromNode(sh.currScriptNodeId)
	for _, edge := range edges {
		fmt.Print("    |check edge:", edge.Id, ", condition:", edge.Content)

		// 无边条件（无条件转移）
		if edge.Stmt == nil {
			newNodeId = edge.Target
			break
		}

		// 求值边的条件表达式
		ret := expressionEval(sh, edge.Stmt.X)
		fmt.Println(", return:", ret)

		// 如果条件为true，转移到目标节点
		if reflect.TypeOf(ret).String() == "bool" && ret.(bool) {
			newNodeId = edge.Target
			break
		}
	}

	// 如果节点发生变化，执行新节点
	if newNodeId != sh.currScriptNodeId {
		targetNode := sh.getNodeById(newNodeId)
		if targetNode != nil {
			fmt.Println("-->to node:", newNodeId, ", exec:", targetNode.Content)
			sh.currScriptNodeId = newNodeId

			// 非退出节点且有语句时执行
			if targetNode.Content != "Exit" && targetNode.Stmt != nil {
				expressionEval(sh, targetNode.Stmt)
			}
		}
	}
}

// getNodeById 根据ID获取节点
// 参数:
//   - id: 节点ID
//
// 返回:
//   - *Node: 节点指针，未找到返回nil
func (sh *ScriptHolder) getNodeById(id string) *Node {
	for i := range sh.script.Nodes {
		if sh.script.Nodes[i].Id == id {
			return &sh.script.Nodes[i]
		}
	}
	return nil
}

// GetContext 获取上下文
// 返回:
//   - interface{}: 用户设置的上下文数据
func (sh *ScriptHolder) GetContext() interface{} {
	return sh.context
}

// SetContext 设置上下文
// 参数:
//   - ctx: 上下文数据
func (sh *ScriptHolder) SetContext(ctx interface{}) {
	sh.context = ctx
}

// getEdgesFromNode 获取从指定节点出发的所有边
// 参数:
//   - nodeId: 节点ID
//
// 返回:
//   - []Edge: 边列表
func (sh *ScriptHolder) getEdgesFromNode(nodeId string) []Edge {
	var edges []Edge
	for i := range sh.script.Edges {
		if sh.script.Edges[i].Source == nodeId {
			edges = append(edges, sh.script.Edges[i])
		}
	}
	return edges
}
