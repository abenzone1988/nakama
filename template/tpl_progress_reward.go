package template

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplProgressReward struct {
	ID        string `json:"id"`
	NeedValue int32  `json:"needValue"`
	Reward    string `json:"reward"`
	Type      int32  `json:"type"`
}

type TableTplProgressReward struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplProgressReward
	result    []TplProgressReward
}

func NewTableTplProgressReward(logger *zap.Logger, loadPath string) *TableTplProgressReward {
	return &TableTplProgressReward{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplProgressReward),
		result:    make([]TplProgressReward, 0),
	}
}

func (t *TableTplProgressReward) FindByKey(key interface{}) (TplProgressReward, bool) {
	val, ok := t.tableData[fmt.Sprintf("%v", key)]
	return val, ok
}

func (t *TableTplProgressReward) FindByFilter(f func(TplProgressReward) bool) []TplProgressReward {
	t.result = make([]TplProgressReward, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplProgressReward) FindAll() []TplProgressReward {
	t.result = make([]TplProgressReward, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplProgressReward) Release() {
	t.tableData = nil
}

func (t *TableTplProgressReward) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplProgressRewardMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplProgressReward.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplProgressRewardMap(fileContent, t.logger)
}

func DeserializeStringToTplProgressRewardMap(jsonStr []byte, logger *zap.Logger) map[string]TplProgressReward {
	if len(jsonStr) == 0 {
		return make(map[string]TplProgressReward)
	}
	var jsonMap map[string]TplProgressReward
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplProgressReward)
	}
	return jsonMap
}
