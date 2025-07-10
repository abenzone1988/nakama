package template

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplHeroBase struct {
	HeroID    string `json:"heroId"`
	ID        int32  `json:"id"`
	Note      string `json:"note"`
	Param     int32  `json:"param"`
	ShowTitle string `json:"showTitle"`
	Type      string `json:"type"`
}

type TableTplHeroBase struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplHeroBase
	result    []TplHeroBase
}

func NewTableTplHeroBase(logger *zap.Logger, loadPath string) *TableTplHeroBase {
	return &TableTplHeroBase{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplHeroBase),
		result:    make([]TplHeroBase, 0),
	}
}

func (t *TableTplHeroBase) FindByKey(key interface{}) (TplHeroBase, bool) {
	val, ok := t.tableData[fmt.Sprintf("%v", key)]
	return val, ok
}

func (t *TableTplHeroBase) FindByFilter(f func(TplHeroBase) bool) []TplHeroBase {
	t.result = make([]TplHeroBase, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplHeroBase) FindAll() []TplHeroBase {
	t.result = make([]TplHeroBase, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplHeroBase) Release() {
	t.tableData = nil
}

func (t *TableTplHeroBase) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplHeroBaseMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplHeroBase.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplHeroBaseMap(fileContent, t.logger)
}

func DeserializeStringToTplHeroBaseMap(jsonStr []byte, logger *zap.Logger) map[string]TplHeroBase {
	if len(jsonStr) == 0 {
		return make(map[string]TplHeroBase)
	}
	var jsonMap map[string]TplHeroBase
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplHeroBase)
	}
	return jsonMap
}
