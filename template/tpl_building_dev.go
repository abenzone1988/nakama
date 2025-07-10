package template

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplBuildingDev struct {
	CostDebris int32  `json:"costDebris"`
	EquipID    string `json:"equipId"`
	ID         string `json:"id"`
	Level      int32  `json:"level"`
	P1         string `json:"p1"`
	P2         int32  `json:"p2"`
	P3         int32  `json:"p3"`
	RogueID    string `json:"rogueId"`
	RougeDesc  string `json:"rougeDesc"`
	V1         int32  `json:"v1"`
	V2         int32  `json:"v2"`
	V3         int32  `json:"v3"`
}

type TableTplBuildingDev struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplBuildingDev
	result    []TplBuildingDev
}

func NewTableTplBuildingDev(logger *zap.Logger, loadPath string) *TableTplBuildingDev {
	return &TableTplBuildingDev{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplBuildingDev),
		result:    make([]TplBuildingDev, 0),
	}
}

func (t *TableTplBuildingDev) FindByKey(key interface{}) (TplBuildingDev, bool) {
	val, ok := t.tableData[fmt.Sprintf("%v", key)]
	return val, ok
}

func (t *TableTplBuildingDev) FindByFilter(f func(TplBuildingDev) bool) []TplBuildingDev {
	t.result = make([]TplBuildingDev, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplBuildingDev) FindAll() []TplBuildingDev {
	t.result = make([]TplBuildingDev, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplBuildingDev) Release() {
	t.tableData = nil
}

func (t *TableTplBuildingDev) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplBuildingDevMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplBuildingDev.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplBuildingDevMap(fileContent, t.logger)
}

func DeserializeStringToTplBuildingDevMap(jsonStr []byte, logger *zap.Logger) map[string]TplBuildingDev {
	if len(jsonStr) == 0 {
		return make(map[string]TplBuildingDev)
	}
	var jsonMap map[string]TplBuildingDev
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplBuildingDev)
	}
	return jsonMap
}
