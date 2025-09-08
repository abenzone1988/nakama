package template

import (
	"encoding/json"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplStarterPack struct {
	Image    int32  `json:"Image"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Reward   string `json:"reward"`
	TaskType int32  `json:"taskType"`
}

type TableTplStarterPack struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplStarterPack
	result    []TplStarterPack
}

func NewTableTplStarterPack(logger *zap.Logger, loadPath string) *TableTplStarterPack {
	return &TableTplStarterPack{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplStarterPack),
		result:    make([]TplStarterPack, 0),
	}
}

func (t *TableTplStarterPack) FindByKey(key string) (TplStarterPack, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplStarterPack) FindByFilter(f func(TplStarterPack) bool) []TplStarterPack {
	t.result = make([]TplStarterPack, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplStarterPack) FindAll() []TplStarterPack {
	t.result = make([]TplStarterPack, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplStarterPack) Release() {
	t.tableData = nil
}

func (t *TableTplStarterPack) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplStarterPackMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplStarterPack.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplStarterPackMap(fileContent, t.logger)
}

func DeserializeStringToTplStarterPackMap(jsonStr []byte, logger *zap.Logger) map[string]TplStarterPack {
	if len(jsonStr) == 0 {
		return make(map[string]TplStarterPack)
	}
	var jsonMap map[string]TplStarterPack
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplStarterPack)
	}
	return jsonMap
}
