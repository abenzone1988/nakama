package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplBoxShop struct {
	Box1        string `json:"Box1"`
	RequiredExp int32  `json:"RequiredExp"`
	ID          string `json:"id"`
	Level       int32  `json:"level"`
}

// ReadOnlyTplBoxShopSlice 只读TplBoxShop切片接口
type ReadOnlyTplBoxShopSlice interface {
	Len() int
	Get(index int) TplBoxShop
	ToSlice() []TplBoxShop
}

// readOnlyTplBoxShopSlice 只读TplBoxShop切片实现
type readOnlyTplBoxShopSlice struct {
	data []TplBoxShop
}

func (r *readOnlyTplBoxShopSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplBoxShopSlice) Get(index int) TplBoxShop {
	if index < 0 || index >= len(r.data) {
		return TplBoxShop{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplBoxShopSlice) ToSlice() []TplBoxShop {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplBoxShop, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplBoxShop struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplBoxShop
}

func NewTableTplBoxShop(logger *zap.Logger, loadPath string) *TableTplBoxShop {
	return &TableTplBoxShop{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplBoxShop),
	}
}

func (t *TableTplBoxShop) FindByKey(key string) (TplBoxShop, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplBoxShop) FindByFilter(f func(TplBoxShop) bool) ReadOnlyTplBoxShopSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplBoxShop, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplBoxShopSlice{data: result}
}

func (t *TableTplBoxShop) FindAll() ReadOnlyTplBoxShopSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplBoxShop, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplBoxShopSlice{data: result}
}

func (t *TableTplBoxShop) Release() {
	t.tableData = nil
}

func (t *TableTplBoxShop) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplBoxShopMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplBoxShop.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplBoxShopMap(fileContent, t.logger)
}

func DeserializeStringToTplBoxShopMap(jsonStr []byte, logger *zap.Logger) map[string]TplBoxShop {
	if len(jsonStr) == 0 {
		return make(map[string]TplBoxShop)
	}
	var jsonMap map[string]TplBoxShop
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplBoxShop)
	}
	return jsonMap
}
