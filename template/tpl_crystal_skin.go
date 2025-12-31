package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplCrystalSkin struct {
	ID              string `json:"id"`
	UnlockCostItems string `json:"unlockCostItems"`
}

// ReadOnlyTplCrystalSkinSlice 只读TplCrystalSkin切片接口
type ReadOnlyTplCrystalSkinSlice interface {
	Len() int
	Get(index int) TplCrystalSkin
	ToSlice() []TplCrystalSkin
}

// readOnlyTplCrystalSkinSlice 只读TplCrystalSkin切片实现
type readOnlyTplCrystalSkinSlice struct {
	data []TplCrystalSkin
}

func (r *readOnlyTplCrystalSkinSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplCrystalSkinSlice) Get(index int) TplCrystalSkin {
	if index < 0 || index >= len(r.data) {
		return TplCrystalSkin{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplCrystalSkinSlice) ToSlice() []TplCrystalSkin {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplCrystalSkin, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplCrystalSkin struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplCrystalSkin
}

func NewTableTplCrystalSkin(logger *zap.Logger, loadPath string) *TableTplCrystalSkin {
	return &TableTplCrystalSkin{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplCrystalSkin),
	}
}

func (t *TableTplCrystalSkin) FindByKey(key string) (TplCrystalSkin, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplCrystalSkin) FindByFilter(f func(TplCrystalSkin) bool) ReadOnlyTplCrystalSkinSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplCrystalSkin, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplCrystalSkinSlice{data: result}
}

func (t *TableTplCrystalSkin) FindAll() ReadOnlyTplCrystalSkinSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplCrystalSkin, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplCrystalSkinSlice{data: result}
}

func (t *TableTplCrystalSkin) Release() {
	t.tableData = nil
}

func (t *TableTplCrystalSkin) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplCrystalSkinMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplCrystalSkin.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplCrystalSkinMap(fileContent, t.logger)
}

func DeserializeStringToTplCrystalSkinMap(jsonStr []byte, logger *zap.Logger) map[string]TplCrystalSkin {
	if len(jsonStr) == 0 {
		return make(map[string]TplCrystalSkin)
	}
	var jsonMap map[string]TplCrystalSkin
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplCrystalSkin)
	}
	return jsonMap
}
