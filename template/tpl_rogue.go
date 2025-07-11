package template

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplRogue struct {
	Brief               string        `json:"brief"`
	Cost                int32         `json:"cost"`
	Count               int32         `json:"count"`
	ExistExclusion      []interface{} `json:"existExclusion"`
	IconRes             string        `json:"iconRes"`
	ID                  string        `json:"id"`
	LinkEquipID         []interface{} `json:"linkEquipId"`
	O1                  string        `json:"o1"`
	O2                  int32         `json:"o2"`
	O3                  int32         `json:"o3"`
	O4                  int32         `json:"o4"`
	O5                  int32         `json:"o5"`
	O6                  int32         `json:"o6"`
	P1                  int32         `json:"p1"`
	P2                  int32         `json:"p2"`
	P3                  int32         `json:"p3"`
	P4                  int32         `json:"p4"`
	P5                  int32         `json:"p5"`
	P6                  int32         `json:"p6"`
	Prerequisite        []interface{} `json:"prerequisite"`
	SelectableExclusion []interface{} `json:"selectableExclusion"`
	Senior              int32         `json:"senior"`
	T1                  string        `json:"t1"`
	T2                  int32         `json:"t2"`
	T3                  int32         `json:"t3"`
	T4                  int32         `json:"t4"`
	T5                  int32         `json:"t5"`
	T6                  int32         `json:"t6"`
	Title               string        `json:"title"`
	Type                int32         `json:"type"`
	Weight              int32         `json:"weight"`
}

type TableTplRogue struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplRogue
	result    []TplRogue
}

func NewTableTplRogue(logger *zap.Logger, loadPath string) *TableTplRogue {
	return &TableTplRogue{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplRogue),
		result:    make([]TplRogue, 0),
	}
}

func (t *TableTplRogue) FindByKey(key interface{}) (TplRogue, bool) {
	val, ok := t.tableData[fmt.Sprintf("%v", key)]
	return val, ok
}

func (t *TableTplRogue) FindByFilter(f func(TplRogue) bool) []TplRogue {
	t.result = make([]TplRogue, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplRogue) FindAll() []TplRogue {
	t.result = make([]TplRogue, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplRogue) Release() {
	t.tableData = nil
}

func (t *TableTplRogue) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplRogueMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplRogue.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplRogueMap(fileContent, t.logger)
}

func DeserializeStringToTplRogueMap(jsonStr []byte, logger *zap.Logger) map[string]TplRogue {
	if len(jsonStr) == 0 {
		return make(map[string]TplRogue)
	}
	var jsonMap map[string]TplRogue
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplRogue)
	}
	return jsonMap
}
