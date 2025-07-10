package template

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplShopEx struct {
	BuyCount0 int32  `json:"buyCount0"`
	BuyCount1 int32  `json:"buyCount1"`
	BuyCount2 int32  `json:"buyCount2"`
	BuyType0  int32  `json:"buyType0"`
	BuyType1  int32  `json:"buyType1"`
	BuyType2  int32  `json:"buyType2"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	Price0    int32  `json:"price0"`
	Price1    int32  `json:"price1"`
	Price2    int32  `json:"price2"`
	RewardID  string `json:"rewardId"`
	ShopType  int32  `json:"shopType"`
}

type TableTplShopEx struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplShopEx
	result    []TplShopEx
}

func NewTableTplShopEx(logger *zap.Logger, loadPath string) *TableTplShopEx {
	return &TableTplShopEx{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplShopEx),
		result:    make([]TplShopEx, 0),
	}
}

func (t *TableTplShopEx) FindByKey(key interface{}) (TplShopEx, bool) {
	val, ok := t.tableData[fmt.Sprintf("%v", key)]
	return val, ok
}

func (t *TableTplShopEx) FindByFilter(f func(TplShopEx) bool) []TplShopEx {
	t.result = make([]TplShopEx, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplShopEx) FindAll() []TplShopEx {
	t.result = make([]TplShopEx, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplShopEx) Release() {
	t.tableData = nil
}

func (t *TableTplShopEx) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplShopExMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplShopEx.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplShopExMap(fileContent, t.logger)
}

func DeserializeStringToTplShopExMap(jsonStr []byte, logger *zap.Logger) map[string]TplShopEx {
	if len(jsonStr) == 0 {
		return make(map[string]TplShopEx)
	}
	var jsonMap map[string]TplShopEx
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplShopEx)
	}
	return jsonMap
}
