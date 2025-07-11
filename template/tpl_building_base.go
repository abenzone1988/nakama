package template

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplBuildingBase struct {
	BuildingID string  `json:"buildingId"`
	ID         int32   `json:"id"`
	Note       string  `json:"note"`
	Param      float64 `json:"param"`
	ShowTitle  string  `json:"showTitle"`
	Type       string  `json:"type"`
}

type TableTplBuildingBase struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplBuildingBase
	result    []TplBuildingBase
}

func NewTableTplBuildingBase(logger *zap.Logger, loadPath string) *TableTplBuildingBase {
	return &TableTplBuildingBase{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplBuildingBase),
		result:    make([]TplBuildingBase, 0),
	}
}

func (t *TableTplBuildingBase) FindByKey(key interface{}) (TplBuildingBase, bool) {
	val, ok := t.tableData[fmt.Sprintf("%v", key)]
	return val, ok
}

func (t *TableTplBuildingBase) FindByFilter(f func(TplBuildingBase) bool) []TplBuildingBase {
	t.result = make([]TplBuildingBase, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplBuildingBase) FindAll() []TplBuildingBase {
	t.result = make([]TplBuildingBase, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplBuildingBase) Release() {
	t.tableData = nil
}

func (t *TableTplBuildingBase) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplBuildingBaseMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplBuildingBase.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplBuildingBaseMap(fileContent, t.logger)
}

func DeserializeStringToTplBuildingBaseMap(jsonStr []byte, logger *zap.Logger) map[string]TplBuildingBase {
	if len(jsonStr) == 0 {
		return make(map[string]TplBuildingBase)
	}
	var jsonMap map[string]TplBuildingBase
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplBuildingBase)
	}
	return jsonMap
}
