package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplCrystalEquipmentSlotBreakDev struct {
	EquipType int32  `json:"equipType"`
	ID        string `json:"id"`
	Level     int32  `json:"level"`
}

// ReadOnlyTplCrystalEquipmentSlotBreakDevSlice 只读TplCrystalEquipmentSlotBreakDev切片接口
type ReadOnlyTplCrystalEquipmentSlotBreakDevSlice interface {
	Len() int
	Get(index int) TplCrystalEquipmentSlotBreakDev
	ToSlice() []TplCrystalEquipmentSlotBreakDev
}

// readOnlyTplCrystalEquipmentSlotBreakDevSlice 只读TplCrystalEquipmentSlotBreakDev切片实现
type readOnlyTplCrystalEquipmentSlotBreakDevSlice struct {
	data []TplCrystalEquipmentSlotBreakDev
}

func (r *readOnlyTplCrystalEquipmentSlotBreakDevSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplCrystalEquipmentSlotBreakDevSlice) Get(index int) TplCrystalEquipmentSlotBreakDev {
	if index < 0 || index >= len(r.data) {
		return TplCrystalEquipmentSlotBreakDev{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplCrystalEquipmentSlotBreakDevSlice) ToSlice() []TplCrystalEquipmentSlotBreakDev {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplCrystalEquipmentSlotBreakDev, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplCrystalEquipmentSlotBreakDev struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplCrystalEquipmentSlotBreakDev
}

func NewTableTplCrystalEquipmentSlotBreakDev(logger *zap.Logger, loadPath string) *TableTplCrystalEquipmentSlotBreakDev {
	return &TableTplCrystalEquipmentSlotBreakDev{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplCrystalEquipmentSlotBreakDev),
	}
}

func (t *TableTplCrystalEquipmentSlotBreakDev) FindByKey(key string) (TplCrystalEquipmentSlotBreakDev, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplCrystalEquipmentSlotBreakDev) FindByFilter(f func(TplCrystalEquipmentSlotBreakDev) bool) ReadOnlyTplCrystalEquipmentSlotBreakDevSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplCrystalEquipmentSlotBreakDev, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplCrystalEquipmentSlotBreakDevSlice{data: result}
}

func (t *TableTplCrystalEquipmentSlotBreakDev) FindAll() ReadOnlyTplCrystalEquipmentSlotBreakDevSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplCrystalEquipmentSlotBreakDev, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplCrystalEquipmentSlotBreakDevSlice{data: result}
}

func (t *TableTplCrystalEquipmentSlotBreakDev) Release() {
	t.tableData = nil
}

func (t *TableTplCrystalEquipmentSlotBreakDev) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplCrystalEquipmentSlotBreakDevMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplCrystalEquipmentSlotBreakDev.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplCrystalEquipmentSlotBreakDevMap(fileContent, t.logger)
}

func DeserializeStringToTplCrystalEquipmentSlotBreakDevMap(jsonStr []byte, logger *zap.Logger) map[string]TplCrystalEquipmentSlotBreakDev {
	if len(jsonStr) == 0 {
		return make(map[string]TplCrystalEquipmentSlotBreakDev)
	}
	var jsonMap map[string]TplCrystalEquipmentSlotBreakDev
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplCrystalEquipmentSlotBreakDev)
	}
	return jsonMap
}
