package template

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplMonsterSkill struct {
	ID      string  `json:"id"`
	Param   float64 `json:"param"`
	ResName string  `json:"resName"`
	Type    int32   `json:"type"`
}

type TableTplMonsterSkill struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplMonsterSkill
	result    []TplMonsterSkill
}

func NewTableTplMonsterSkill(logger *zap.Logger, loadPath string) *TableTplMonsterSkill {
	return &TableTplMonsterSkill{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplMonsterSkill),
		result:    make([]TplMonsterSkill, 0),
	}
}

func (t *TableTplMonsterSkill) FindByKey(key interface{}) (TplMonsterSkill, bool) {
	val, ok := t.tableData[fmt.Sprintf("%v", key)]
	return val, ok
}

func (t *TableTplMonsterSkill) FindByFilter(f func(TplMonsterSkill) bool) []TplMonsterSkill {
	t.result = make([]TplMonsterSkill, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplMonsterSkill) FindAll() []TplMonsterSkill {
	t.result = make([]TplMonsterSkill, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplMonsterSkill) Release() {
	t.tableData = nil
}

func (t *TableTplMonsterSkill) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplMonsterSkillMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplMonsterSkill.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplMonsterSkillMap(fileContent, t.logger)
}

func DeserializeStringToTplMonsterSkillMap(jsonStr []byte, logger *zap.Logger) map[string]TplMonsterSkill {
	if len(jsonStr) == 0 {
		return make(map[string]TplMonsterSkill)
	}
	var jsonMap map[string]TplMonsterSkill
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplMonsterSkill)
	}
	return jsonMap
}
