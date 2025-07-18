package template

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplChallenge struct {
	ActivityID    string `json:"activity_id"`
	CloseTime     string `json:"close_time"`
	EndTime       string `json:"end_time"`
	ID            int32  `json:"id"`
	MaxPart       int32  `json:"max_part"`
	Name          string `json:"name"`
	OpenTime      string `json:"open_time"`
	RewardRemains int32  `json:"reward_remains"`
	Status        int32  `json:"status"`
}

type TableTplChallenge struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplChallenge
	result    []TplChallenge
}

func NewTableTplChallenge(logger *zap.Logger, loadPath string) *TableTplChallenge {
	return &TableTplChallenge{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplChallenge),
		result:    make([]TplChallenge, 0),
	}
}

func (t *TableTplChallenge) FindByKey(key interface{}) (TplChallenge, bool) {
	val, ok := t.tableData[fmt.Sprintf("%v", key)]
	return val, ok
}

func (t *TableTplChallenge) FindByFilter(f func(TplChallenge) bool) []TplChallenge {
	t.result = make([]TplChallenge, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplChallenge) FindAll() []TplChallenge {
	t.result = make([]TplChallenge, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplChallenge) Release() {
	t.tableData = nil
}

func (t *TableTplChallenge) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplChallengeMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplChallenge.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplChallengeMap(fileContent, t.logger)
}

func DeserializeStringToTplChallengeMap(jsonStr []byte, logger *zap.Logger) map[string]TplChallenge {
	if len(jsonStr) == 0 {
		return make(map[string]TplChallenge)
	}
	var jsonMap map[string]TplChallenge
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplChallenge)
	}
	return jsonMap
}
