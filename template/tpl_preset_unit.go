package template

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplPresetUnit struct {
	RescueBrief string `json:"RescueBrief"`
	Brief       string `json:"brief"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	PrefabPath  string `json:"prefabPath"`
	TargetType  int32  `json:"targetType"`
	UIPath      string `json:"uiPath"`
}

type TableTplPresetUnit struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplPresetUnit
	result    []TplPresetUnit
}

func NewTableTplPresetUnit(logger *zap.Logger, loadPath string) *TableTplPresetUnit {
	return &TableTplPresetUnit{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplPresetUnit),
		result:    make([]TplPresetUnit, 0),
	}
}

func (t *TableTplPresetUnit) FindByKey(key interface{}) (TplPresetUnit, bool) {
	val, ok := t.tableData[fmt.Sprintf("%v", key)]
	return val, ok
}

func (t *TableTplPresetUnit) FindByFilter(f func(TplPresetUnit) bool) []TplPresetUnit {
	t.result = make([]TplPresetUnit, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplPresetUnit) FindAll() []TplPresetUnit {
	t.result = make([]TplPresetUnit, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplPresetUnit) Release() {
	t.tableData = nil
}

func (t *TableTplPresetUnit) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplPresetUnitMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplPresetUnit.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplPresetUnitMap(fileContent, t.logger)
}

func DeserializeStringToTplPresetUnitMap(jsonStr []byte, logger *zap.Logger) map[string]TplPresetUnit {
	if len(jsonStr) == 0 {
		return make(map[string]TplPresetUnit)
	}
	var jsonMap map[string]TplPresetUnit
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplPresetUnit)
	}
	return jsonMap
}
