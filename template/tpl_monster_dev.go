package template

import (
	"encoding/json"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplMonsterDev struct {
	AtkScale float64 `json:"atkScale"`
	HpScale  float64 `json:"hpScale"`
	ID       int32   `json:"id"`
}

type TableTplMonsterDev struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplMonsterDev
	result    []TplMonsterDev
}

func NewTableTplMonsterDev(logger *zap.Logger, loadPath string) *TableTplMonsterDev {
	return &TableTplMonsterDev{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplMonsterDev),
		result:    make([]TplMonsterDev, 0),
	}
}

func (t *TableTplMonsterDev) FindByKey(key string) (TplMonsterDev, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplMonsterDev) FindByFilter(f func(TplMonsterDev) bool) []TplMonsterDev {
	t.result = make([]TplMonsterDev, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplMonsterDev) FindAll() []TplMonsterDev {
	t.result = make([]TplMonsterDev, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplMonsterDev) Release() {
	t.tableData = nil
}

func (t *TableTplMonsterDev) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplMonsterDevMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplMonsterDev.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplMonsterDevMap(fileContent, t.logger)
}

func DeserializeStringToTplMonsterDevMap(jsonStr []byte, logger *zap.Logger) map[string]TplMonsterDev {
	if len(jsonStr) == 0 {
		return make(map[string]TplMonsterDev)
	}
	var jsonMap map[string]TplMonsterDev
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplMonsterDev)
	}
	return jsonMap
}
