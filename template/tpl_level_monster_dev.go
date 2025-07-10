package template

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplLevelMonsterDev struct {
	Describe string  `json:"describe"`
	GroupID  string  `json:"groupId"`
	HpScale  float64 `json:"hpScale"`
	ID       string  `json:"id"`
	Progress int32   `json:"progress"`
}

type TableTplLevelMonsterDev struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplLevelMonsterDev
	result    []TplLevelMonsterDev
}

func NewTableTplLevelMonsterDev(logger *zap.Logger, loadPath string) *TableTplLevelMonsterDev {
	return &TableTplLevelMonsterDev{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplLevelMonsterDev),
		result:    make([]TplLevelMonsterDev, 0),
	}
}

func (t *TableTplLevelMonsterDev) FindByKey(key interface{}) (TplLevelMonsterDev, bool) {
	val, ok := t.tableData[fmt.Sprintf("%v", key)]
	return val, ok
}

func (t *TableTplLevelMonsterDev) FindByFilter(f func(TplLevelMonsterDev) bool) []TplLevelMonsterDev {
	t.result = make([]TplLevelMonsterDev, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplLevelMonsterDev) FindAll() []TplLevelMonsterDev {
	t.result = make([]TplLevelMonsterDev, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplLevelMonsterDev) Release() {
	t.tableData = nil
}

func (t *TableTplLevelMonsterDev) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplLevelMonsterDevMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplLevelMonsterDev.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplLevelMonsterDevMap(fileContent, t.logger)
}

func DeserializeStringToTplLevelMonsterDevMap(jsonStr []byte, logger *zap.Logger) map[string]TplLevelMonsterDev {
	if len(jsonStr) == 0 {
		return make(map[string]TplLevelMonsterDev)
	}
	var jsonMap map[string]TplLevelMonsterDev
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplLevelMonsterDev)
	}
	return jsonMap
}
