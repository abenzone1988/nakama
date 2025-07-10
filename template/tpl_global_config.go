package template

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplGlobalConfig struct {
	Brief    string   `json:"brief"`
	Data     []string `json:"data"`
	DataType int32    `json:"dataType"`
	DataName string   `json:"data_name"`
	ID       string   `json:"id"`
}

type TableTplGlobalConfig struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplGlobalConfig
	result    []TplGlobalConfig
}

func NewTableTplGlobalConfig(logger *zap.Logger, loadPath string) *TableTplGlobalConfig {
	return &TableTplGlobalConfig{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplGlobalConfig),
		result:    make([]TplGlobalConfig, 0),
	}
}

func (t *TableTplGlobalConfig) FindByKey(key interface{}) (TplGlobalConfig, bool) {
	val, ok := t.tableData[fmt.Sprintf("%v", key)]
	return val, ok
}

func (t *TableTplGlobalConfig) FindByFilter(f func(TplGlobalConfig) bool) []TplGlobalConfig {
	t.result = make([]TplGlobalConfig, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplGlobalConfig) FindAll() []TplGlobalConfig {
	t.result = make([]TplGlobalConfig, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplGlobalConfig) Release() {
	t.tableData = nil
}

func (t *TableTplGlobalConfig) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplGlobalConfigMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplGlobalConfig.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplGlobalConfigMap(fileContent, t.logger)
}

func DeserializeStringToTplGlobalConfigMap(jsonStr []byte, logger *zap.Logger) map[string]TplGlobalConfig {
	if len(jsonStr) == 0 {
		return make(map[string]TplGlobalConfig)
	}
	var jsonMap map[string]TplGlobalConfig
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplGlobalConfig)
	}
	return jsonMap
}
