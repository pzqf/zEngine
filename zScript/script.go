package zScript

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"log"
	"os"
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

var scriptFileList = make(map[string]*ScriptData)

func LoadScriptFile(filename string) error {
	log.Println("load script file", filename)
	if _, ok := scriptFileList[filename]; ok {
		log.Println("script file", filename, "had load")
		return errors.New(fmt.Sprintf("script file %s had load", filename))
	}

	r, err := os.Open(filename)
	if err != nil {
		log.Println(err)
		return err
	}
	defer r.Close()

	var script ScriptData
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&script); err != nil {
		log.Println(err)
		return err
	}

	for i := range script.Nodes {
		script.Nodes[i].Stmt = astParser(script.Nodes[i].Content)
	}

	for i := range script.Edges {
		script.Edges[i].Stmt = astParser(script.Edges[i].Content)
	}

	scriptFileList[filename] = &script

	return nil
}

func GetScriptData(filename string) (*ScriptData, error) {
	if v, ok := scriptFileList[filename]; ok {
		return v, nil
	}

	log.Println("script file", filename, "had load")
	return nil, errors.New(fmt.Sprintf("script file %s had load", filename))
}
