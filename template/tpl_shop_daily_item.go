package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplShopDailyItem struct {
	CanAd     int32  `json:"canAd"`
	CanCoin   int32  `json:"canCoin"`
	CanFree   int32  `json:"canFree"`
	CanGem    int32  `json:"canGem"`
	CoinPrice int32  `json:"coinPrice"`
	GemPrice  int32  `json:"gemPrice"`
	ID        string `json:"id"`
	Item      string `json:"item"`
	Num       int32  `json:"num"`
}

// ReadOnlyTplShopDailyItemSlice 只读TplShopDailyItem切片接口
type ReadOnlyTplShopDailyItemSlice interface {
	Len() int
	Get(index int) TplShopDailyItem
	ToSlice() []TplShopDailyItem
}

// readOnlyTplShopDailyItemSlice 只读TplShopDailyItem切片实现
type readOnlyTplShopDailyItemSlice struct {
	data []TplShopDailyItem
}

func (r *readOnlyTplShopDailyItemSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplShopDailyItemSlice) Get(index int) TplShopDailyItem {
	if index < 0 || index >= len(r.data) {
		return TplShopDailyItem{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplShopDailyItemSlice) ToSlice() []TplShopDailyItem {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplShopDailyItem, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplShopDailyItem struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplShopDailyItem
}

func NewTableTplShopDailyItem(logger *zap.Logger, loadPath string) *TableTplShopDailyItem {
	return &TableTplShopDailyItem{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplShopDailyItem),
	}
}

func (t *TableTplShopDailyItem) FindByKey(key string) (TplShopDailyItem, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplShopDailyItem) FindByFilter(f func(TplShopDailyItem) bool) ReadOnlyTplShopDailyItemSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplShopDailyItem, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplShopDailyItemSlice{data: result}
}

func (t *TableTplShopDailyItem) FindAll() ReadOnlyTplShopDailyItemSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplShopDailyItem, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplShopDailyItemSlice{data: result}
}

func (t *TableTplShopDailyItem) Release() {
	t.tableData = nil
}

func (t *TableTplShopDailyItem) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplShopDailyItemMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplShopDailyItem.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplShopDailyItemMap(fileContent, t.logger)
}

func DeserializeStringToTplShopDailyItemMap(jsonStr []byte, logger *zap.Logger) map[string]TplShopDailyItem {
	if len(jsonStr) == 0 {
		return make(map[string]TplShopDailyItem)
	}
	var jsonMap map[string]TplShopDailyItem
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplShopDailyItem)
	}

	return jsonMap
}
