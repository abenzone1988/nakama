package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplShopChapterItem struct {
	Brief   string `json:"brief"`
	Count   int32  `json:"count"`
	ID      string `json:"id"`
	LevelID string `json:"levelId"`
	Name    string `json:"name"`
	Price   int32  `json:"price"`
	Reward  string `json:"reward"`
}

// ReadOnlyTplShopChapterItemSlice 只读TplShopChapterItem切片接口
type ReadOnlyTplShopChapterItemSlice interface {
	Len() int
	Get(index int) TplShopChapterItem
	ToSlice() []TplShopChapterItem
}

// readOnlyTplShopChapterItemSlice 只读TplShopChapterItem切片实现
type readOnlyTplShopChapterItemSlice struct {
	data []TplShopChapterItem
}

func (r *readOnlyTplShopChapterItemSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplShopChapterItemSlice) Get(index int) TplShopChapterItem {
	if index < 0 || index >= len(r.data) {
		return TplShopChapterItem{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplShopChapterItemSlice) ToSlice() []TplShopChapterItem {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplShopChapterItem, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplShopChapterItem struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplShopChapterItem
}

func NewTableTplShopChapterItem(logger *zap.Logger, loadPath string) *TableTplShopChapterItem {
	return &TableTplShopChapterItem{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplShopChapterItem),
	}
}

func (t *TableTplShopChapterItem) FindByKey(key string) (TplShopChapterItem, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplShopChapterItem) FindByFilter(f func(TplShopChapterItem) bool) ReadOnlyTplShopChapterItemSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplShopChapterItem, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplShopChapterItemSlice{data: result}
}

func (t *TableTplShopChapterItem) FindAll() ReadOnlyTplShopChapterItemSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplShopChapterItem, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplShopChapterItemSlice{data: result}
}

func (t *TableTplShopChapterItem) Release() {
	t.tableData = nil
}

func (t *TableTplShopChapterItem) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplShopChapterItemMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplShopChapterItem.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplShopChapterItemMap(fileContent, t.logger)
}

func DeserializeStringToTplShopChapterItemMap(jsonStr []byte, logger *zap.Logger) map[string]TplShopChapterItem {
	if len(jsonStr) == 0 {
		return make(map[string]TplShopChapterItem)
	}
	var jsonMap map[string]TplShopChapterItem
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplShopChapterItem)
	}
	return jsonMap
}
