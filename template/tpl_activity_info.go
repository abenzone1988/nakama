package template

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplActivityInfo struct {
	LevelMonsterDev    string   `json:"LevelMonsterDev"`
	AddHpScale         float64  `json:"addHpScale"`
	BattleWaveGroupID  string   `json:"battleWaveGroupId"`
	Brief              string   `json:"brief"`
	BuildingIds        []string `json:"buildingIds"`
	Condition01        int32    `json:"condition01"`
	Condition02        int32    `json:"condition02"`
	Condition03        int32    `json:"condition03"`
	EndlessWavesCount  int32    `json:"endlessWavesCount"`
	ID                 string   `json:"id"`
	MaiBaseID          string   `json:"maiBaseId"`
	MonsterLevel       int32    `json:"monsterLevel"`
	MonsterWaveGroupID string   `json:"monsterWaveGroupId"`
	Name               string   `json:"name"`
	PropertyLevel      int32    `json:"propertyLevel"`
	Reward01           string   `json:"reward01"`
	Reward02           string   `json:"reward02"`
	Reward03           string   `json:"reward03"`
	SceneConfigData    string   `json:"sceneConfigData"`
	UIName             string   `json:"uiName"`
}

type TableTplActivityInfo struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplActivityInfo
	result    []TplActivityInfo
}

func NewTableTplActivityInfo(logger *zap.Logger, loadPath string) *TableTplActivityInfo {
	return &TableTplActivityInfo{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplActivityInfo),
		result:    make([]TplActivityInfo, 0),
	}
}

func (t *TableTplActivityInfo) FindByKey(key interface{}) (TplActivityInfo, bool) {
	val, ok := t.tableData[fmt.Sprintf("%v", key)]
	return val, ok
}

func (t *TableTplActivityInfo) FindByFilter(f func(TplActivityInfo) bool) []TplActivityInfo {
	t.result = make([]TplActivityInfo, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplActivityInfo) FindAll() []TplActivityInfo {
	t.result = make([]TplActivityInfo, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplActivityInfo) Release() {
	t.tableData = nil
}

func (t *TableTplActivityInfo) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplActivityInfoMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplActivityInfo.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplActivityInfoMap(fileContent, t.logger)
}

func DeserializeStringToTplActivityInfoMap(jsonStr []byte, logger *zap.Logger) map[string]TplActivityInfo {
	if len(jsonStr) == 0 {
		return make(map[string]TplActivityInfo)
	}
	var jsonMap map[string]TplActivityInfo
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplActivityInfo)
	}
	return jsonMap
}
