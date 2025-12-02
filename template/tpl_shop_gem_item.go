package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplShopGemItem struct {
	ExtraNum int32  `json:"extraNum"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Num      int32  `json:"num"`
	PayID    string `json:"payId"`
	Res      string `json:"res"`
}

// ReadOnlyTplShopGemItemSlice 只读TplShopGemItem切片接口
type ReadOnlyTplShopGemItemSlice interface {
	Len() int
	Get(index int) TplShopGemItem
	ToSlice() []TplShopGemItem
}

// readOnlyTplShopGemItemSlice 只读TplShopGemItem切片实现
type readOnlyTplShopGemItemSlice struct {
	data []TplShopGemItem
}

func (r *readOnlyTplShopGemItemSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplShopGemItemSlice) Get(index int) TplShopGemItem {
	if index < 0 || index >= len(r.data) {
		return TplShopGemItem{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplShopGemItemSlice) ToSlice() []TplShopGemItem {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplShopGemItem, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplShopGemItem struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplShopGemItem
}

func NewTableTplShopGemItem(logger *zap.Logger, loadPath string) *TableTplShopGemItem {
	return &TableTplShopGemItem{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplShopGemItem),
	}
}

func (t *TableTplShopGemItem) FindByKey(key string) (TplShopGemItem, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplShopGemItem) FindByFilter(f func(TplShopGemItem) bool) ReadOnlyTplShopGemItemSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplShopGemItem, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplShopGemItemSlice{data: result}
}

func (t *TableTplShopGemItem) FindAll() ReadOnlyTplShopGemItemSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplShopGemItem, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplShopGemItemSlice{data: result}
}

func (t *TableTplShopGemItem) Release() {
	t.tableData = nil
}

func (t *TableTplShopGemItem) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplShopGemItemMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplShopGemItem.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplShopGemItemMap(fileContent, t.logger)
}

func DeserializeStringToTplShopGemItemMap(jsonStr []byte, logger *zap.Logger) map[string]TplShopGemItem {
	if len(jsonStr) == 0 {
		return make(map[string]TplShopGemItem)
	}
	var jsonMap map[string]TplShopGemItem
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplShopGemItem)
	}
	return jsonMap
}
