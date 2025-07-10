package template

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplBoxShopItem struct {
	RequiredExp    int32  `json:"RequiredExp"`
	BoxExp         int32  `json:"boxExp"`
	ID             string `json:"id"`
	Level          int32  `json:"level"`
	RefreshTime    int32  `json:"refreshTime"`
	Res            string `json:"res"`
	RetailPrice    int32  `json:"retailPrice"`
	RewardInfo     string `json:"rewardInfo"`
	ShopItemName   string `json:"shopItemName"`
	WholesalePrice int32  `json:"wholesalePrice"`
}

type TableTplBoxShopItem struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplBoxShopItem
	result    []TplBoxShopItem
}

func NewTableTplBoxShopItem(logger *zap.Logger, loadPath string) *TableTplBoxShopItem {
	return &TableTplBoxShopItem{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplBoxShopItem),
		result:    make([]TplBoxShopItem, 0),
	}
}

func (t *TableTplBoxShopItem) FindByKey(key interface{}) (TplBoxShopItem, bool) {
	val, ok := t.tableData[fmt.Sprintf("%v", key)]
	return val, ok
}

func (t *TableTplBoxShopItem) FindByFilter(f func(TplBoxShopItem) bool) []TplBoxShopItem {
	t.result = make([]TplBoxShopItem, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplBoxShopItem) FindAll() []TplBoxShopItem {
	t.result = make([]TplBoxShopItem, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplBoxShopItem) Release() {
	t.tableData = nil
}

func (t *TableTplBoxShopItem) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplBoxShopItemMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplBoxShopItem.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplBoxShopItemMap(fileContent, t.logger)
}

func DeserializeStringToTplBoxShopItemMap(jsonStr []byte, logger *zap.Logger) map[string]TplBoxShopItem {
	if len(jsonStr) == 0 {
		return make(map[string]TplBoxShopItem)
	}
	var jsonMap map[string]TplBoxShopItem
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplBoxShopItem)
	}
	return jsonMap
}
