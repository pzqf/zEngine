package zScript

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"os"
	"sync"

	"github.com/pzqf/zEngine/zLog"
	"go.uber.org/zap"
)

type Edge struct {
	Id      string `json:"id"`
	Content string `json:"content"`
	Stmt    *ast.ExprStmt
	Target  string `json:"target"`
	Source  string `json:"source"`
}

type Node struct {
	Id      string `json:"id"`
	Content string `json:"content"`
	Stmt    *ast.ExprStmt
	Edges   []Edge `json:"edges"`
}

type ScriptData struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

func (sd ScriptData) GetEntry() *Node {
	for i := range sd.Nodes {
		if sd.Nodes[i].Content == "Entry" {
			return &sd.Nodes[i]
		}
	}
	return nil
}

var (
	scriptFileList   = make(map[string]*ScriptData)
	scriptFileListMu sync.RWMutex
)

func LoadScriptFile(filename string) error {
	scriptFileListMu.RLock()
	_, exists := scriptFileList[filename]
	scriptFileListMu.RUnlock()

	if exists {
		zLog.Info("script file already loaded", zap.String("filename", filename))
		return fmt.Errorf("script file %s already loaded", filename)
	}

	r, err := os.Open(filename)
	if err != nil {
		zLog.Error("failed to open script file", zap.String("filename", filename), zap.Error(err))
		return err
	}
	defer r.Close()

	var script ScriptData
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&script); err != nil {
		zLog.Error("failed to decode script file", zap.String("filename", filename), zap.Error(err))
		return err
	}

	for i := range script.Nodes {
		script.Nodes[i].Stmt = astParser(script.Nodes[i].Content)
	}

	for i := range script.Edges {
		script.Edges[i].Stmt = astParser(script.Edges[i].Content)
	}

	scriptFileListMu.Lock()
	scriptFileList[filename] = &script
	scriptFileListMu.Unlock()

	zLog.Info("script file loaded", zap.String("filename", filename))
	return nil
}

func GetScriptData(filename string) (*ScriptData, error) {
	scriptFileListMu.RLock()
	defer scriptFileListMu.RUnlock()

	if v, ok := scriptFileList[filename]; ok {
		return v, nil
	}

	return nil, fmt.Errorf("script file %s not found", filename)
}
