package template

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplHero struct {
	Brief          string `json:"brief"`
	ComboSkill     string `json:"comboSkill"`
	DebrisID       string `json:"debrisId"`
	DynamicsUIPath string `json:"dynamicsUiPath"`
	ID             string `json:"id"`
	Name           string `json:"name"`
	PrefabPath     string `json:"prefabPath"`
	Skill0         string `json:"skill0"`
	Skill1         string `json:"skill1"`
	Skill2         string `json:"skill2"`
	TargetType     int32  `json:"targetType"`
	UIPath         string `json:"uiPath"`
}

type TableTplHero struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplHero
	result    []TplHero
}

func NewTableTplHero(logger *zap.Logger, loadPath string) *TableTplHero {
	return &TableTplHero{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplHero),
		result:    make([]TplHero, 0),
	}
}

func (t *TableTplHero) FindByKey(key interface{}) (TplHero, bool) {
	val, ok := t.tableData[fmt.Sprintf("%v", key)]
	return val, ok
}

func (t *TableTplHero) FindByFilter(f func(TplHero) bool) []TplHero {
	t.result = make([]TplHero, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplHero) FindAll() []TplHero {
	t.result = make([]TplHero, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplHero) Release() {
	t.tableData = nil
}

func (t *TableTplHero) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplHeroMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplHero.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplHeroMap(fileContent, t.logger)
}

func DeserializeStringToTplHeroMap(jsonStr []byte, logger *zap.Logger) map[string]TplHero {
	if len(jsonStr) == 0 {
		return make(map[string]TplHero)
	}
	var jsonMap map[string]TplHero
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplHero)
	}
	return jsonMap
}
