package template

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplMonster struct {
	ActivityScore       int32    `json:"activityScore"`
	Atk                 int32    `json:"atk"`
	AtkRange            int32    `json:"atkRange"`
	AtkSpeed            float64  `json:"atkSpeed"`
	Brief               string   `json:"brief"`
	CharacteristicValue string   `json:"characteristicValue"`
	FaintImmune         int32    `json:"faintImmune"`
	Flee                int32    `json:"flee"`
	FreezeImmune        int32    `json:"freezeImmune"`
	GhostHouseGold      int32    `json:"ghostHouseGold"`
	HitbackImmune       int32    `json:"hitbackImmune"`
	Hp                  int32    `json:"hp"`
	Icon                string   `json:"icon"`
	ID                  string   `json:"id"`
	IsActiveAtk         int32    `json:"isActiveAtk"`
	MonsterTag          []string `json:"monsterTag"`
	Name                string   `json:"name"`
	PalsyImmune         int32    `json:"palsyImmune"`
	ResPath             string   `json:"resPath"`
	RewardWeight        int32    `json:"rewardWeight"`
	Scale               int32    `json:"scale"`
	SlowDownImmune      int32    `json:"slowDownImmune"`
	Speed               int32    `json:"speed"`
	Type                int32    `json:"type"`
	UnitType            int32    `json:"unitType"`
	WayPathType         int32    `json:"wayPathType"`
}

type TableTplMonster struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplMonster
	result    []TplMonster
}

func NewTableTplMonster(logger *zap.Logger, loadPath string) *TableTplMonster {
	return &TableTplMonster{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplMonster),
		result:    make([]TplMonster, 0),
	}
}

func (t *TableTplMonster) FindByKey(key interface{}) (TplMonster, bool) {
	val, ok := t.tableData[fmt.Sprintf("%v", key)]
	return val, ok
}

func (t *TableTplMonster) FindByFilter(f func(TplMonster) bool) []TplMonster {
	t.result = make([]TplMonster, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplMonster) FindAll() []TplMonster {
	t.result = make([]TplMonster, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplMonster) Release() {
	t.tableData = nil
}

func (t *TableTplMonster) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplMonsterMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplMonster.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplMonsterMap(fileContent, t.logger)
}

func DeserializeStringToTplMonsterMap(jsonStr []byte, logger *zap.Logger) map[string]TplMonster {
	if len(jsonStr) == 0 {
		return make(map[string]TplMonster)
	}
	var jsonMap map[string]TplMonster
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplMonster)
	}
	return jsonMap
}
