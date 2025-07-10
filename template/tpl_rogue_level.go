package template

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplRogueLevel struct {
	DmgUp float64 `json:"dmgUp"`
	ID    int32   `json:"id"`
}

type TableTplRogueLevel struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplRogueLevel
	result    []TplRogueLevel
}

func NewTableTplRogueLevel(logger *zap.Logger, loadPath string) *TableTplRogueLevel {
	return &TableTplRogueLevel{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplRogueLevel),
		result:    make([]TplRogueLevel, 0),
	}
}

func (t *TableTplRogueLevel) FindByKey(key interface{}) (TplRogueLevel, bool) {
	val, ok := t.tableData[fmt.Sprintf("%v", key)]
	return val, ok
}

func (t *TableTplRogueLevel) FindByFilter(f func(TplRogueLevel) bool) []TplRogueLevel {
	t.result = make([]TplRogueLevel, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplRogueLevel) FindAll() []TplRogueLevel {
	t.result = make([]TplRogueLevel, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplRogueLevel) Release() {
	t.tableData = nil
}

func (t *TableTplRogueLevel) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplRogueLevelMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplRogueLevel.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplRogueLevelMap(fileContent, t.logger)
}

func DeserializeStringToTplRogueLevelMap(jsonStr []byte, logger *zap.Logger) map[string]TplRogueLevel {
	if len(jsonStr) == 0 {
		return make(map[string]TplRogueLevel)
	}
	var jsonMap map[string]TplRogueLevel
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplRogueLevel)
	}
	return jsonMap
}
