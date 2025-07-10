package template

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplPresetUnitBase struct {
	ID           int32  `json:"id"`
	Note         string `json:"note"`
	Param        int32  `json:"param"`
	PresetUnitID string `json:"presetUnitId"`
	ShowTitle    string `json:"showTitle"`
	Type         string `json:"type"`
}

type TableTplPresetUnitBase struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplPresetUnitBase
	result    []TplPresetUnitBase
}

func NewTableTplPresetUnitBase(logger *zap.Logger, loadPath string) *TableTplPresetUnitBase {
	return &TableTplPresetUnitBase{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplPresetUnitBase),
		result:    make([]TplPresetUnitBase, 0),
	}
}

func (t *TableTplPresetUnitBase) FindByKey(key interface{}) (TplPresetUnitBase, bool) {
	val, ok := t.tableData[fmt.Sprintf("%v", key)]
	return val, ok
}

func (t *TableTplPresetUnitBase) FindByFilter(f func(TplPresetUnitBase) bool) []TplPresetUnitBase {
	t.result = make([]TplPresetUnitBase, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplPresetUnitBase) FindAll() []TplPresetUnitBase {
	t.result = make([]TplPresetUnitBase, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplPresetUnitBase) Release() {
	t.tableData = nil
}

func (t *TableTplPresetUnitBase) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplPresetUnitBaseMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplPresetUnitBase.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplPresetUnitBaseMap(fileContent, t.logger)
}

func DeserializeStringToTplPresetUnitBaseMap(jsonStr []byte, logger *zap.Logger) map[string]TplPresetUnitBase {
	if len(jsonStr) == 0 {
		return make(map[string]TplPresetUnitBase)
	}
	var jsonMap map[string]TplPresetUnitBase
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplPresetUnitBase)
	}
	return jsonMap
}
