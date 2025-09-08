package template

import (
	"encoding/json"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplLevelProgress struct {
	BornPosIdx         []int32 `json:"bornPosIdx"`
	Count              int32   `json:"count"`
	HasWarn            int32   `json:"hasWarn"`
	ID                 string  `json:"id"`
	Interval           int32   `json:"interval"`
	MonsterWaveGroupID string  `json:"monsterWaveGroupId"`
	Param              string  `json:"param"`
	Progress           int32   `json:"progress"`
	Time               int32   `json:"time"`
	Type               int32   `json:"type"`
}

type TableTplLevelProgress struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplLevelProgress
	result    []TplLevelProgress
}

func NewTableTplLevelProgress(logger *zap.Logger, loadPath string) *TableTplLevelProgress {
	return &TableTplLevelProgress{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplLevelProgress),
		result:    make([]TplLevelProgress, 0),
	}
}

func (t *TableTplLevelProgress) FindByKey(key string) (TplLevelProgress, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplLevelProgress) FindByFilter(f func(TplLevelProgress) bool) []TplLevelProgress {
	t.result = make([]TplLevelProgress, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplLevelProgress) FindAll() []TplLevelProgress {
	t.result = make([]TplLevelProgress, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplLevelProgress) Release() {
	t.tableData = nil
}

func (t *TableTplLevelProgress) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplLevelProgressMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplLevelProgress.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplLevelProgressMap(fileContent, t.logger)
}

func DeserializeStringToTplLevelProgressMap(jsonStr []byte, logger *zap.Logger) map[string]TplLevelProgress {
	if len(jsonStr) == 0 {
		return make(map[string]TplLevelProgress)
	}
	var jsonMap map[string]TplLevelProgress
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplLevelProgress)
	}
	return jsonMap
}
