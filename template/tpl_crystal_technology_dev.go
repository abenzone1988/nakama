package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplCrystalTechnologyDev struct {
	CostItemID  string `json:"costItemID"`
	CostItemNum int32  `json:"costItemNum"`
	CostPayType int32  `json:"costPayType"`
	ID          string `json:"id"`
	Price       int32  `json:"price"`
}

// ReadOnlyTplCrystalTechnologyDevSlice 只读TplCrystalTechnologyDev切片接口
type ReadOnlyTplCrystalTechnologyDevSlice interface {
	Len() int
	Get(index int) TplCrystalTechnologyDev
	ToSlice() []TplCrystalTechnologyDev
}

// readOnlyTplCrystalTechnologyDevSlice 只读TplCrystalTechnologyDev切片实现
type readOnlyTplCrystalTechnologyDevSlice struct {
	data []TplCrystalTechnologyDev
}

func (r *readOnlyTplCrystalTechnologyDevSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplCrystalTechnologyDevSlice) Get(index int) TplCrystalTechnologyDev {
	if index < 0 || index >= len(r.data) {
		return TplCrystalTechnologyDev{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplCrystalTechnologyDevSlice) ToSlice() []TplCrystalTechnologyDev {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplCrystalTechnologyDev, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplCrystalTechnologyDev struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplCrystalTechnologyDev
}

func NewTableTplCrystalTechnologyDev(logger *zap.Logger, loadPath string) *TableTplCrystalTechnologyDev {
	return &TableTplCrystalTechnologyDev{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplCrystalTechnologyDev),
	}
}

func (t *TableTplCrystalTechnologyDev) FindByKey(key string) (TplCrystalTechnologyDev, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplCrystalTechnologyDev) FindByFilter(f func(TplCrystalTechnologyDev) bool) ReadOnlyTplCrystalTechnologyDevSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplCrystalTechnologyDev, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplCrystalTechnologyDevSlice{data: result}
}

func (t *TableTplCrystalTechnologyDev) FindAll() ReadOnlyTplCrystalTechnologyDevSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplCrystalTechnologyDev, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplCrystalTechnologyDevSlice{data: result}
}

func (t *TableTplCrystalTechnologyDev) Release() {
	t.tableData = nil
}

func (t *TableTplCrystalTechnologyDev) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplCrystalTechnologyDevMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplCrystalTechnologyDev.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplCrystalTechnologyDevMap(fileContent, t.logger)
}

func DeserializeStringToTplCrystalTechnologyDevMap(jsonStr []byte, logger *zap.Logger) map[string]TplCrystalTechnologyDev {
	if len(jsonStr) == 0 {
		return make(map[string]TplCrystalTechnologyDev)
	}
	var jsonMap map[string]TplCrystalTechnologyDev
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplCrystalTechnologyDev)
	}
	return jsonMap
}
