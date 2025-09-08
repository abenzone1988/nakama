package template

import (
	"encoding/json"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplBattleWave struct {
	GroupID  string `json:"groupId"`
	ID       int32  `json:"id"`
	LvUp     int32  `json:"lvUp"`
	Money    int32  `json:"money"`
	Progress int32  `json:"progress"`
}

type TableTplBattleWave struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplBattleWave
	result    []TplBattleWave
}

func NewTableTplBattleWave(logger *zap.Logger, loadPath string) *TableTplBattleWave {
	return &TableTplBattleWave{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplBattleWave),
		result:    make([]TplBattleWave, 0),
	}
}

func (t *TableTplBattleWave) FindByKey(key string) (TplBattleWave, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplBattleWave) FindByFilter(f func(TplBattleWave) bool) []TplBattleWave {
	t.result = make([]TplBattleWave, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplBattleWave) FindAll() []TplBattleWave {
	t.result = make([]TplBattleWave, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplBattleWave) Release() {
	t.tableData = nil
}

func (t *TableTplBattleWave) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplBattleWaveMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplBattleWave.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplBattleWaveMap(fileContent, t.logger)
}

func DeserializeStringToTplBattleWaveMap(jsonStr []byte, logger *zap.Logger) map[string]TplBattleWave {
	if len(jsonStr) == 0 {
		return make(map[string]TplBattleWave)
	}
	var jsonMap map[string]TplBattleWave
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplBattleWave)
	}
	return jsonMap
}
