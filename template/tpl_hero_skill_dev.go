package template

import (
	"encoding/json"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplHeroSkillDev struct {
	ID      int32  `json:"id"`
	Lv      int32  `json:"lv"`
	Note    string `json:"note"`
	P1      string `json:"p1"`
	P2      string `json:"p2"`
	P3      string `json:"p3"`
	SkillID string `json:"skillId"`
	V1      int32  `json:"v1"`
	V2      int32  `json:"v2"`
	V3      int32  `json:"v3"`
}

type TableTplHeroSkillDev struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplHeroSkillDev
	result    []TplHeroSkillDev
}

func NewTableTplHeroSkillDev(logger *zap.Logger, loadPath string) *TableTplHeroSkillDev {
	return &TableTplHeroSkillDev{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplHeroSkillDev),
		result:    make([]TplHeroSkillDev, 0),
	}
}

func (t *TableTplHeroSkillDev) FindByKey(key string) (TplHeroSkillDev, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplHeroSkillDev) FindByFilter(f func(TplHeroSkillDev) bool) []TplHeroSkillDev {
	t.result = make([]TplHeroSkillDev, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplHeroSkillDev) FindAll() []TplHeroSkillDev {
	t.result = make([]TplHeroSkillDev, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplHeroSkillDev) Release() {
	t.tableData = nil
}

func (t *TableTplHeroSkillDev) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplHeroSkillDevMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplHeroSkillDev.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplHeroSkillDevMap(fileContent, t.logger)
}

func DeserializeStringToTplHeroSkillDevMap(jsonStr []byte, logger *zap.Logger) map[string]TplHeroSkillDev {
	if len(jsonStr) == 0 {
		return make(map[string]TplHeroSkillDev)
	}
	var jsonMap map[string]TplHeroSkillDev
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplHeroSkillDev)
	}
	return jsonMap
}
