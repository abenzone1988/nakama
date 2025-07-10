package template

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplBuildingConfig struct {
	Cost     int32  `json:"cost"`
	ID       string `json:"id"`
	MaxLevel int32  `json:"maxLevel"`
	MinLevel int32  `json:"minLevel"`
	Rank     int32  `json:"rank"`
	Type     int32  `json:"type"`
}

type TableTplBuildingConfig struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplBuildingConfig
	result    []TplBuildingConfig
}

func NewTableTplBuildingConfig(logger *zap.Logger, loadPath string) *TableTplBuildingConfig {
	return &TableTplBuildingConfig{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplBuildingConfig),
		result:    make([]TplBuildingConfig, 0),
	}
}

func (t *TableTplBuildingConfig) FindByKey(key interface{}) (TplBuildingConfig, bool) {
	val, ok := t.tableData[fmt.Sprintf("%v", key)]
	return val, ok
}

func (t *TableTplBuildingConfig) FindByFilter(f func(TplBuildingConfig) bool) []TplBuildingConfig {
	t.result = make([]TplBuildingConfig, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplBuildingConfig) FindAll() []TplBuildingConfig {
	t.result = make([]TplBuildingConfig, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplBuildingConfig) Release() {
	t.tableData = nil
}

func (t *TableTplBuildingConfig) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplBuildingConfigMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplBuildingConfig.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplBuildingConfigMap(fileContent, t.logger)
}

func DeserializeStringToTplBuildingConfigMap(jsonStr []byte, logger *zap.Logger) map[string]TplBuildingConfig {
	if len(jsonStr) == 0 {
		return make(map[string]TplBuildingConfig)
	}
	var jsonMap map[string]TplBuildingConfig
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplBuildingConfig)
	}
	return jsonMap
}
