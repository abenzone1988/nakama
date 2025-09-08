package template

import (
	"encoding/json"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplUnlock struct {
	Content string `json:"content"`
	ID      string `json:"id"`
	LevelID string `json:"levelId"`
	Title   string `json:"title"`
}

type TableTplUnlock struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplUnlock
	result    []TplUnlock
}

func NewTableTplUnlock(logger *zap.Logger, loadPath string) *TableTplUnlock {
	return &TableTplUnlock{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplUnlock),
		result:    make([]TplUnlock, 0),
	}
}

func (t *TableTplUnlock) FindByKey(key string) (TplUnlock, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplUnlock) FindByFilter(f func(TplUnlock) bool) []TplUnlock {
	t.result = make([]TplUnlock, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplUnlock) FindAll() []TplUnlock {
	t.result = make([]TplUnlock, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplUnlock) Release() {
	t.tableData = nil
}

func (t *TableTplUnlock) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplUnlockMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplUnlock.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplUnlockMap(fileContent, t.logger)
}

func DeserializeStringToTplUnlockMap(jsonStr []byte, logger *zap.Logger) map[string]TplUnlock {
	if len(jsonStr) == 0 {
		return make(map[string]TplUnlock)
	}
	var jsonMap map[string]TplUnlock
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplUnlock)
	}
	return jsonMap
}
