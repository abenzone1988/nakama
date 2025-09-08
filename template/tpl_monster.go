package template

import (
	"encoding/json"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplMonster struct {
	ActivityScore       int32    `json:"activityScore"`
	Atk                 int32    `json:"atk"`
	AtkRange            int32    `json:"atkRange"`
	AtkSpeed            int32    `json:"atkSpeed"`
	Brief               string   `json:"brief"`
	BulletResist        int32    `json:"bulletResist"`
	CanBeEaten          int32    `json:"canBeEaten"`
	CharacteristicValue string   `json:"characteristicValue"`
	EnergyResist        int32    `json:"energyResist"`
	FaintImmune         int32    `json:"faintImmune"`
	FireResist          int32    `json:"fireResist"`
	Flee                int32    `json:"flee"`
	FreezeImmune        int32    `json:"freezeImmune"`
	GhostHouseGold      int32    `json:"ghostHouseGold"`
	HitbackImmune       int32    `json:"hitbackImmune"`
	Hp                  int32    `json:"hp"`
	IceResist           int32    `json:"iceResist"`
	Icon                string   `json:"icon"`
	ID                  string   `json:"id"`
	IsActiveAtk         int32    `json:"isActiveAtk"`
	LightningResist     int32    `json:"lightningResist"`
	MonsterTag          []string `json:"monsterTag"`
	Name                string   `json:"name"`
	PalsyImmune         int32    `json:"palsyImmune"`
	PhysicalResist      int32    `json:"physicalResist"`
	ResPath             string   `json:"resPath"`
	RewardWeight        int32    `json:"rewardWeight"`
	Scale               float64  `json:"scale"`
	ShellResist         int32    `json:"shellResist"`
	SlowDownImmune      int32    `json:"slowDownImmune"`
	Speed               float64  `json:"speed"`
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

func (t *TableTplMonster) FindByKey(key string) (TplMonster, bool) {
	val, ok := t.tableData[key]
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
