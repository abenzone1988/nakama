package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplItem struct {
	Gain       []string `json:"gain"`
	IconScale  string   `json:"iconScale"`
	ID         string   `json:"id"`
	ItemType   int32    `json:"itemType"`
	Name       string   `json:"name"`
	OnlyOne    int32    `json:"onlyOne"`
	OrderIndex int32    `json:"orderIndex"`
	Quality    int32    `json:"quality"`
}

// ReadOnlyTplItemSlice 只读TplItem切片接口
type ReadOnlyTplItemSlice interface {
	Len() int
	Get(index int) TplItem
	ToSlice() []TplItem
}

// readOnlyTplItemSlice 只读TplItem切片实现
type readOnlyTplItemSlice struct {
	data []TplItem
}

func (r *readOnlyTplItemSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplItemSlice) Get(index int) TplItem {
	if index < 0 || index >= len(r.data) {
		return TplItem{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplItemSlice) ToSlice() []TplItem {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplItem, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplItem struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplItem
}

func NewTableTplItem(logger *zap.Logger, loadPath string) *TableTplItem {
	return &TableTplItem{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplItem),
	}
}

func (t *TableTplItem) FindByKey(key string) (TplItem, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplItem) FindByFilter(f func(TplItem) bool) ReadOnlyTplItemSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplItem, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplItemSlice{data: result}
}

func (t *TableTplItem) FindAll() ReadOnlyTplItemSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplItem, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplItemSlice{data: result}
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
