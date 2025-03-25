package template

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplRedemption struct {
	Coin   int32  `json:"coin"`
	Coupon int32  `json:"coupon"`
	Expire string `json:"expire"`
	Gem    int32  `json:"gem"`
	ID     string `json:"id"`
	Items  string `json:"items"`
	Name   string `json:"name"`
}

type TableTplRedemption struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplRedemption
	result    []TplRedemption
}

func NewTableTplRedemption(logger *zap.Logger, loadPath string) *TableTplRedemption {
	return &TableTplRedemption{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplRedemption),
		result:    make([]TplRedemption, 0),
	}
}

func (t *TableTplRedemption) FindByKey(key interface{}) (TplRedemption, bool) {
	val, ok := t.tableData[fmt.Sprintf("%v", key)]
	return val, ok
}

func (t *TableTplRedemption) FindByFilter(f func(TplRedemption) bool) []TplRedemption {
	t.result = make([]TplRedemption, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplRedemption) FindAll() []TplRedemption {
	t.result = make([]TplRedemption, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplRedemption) Release() {
	t.tableData = nil
}

func (t *TableTplRedemption) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplRedemptionMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplRedemption.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplRedemptionMap(fileContent, t.logger)
}

func DeserializeStringToTplRedemptionMap(jsonStr []byte, logger *zap.Logger) map[string]TplRedemption {
	if len(jsonStr) == 0 {
		return make(map[string]TplRedemption)
	}
	var jsonMap map[string]TplRedemption
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplRedemption)
	}
	return jsonMap
}
