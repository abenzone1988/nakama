package template

import (
	"encoding/json"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplDailySignIn struct {
	Group  int32  `json:"group"`
	ID     string `json:"id"`
	Name   string `json:"name"`
	Reward string `json:"reward"`
}

type TableTplDailySignIn struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplDailySignIn
	result    []TplDailySignIn
}

func NewTableTplDailySignIn(logger *zap.Logger, loadPath string) *TableTplDailySignIn {
	return &TableTplDailySignIn{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplDailySignIn),
		result:    make([]TplDailySignIn, 0),
	}
}

func (t *TableTplDailySignIn) FindByKey(key string) (TplDailySignIn, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplDailySignIn) FindByFilter(f func(TplDailySignIn) bool) []TplDailySignIn {
	t.result = make([]TplDailySignIn, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplDailySignIn) FindAll() []TplDailySignIn {
	t.result = make([]TplDailySignIn, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplDailySignIn) Release() {
	t.tableData = nil
}

func (t *TableTplDailySignIn) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplDailySignInMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplDailySignIn.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplDailySignInMap(fileContent, t.logger)
}

func DeserializeStringToTplDailySignInMap(jsonStr []byte, logger *zap.Logger) map[string]TplDailySignIn {
	if len(jsonStr) == 0 {
		return make(map[string]TplDailySignIn)
	}
	var jsonMap map[string]TplDailySignIn
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplDailySignIn)
	}
	return jsonMap
}
