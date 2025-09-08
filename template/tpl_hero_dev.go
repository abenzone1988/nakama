package template

import (
	"encoding/json"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplHeroDev struct {
	CostDebris       int32  `json:"costDebris"`
	HeroID           string `json:"heroId"`
	ID               string `json:"id"`
	Level            int32  `json:"level"`
	P1               string `json:"p1"`
	UpgradeSkillType int32  `json:"upgradeSkillType"`
	V1               int32  `json:"v1"`
}

type TableTplHeroDev struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplHeroDev
	result    []TplHeroDev
}

func NewTableTplHeroDev(logger *zap.Logger, loadPath string) *TableTplHeroDev {
	return &TableTplHeroDev{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplHeroDev),
		result:    make([]TplHeroDev, 0),
	}
}

func (t *TableTplHeroDev) FindByKey(key string) (TplHeroDev, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplHeroDev) FindByFilter(f func(TplHeroDev) bool) []TplHeroDev {
	t.result = make([]TplHeroDev, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplHeroDev) FindAll() []TplHeroDev {
	t.result = make([]TplHeroDev, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplHeroDev) Release() {
	t.tableData = nil
}

func (t *TableTplHeroDev) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplHeroDevMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplHeroDev.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplHeroDevMap(fileContent, t.logger)
}

func DeserializeStringToTplHeroDevMap(jsonStr []byte, logger *zap.Logger) map[string]TplHeroDev {
	if len(jsonStr) == 0 {
		return make(map[string]TplHeroDev)
	}
	var jsonMap map[string]TplHeroDev
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplHeroDev)
	}
	return jsonMap
}
