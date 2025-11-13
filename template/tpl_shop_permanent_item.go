package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplShopPermanentItem struct {
	ID           string  `json:"id"`
	Item         string  `json:"item"`
	LimitTimes   []int32 `json:"limitTimes"`
	Num          int32   `json:"num"`
	PayType      string  `json:"payType"`
	RefreshTime  int32   `json:"refreshTime"`
	Res          string  `json:"res"`
	SalePrice    []int32 `json:"salePrice"`
	ShopItemName string  `json:"shopItemName"`
}

// ReadOnlyTplShopPermanentItemSlice 只读TplShopPermanentItem切片接口
type ReadOnlyTplShopPermanentItemSlice interface {
	Len() int
	Get(index int) TplShopPermanentItem
	ToSlice() []TplShopPermanentItem
}

// readOnlyTplShopPermanentItemSlice 只读TplShopPermanentItem切片实现
type readOnlyTplShopPermanentItemSlice struct {
	data []TplShopPermanentItem
}

func (r *readOnlyTplShopPermanentItemSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplShopPermanentItemSlice) Get(index int) TplShopPermanentItem {
	if index < 0 || index >= len(r.data) {
		return TplShopPermanentItem{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplShopPermanentItemSlice) ToSlice() []TplShopPermanentItem {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplShopPermanentItem, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplShopPermanentItem struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplShopPermanentItem
}

func NewTableTplShopPermanentItem(logger *zap.Logger, loadPath string) *TableTplShopPermanentItem {
	return &TableTplShopPermanentItem{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplShopPermanentItem),
	}
}

func (t *TableTplShopPermanentItem) FindByKey(key string) (TplShopPermanentItem, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplShopPermanentItem) FindByFilter(f func(TplShopPermanentItem) bool) ReadOnlyTplShopPermanentItemSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplShopPermanentItem, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplShopPermanentItemSlice{data: result}
}

func (t *TableTplShopPermanentItem) FindAll() ReadOnlyTplShopPermanentItemSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplShopPermanentItem, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplShopPermanentItemSlice{data: result}
}

func (t *TableTplShopPermanentItem) Release() {
	t.tableData = nil
}

func (t *TableTplShopPermanentItem) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplShopPermanentItemMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplShopPermanentItem.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplShopPermanentItemMap(fileContent, t.logger)
}

func DeserializeStringToTplShopPermanentItemMap(jsonStr []byte, logger *zap.Logger) map[string]TplShopPermanentItem {
	if len(jsonStr) == 0 {
		return make(map[string]TplShopPermanentItem)
	}
	var jsonMap map[string]TplShopPermanentItem
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplShopPermanentItem)
	}
	return jsonMap
}
