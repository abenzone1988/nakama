package template

import (
	"encoding/json"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplBuilding struct {
	BaseParameter []string `json:"baseParameter"`
	Brief         string   `json:"brief"`
	BuildingGroup int32    `json:"buildingGroup"`
	DebrisID      string   `json:"debrisId"`
	ID            string   `json:"id"`
	InitLevel     int32    `json:"initLevel"`
	Name          string   `json:"name"`
	PrefabPath    string   `json:"prefabPath"`
	SortIndex     int32    `json:"sortIndex"`
	TargetType    int32    `json:"targetType"`
	UIPath        string   `json:"uiPath"`
	UnlockLevel   int32    `json:"unlockLevel"`
}

type TableTplBuilding struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplBuilding
	result    []TplBuilding
}

func NewTableTplBuilding(logger *zap.Logger, loadPath string) *TableTplBuilding {
	return &TableTplBuilding{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplBuilding),
		result:    make([]TplBuilding, 0),
	}
}

func (t *TableTplBuilding) FindByKey(key string) (TplBuilding, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplBuilding) FindByFilter(f func(TplBuilding) bool) []TplBuilding {
	t.result = make([]TplBuilding, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplBuilding) FindAll() []TplBuilding {
	t.result = make([]TplBuilding, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplBuilding) Release() {
	t.tableData = nil
}

func (t *TableTplBuilding) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplBuildingMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplBuilding.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplBuildingMap(fileContent, t.logger)
}

func DeserializeStringToTplBuildingMap(jsonStr []byte, logger *zap.Logger) map[string]TplBuilding {
	if len(jsonStr) == 0 {
		return make(map[string]TplBuilding)
	}
	var jsonMap map[string]TplBuilding
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplBuilding)
	}
	return jsonMap
}
