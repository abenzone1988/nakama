package template

import (
	"encoding/json"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplHeroSkill struct {
	Brief string `json:"brief"`
	Icon  string `json:"icon"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Type  int32  `json:"type"`
}

type TableTplHeroSkill struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplHeroSkill
	result    []TplHeroSkill
}

func NewTableTplHeroSkill(logger *zap.Logger, loadPath string) *TableTplHeroSkill {
	return &TableTplHeroSkill{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplHeroSkill),
		result:    make([]TplHeroSkill, 0),
	}
}

func (t *TableTplHeroSkill) FindByKey(key string) (TplHeroSkill, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplHeroSkill) FindByFilter(f func(TplHeroSkill) bool) []TplHeroSkill {
	t.result = make([]TplHeroSkill, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplHeroSkill) FindAll() []TplHeroSkill {
	t.result = make([]TplHeroSkill, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplHeroSkill) Release() {
	t.tableData = nil
}

func (t *TableTplHeroSkill) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplHeroSkillMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplHeroSkill.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplHeroSkillMap(fileContent, t.logger)
}

func DeserializeStringToTplHeroSkillMap(jsonStr []byte, logger *zap.Logger) map[string]TplHeroSkill {
	if len(jsonStr) == 0 {
		return make(map[string]TplHeroSkill)
	}
	var jsonMap map[string]TplHeroSkill
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplHeroSkill)
	}
	return jsonMap
}
