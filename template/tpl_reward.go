package template

import (
	"encoding/json"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplReward struct {
	Coin   int32  `json:"coin"`
	Coupon int32  `json:"coupon"`
	Gem    int32  `json:"gem"`
	ID     string `json:"id"`
	Items  string `json:"items"`
	Name   string `json:"name"`
}

type TableTplReward struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplReward
	result    []TplReward
}

func NewTableTplReward(logger *zap.Logger, loadPath string) *TableTplReward {
	return &TableTplReward{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplReward),
		result:    make([]TplReward, 0),
	}
}

func (t *TableTplReward) FindByKey(key string) (TplReward, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplReward) FindByFilter(f func(TplReward) bool) []TplReward {
	t.result = make([]TplReward, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplReward) FindAll() []TplReward {
	t.result = make([]TplReward, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplReward) Release() {
	t.tableData = nil
}

func (t *TableTplReward) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplRewardMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplReward.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplRewardMap(fileContent, t.logger)
}

func DeserializeStringToTplRewardMap(jsonStr []byte, logger *zap.Logger) map[string]TplReward {
	if len(jsonStr) == 0 {
		return make(map[string]TplReward)
	}
	var jsonMap map[string]TplReward
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplReward)
	}
	return jsonMap
}
