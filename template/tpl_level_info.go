package template

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplLevelInfo struct {
	LevelMonsterDev         string        `json:"LevelMonsterDev"`
	BattleWaveGroupID       string        `json:"battleWaveGroupId"`
	Chapter                 int32         `json:"chapter"`
	Condition01             int32         `json:"condition01"`
	Condition02             int32         `json:"condition02"`
	Condition03             int32         `json:"condition03"`
	EquipmentIds            []interface{} `json:"equipmentIds"`
	ID                      string        `json:"id"`
	MonsterLevel            int32         `json:"monsterLevel"`
	MonsterWaveGroupID      string        `json:"monsterWaveGroupId"`
	MoppingReward           string        `json:"moppingReward"`
	Name                    string        `json:"name"`
	OnHookRewardCoin        int32         `json:"onHookRewardCoin"`
	OnHookRewardDebrisCount int32         `json:"onHookRewardDebrisCount"`
	OnHookRewardGroupID     string        `json:"onHookRewardGroupId"`
	Reward01                string        `json:"reward01"`
	Reward02                string        `json:"reward02"`
	Reward03                string        `json:"reward03"`
	SceneConfigData         string        `json:"sceneConfigData"`
	SceneNode               string        `json:"sceneNode"`
	Subsection              int32         `json:"subsection"`
	WinRewards              string        `json:"winRewards"`
}

type TableTplLevelInfo struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplLevelInfo
	result    []TplLevelInfo
}

func NewTableTplLevelInfo(logger *zap.Logger, loadPath string) *TableTplLevelInfo {
	return &TableTplLevelInfo{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplLevelInfo),
		result:    make([]TplLevelInfo, 0),
	}
}

func (t *TableTplLevelInfo) FindByKey(key interface{}) (TplLevelInfo, bool) {
	val, ok := t.tableData[fmt.Sprintf("%v", key)]
	return val, ok
}

func (t *TableTplLevelInfo) FindByFilter(f func(TplLevelInfo) bool) []TplLevelInfo {
	t.result = make([]TplLevelInfo, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplLevelInfo) FindAll() []TplLevelInfo {
	t.result = make([]TplLevelInfo, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplLevelInfo) Release() {
	t.tableData = nil
}

func (t *TableTplLevelInfo) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplLevelInfoMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplLevelInfo.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplLevelInfoMap(fileContent, t.logger)
}

func DeserializeStringToTplLevelInfoMap(jsonStr []byte, logger *zap.Logger) map[string]TplLevelInfo {
	if len(jsonStr) == 0 {
		return make(map[string]TplLevelInfo)
	}
	var jsonMap map[string]TplLevelInfo
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplLevelInfo)
	}
	return jsonMap
}
