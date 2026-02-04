package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplBoxShopItem struct {
	BoxExp         int32  `json:"boxExp"`
	FixedReward    string `json:"fixedReward"`
	ID             string `json:"id"`
	Level          int32  `json:"level"`
	PayType        string `json:"payType"`
	RandomReward   string `json:"randomReward"`
	Res            string `json:"res"`
	RetailPrice    int32  `json:"retailPrice"`
	ShopItemName   string `json:"shopItemName"`
	WholesalePrice int32  `json:"wholesalePrice"`
}

// ReadOnlyTplBoxShopItemSlice 只读TplBoxShopItem切片接口
type ReadOnlyTplBoxShopItemSlice interface {
	Len() int
	Get(index int) TplBoxShopItem
	ToSlice() []TplBoxShopItem
}

// readOnlyTplBoxShopItemSlice 只读TplBoxShopItem切片实现
type readOnlyTplBoxShopItemSlice struct {
	data []TplBoxShopItem
}

func (r *readOnlyTplBoxShopItemSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplBoxShopItemSlice) Get(index int) TplBoxShopItem {
	if index < 0 || index >= len(r.data) {
		return TplBoxShopItem{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplBoxShopItemSlice) ToSlice() []TplBoxShopItem {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplBoxShopItem, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplBoxShopItem struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplBoxShopItem
}

func NewTableTplBoxShopItem(logger *zap.Logger, loadPath string) *TableTplBoxShopItem {
	return &TableTplBoxShopItem{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplBoxShopItem),
	}
}

func (t *TableTplBoxShopItem) FindByKey(key string) (TplBoxShopItem, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplBoxShopItem) FindByFilter(f func(TplBoxShopItem) bool) ReadOnlyTplBoxShopItemSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplBoxShopItem, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplBoxShopItemSlice{data: result}
}

func (t *TableTplBoxShopItem) FindAll() ReadOnlyTplBoxShopItemSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplBoxShopItem, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplBoxShopItemSlice{data: result}
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
