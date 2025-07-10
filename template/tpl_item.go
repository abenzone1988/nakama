package template

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplItem struct {
	Brief    string `json:"brief"`
	ID       string `json:"id"`
	ItemType int32  `json:"itemType"`
	Name     string `json:"name"`
	Quality  int32  `json:"quality"`
	Res      string `json:"res"`
}

type TableTplItem struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplItem
	result    []TplItem
}

func NewTableTplItem(logger *zap.Logger, loadPath string) *TableTplItem {
	return &TableTplItem{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplItem),
		result:    make([]TplItem, 0),
	}
}

func (t *TableTplItem) FindByKey(key interface{}) (TplItem, bool) {
	val, ok := t.tableData[fmt.Sprintf("%v", key)]
	return val, ok
}

func (t *TableTplItem) FindByFilter(f func(TplItem) bool) []TplItem {
	t.result = make([]TplItem, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplItem) FindAll() []TplItem {
	t.result = make([]TplItem, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplItem) Release() {
	t.tableData = nil
}

func (t *TableTplItem) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplItemMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplItem.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplItemMap(fileContent, t.logger)
}

func DeserializeStringToTplItemMap(jsonStr []byte, logger *zap.Logger) map[string]TplItem {
	if len(jsonStr) == 0 {
		return make(map[string]TplItem)
	}
	var jsonMap map[string]TplItem
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplItem)
	}
	return jsonMap
}
