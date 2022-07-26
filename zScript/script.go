package zScript

import (
	"encoding/xml"
	"errors"
	"fmt"
	"go/ast"
	"log"
	"os"
	"reflect"
	"strings"

	"github.com/dennwc/graphml"
)

type Edge struct {
	Id      string `json:"id"`
	Content string `json:"content"`
	Stmt    *ast.ExprStmt
	Target  string `json:"target"`
}

type Node struct {
	Id      string `json:"id"`
	Content string `json:"content"`
	Stmt    *ast.ExprStmt
	Edges   []Edge `json:"edges"`
}

// ScriptData map[node.id]*node
type ScriptData map[string]*Node

func (sn ScriptData) getEntry() *Node {
	for _, v := range sn {
		if v.Content == "Entry" {
			return v
		}
	}
	return nil
}

// ScriptFileList  map[filename]*ScriptData
var scriptFileList = make(map[string]*ScriptData)

func LoadScriptFile(filename string) error {
	log.Println("load script file", filename)
	if _, ok := scriptFileList[filename]; ok {
		log.Println("script file", filename, "had load")
		return errors.New(fmt.Sprintf("script file %s had load", filename))
	}

	var nodes = make(ScriptData)
	r, err := os.Open(filename)
	if err != nil {
		log.Println(err)
		return err
	}

	decode, err := graphml.Decode(r)
	if err != nil {
		log.Println(err)
		return err
	}

	for _, v := range decode.Graphs {
		for _, node := range v.Nodes {
			//log.Println("==node id:", node.ID)
			n := Node{Id: node.ID}
			for _, data := range node.Data {
				for _, t := range data.Data {
					if reflect.TypeOf(t).String() == "xml.CharData" {
						content := string(t.(xml.CharData))

						if !strings.Contains(content, "\n") {
							//log.Println("node:", node.ID, "value:", content)
							n.Content = content
						}

					}
				}
			}
			n.Stmt = astParser(n.Content)
			nodes[node.ID] = &n
		}

		for _, edge := range v.Edges {
			//log.Println("===edge id:", edge.ID, "line:", edge.Source, "-->", edge.Target)
			_, ok := nodes[edge.Source]
			if !ok {
				//log.Println("edge error, no source node =====")
				continue
			}

			e := Edge{
				Id:     edge.ID,
				Target: edge.Target,
			}

			for _, data := range edge.Data {
				for _, t := range data.Data {
					if reflect.TypeOf(t).String() == "xml.CharData" {
						content := string(t.(xml.CharData))
						if !strings.Contains(content, "\n") {
							//log.Println("edge:", edge.ID, "value:", content)
							e.Content = content
						}
					}
				}
			}
			e.Stmt = astParser(e.Content)
			nodes[edge.Source].Edges = append(nodes[edge.Source].Edges, e)
		}
	}

	scriptFileList[filename] = &nodes

	return nil
}

func GetScriptData(filename string) (*ScriptData, error) {
	if v, ok := scriptFileList[filename]; ok {
		return v, nil
	}

	log.Println("script file", filename, "had load")
	return nil, errors.New(fmt.Sprintf("script file %s had load", filename))
}
