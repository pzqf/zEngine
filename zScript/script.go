package zScript

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"log"
	"os"
)

// Edge 脚本边（连接两个节点的条件边）
// 表示从源节点到目标节点的转移条件
type Edge struct {
	Id      string        `json:"id"`      // 边的唯一标识
	Content string        `json:"content"` // 条件表达式内容
	Stmt    *ast.ExprStmt // 解析后的AST语句
	Target  string        `json:"target"` // 目标节点ID
	Source  string        `json:"source"` // 源节点ID
}

// Node 脚本节点（状态节点）
// 表示脚本执行流程中的一个状态点
type Node struct {
	Id      string        `json:"id"`      // 节点唯一标识
	Content string        `json:"content"` // 节点内容/执行语句
	Stmt    *ast.ExprStmt // 解析后的AST语句
	Edges   []Edge        `json:"edges"` // 从该节点出发的边列表
}

// ScriptData 脚本数据结构
// 包含完整的脚本节点和边的定义
type ScriptData struct {
	Nodes []Node `json:"nodes"` // 所有节点
	Edges []Edge `json:"edges"` // 所有边
}

// GetEntry 获取脚本入口节点
// 遍历所有节点，找到Content为"Entry"的节点作为入口
// 返回:
//   - *Node: 入口节点指针，未找到返回nil
func (sd ScriptData) GetEntry() *Node {
	for i := range sd.Nodes {
		if sd.Nodes[i].Content == "Entry" {
			return &sd.Nodes[i]
		}
	}
	return nil
}

// scriptFileList 已加载的脚本文件映射
// key: 文件名, value: 脚本数据
var scriptFileList = make(map[string]*ScriptData)

// LoadScriptFile 加载脚本文件
// 从JSON文件读取脚本定义，解析所有节点和边的表达式
// 参数:
//   - filename: 脚本文件路径
//
// 返回:
//   - error: 加载失败返回错误，成功返回nil
func LoadScriptFile(filename string) error {
	log.Println("load script file", filename)

	// 检查是否已加载
	if _, ok := scriptFileList[filename]; ok {
		log.Println("script file", filename, "had load")
		return errors.New(fmt.Sprintf("script file %s had load", filename))
	}

	// 打开文件
	r, err := os.Open(filename)
	if err != nil {
		log.Println(err)
		return err
	}
	defer r.Close()

	// 解析JSON
	var script ScriptData
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&script); err != nil {
		log.Println(err)
		return err
	}

	// 解析所有节点的表达式
	for i := range script.Nodes {
		script.Nodes[i].Stmt = astParser(script.Nodes[i].Content)
	}

	// 解析所有边的条件表达式
	for i := range script.Edges {
		script.Edges[i].Stmt = astParser(script.Edges[i].Content)
	}

	// 缓存脚本数据
	scriptFileList[filename] = &script

	return nil
}

// GetScriptData 获取已加载的脚本数据
// 参数:
//   - filename: 脚本文件名
//
// 返回:
//   - *ScriptData: 脚本数据指针
//   - error: 未找到返回错误
func GetScriptData(filename string) (*ScriptData, error) {
	if v, ok := scriptFileList[filename]; ok {
		return v, nil
	}

	log.Println("script file", filename, "had load")
	return nil, fmt.Errorf("script file %s had load", filename)
}
