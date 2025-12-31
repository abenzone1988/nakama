package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplCrystalSkinDev struct {
	CostItemInfo string `json:"costItemInfo"`
	ID           string `json:"id"`
	SkinID       string `json:"skinID"`
	StarLevel    int32  `json:"starLevel"`
}

// ReadOnlyTplCrystalSkinDevSlice 只读TplCrystalSkinDev切片接口
type ReadOnlyTplCrystalSkinDevSlice interface {
	Len() int
	Get(index int) TplCrystalSkinDev
	ToSlice() []TplCrystalSkinDev
}

// readOnlyTplCrystalSkinDevSlice 只读TplCrystalSkinDev切片实现
type readOnlyTplCrystalSkinDevSlice struct {
	data []TplCrystalSkinDev
}

func (r *readOnlyTplCrystalSkinDevSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplCrystalSkinDevSlice) Get(index int) TplCrystalSkinDev {
	if index < 0 || index >= len(r.data) {
		return TplCrystalSkinDev{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplCrystalSkinDevSlice) ToSlice() []TplCrystalSkinDev {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplCrystalSkinDev, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplCrystalSkinDev struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplCrystalSkinDev
}

func NewTableTplCrystalSkinDev(logger *zap.Logger, loadPath string) *TableTplCrystalSkinDev {
	return &TableTplCrystalSkinDev{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplCrystalSkinDev),
	}
}

func (t *TableTplCrystalSkinDev) FindByKey(key string) (TplCrystalSkinDev, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplCrystalSkinDev) FindByFilter(f func(TplCrystalSkinDev) bool) ReadOnlyTplCrystalSkinDevSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplCrystalSkinDev, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplCrystalSkinDevSlice{data: result}
}

func (t *TableTplCrystalSkinDev) FindAll() ReadOnlyTplCrystalSkinDevSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplCrystalSkinDev, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplCrystalSkinDevSlice{data: result}
}

func (t *TableTplCrystalSkinDev) Release() {
	t.tableData = nil
}

func (t *TableTplCrystalSkinDev) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplCrystalSkinDevMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplCrystalSkinDev.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplCrystalSkinDevMap(fileContent, t.logger)
}

func DeserializeStringToTplCrystalSkinDevMap(jsonStr []byte, logger *zap.Logger) map[string]TplCrystalSkinDev {
	if len(jsonStr) == 0 {
		return make(map[string]TplCrystalSkinDev)
	}
	var jsonMap map[string]TplCrystalSkinDev
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplCrystalSkinDev)
	}
	return jsonMap
}
