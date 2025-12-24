package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplCrystalEquipmentDev struct {
	EquipType int32  `json:"equipType"`
	ID        string `json:"id"`
}

// ReadOnlyTplCrystalEquipmentDevSlice 只读TplCrystalEquipmentDev切片接口
type ReadOnlyTplCrystalEquipmentDevSlice interface {
	Len() int
	Get(index int) TplCrystalEquipmentDev
	ToSlice() []TplCrystalEquipmentDev
}

// readOnlyTplCrystalEquipmentDevSlice 只读TplCrystalEquipmentDev切片实现
type readOnlyTplCrystalEquipmentDevSlice struct {
	data []TplCrystalEquipmentDev
}

func (r *readOnlyTplCrystalEquipmentDevSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplCrystalEquipmentDevSlice) Get(index int) TplCrystalEquipmentDev {
	if index < 0 || index >= len(r.data) {
		return TplCrystalEquipmentDev{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplCrystalEquipmentDevSlice) ToSlice() []TplCrystalEquipmentDev {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplCrystalEquipmentDev, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplCrystalEquipmentDev struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplCrystalEquipmentDev
}

func NewTableTplCrystalEquipmentDev(logger *zap.Logger, loadPath string) *TableTplCrystalEquipmentDev {
	return &TableTplCrystalEquipmentDev{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplCrystalEquipmentDev),
	}
}

func (t *TableTplCrystalEquipmentDev) FindByKey(key string) (TplCrystalEquipmentDev, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplCrystalEquipmentDev) FindByFilter(f func(TplCrystalEquipmentDev) bool) ReadOnlyTplCrystalEquipmentDevSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplCrystalEquipmentDev, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplCrystalEquipmentDevSlice{data: result}
}

func (t *TableTplCrystalEquipmentDev) FindAll() ReadOnlyTplCrystalEquipmentDevSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplCrystalEquipmentDev, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplCrystalEquipmentDevSlice{data: result}
}

func (t *TableTplCrystalEquipmentDev) Release() {
	t.tableData = nil
}

func (t *TableTplCrystalEquipmentDev) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplCrystalEquipmentDevMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplCrystalEquipmentDev.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplCrystalEquipmentDevMap(fileContent, t.logger)
}

func DeserializeStringToTplCrystalEquipmentDevMap(jsonStr []byte, logger *zap.Logger) map[string]TplCrystalEquipmentDev {
	if len(jsonStr) == 0 {
		return make(map[string]TplCrystalEquipmentDev)
	}
	var jsonMap map[string]TplCrystalEquipmentDev
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplCrystalEquipmentDev)
	}
	return jsonMap
}
