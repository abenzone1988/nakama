package template

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplActivityReward struct {
	ActiveID   string `json:"activeId"`
	ID         string `json:"id"`
	MaxRank    int32  `json:"maxRank"`
	MinRank    int32  `json:"minRank"`
	Name       string `json:"name"`
	RewardIcon string `json:"rewardIcon"`
	RewardID   string `json:"rewardId"`
}

type TableTplActivityReward struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplActivityReward
	result    []TplActivityReward
}

func NewTableTplActivityReward(logger *zap.Logger, loadPath string) *TableTplActivityReward {
	return &TableTplActivityReward{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplActivityReward),
		result:    make([]TplActivityReward, 0),
	}
}

func (t *TableTplActivityReward) FindByKey(key interface{}) (TplActivityReward, bool) {
	val, ok := t.tableData[fmt.Sprintf("%v", key)]
	return val, ok
}

func (t *TableTplActivityReward) FindByFilter(f func(TplActivityReward) bool) []TplActivityReward {
	t.result = make([]TplActivityReward, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplActivityReward) FindAll() []TplActivityReward {
	t.result = make([]TplActivityReward, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplActivityReward) Release() {
	t.tableData = nil
}

func (t *TableTplActivityReward) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplActivityRewardMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplActivityReward.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplActivityRewardMap(fileContent, t.logger)
}

func DeserializeStringToTplActivityRewardMap(jsonStr []byte, logger *zap.Logger) map[string]TplActivityReward {
	if len(jsonStr) == 0 {
		return make(map[string]TplActivityReward)
	}
	var jsonMap map[string]TplActivityReward
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplActivityReward)
	}
	return jsonMap
}
