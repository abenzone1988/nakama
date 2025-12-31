package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplCrystalEquipmentSlotDev struct {
	CostItem  string `json:"costItem"`
	EquipType int32  `json:"equipType"`
	ID        string `json:"id"`
	Level     int32  `json:"level"`
}

// ReadOnlyTplCrystalEquipmentSlotDevSlice 只读TplCrystalEquipmentSlotDev切片接口
type ReadOnlyTplCrystalEquipmentSlotDevSlice interface {
	Len() int
	Get(index int) TplCrystalEquipmentSlotDev
	ToSlice() []TplCrystalEquipmentSlotDev
}

// readOnlyTplCrystalEquipmentSlotDevSlice 只读TplCrystalEquipmentSlotDev切片实现
type readOnlyTplCrystalEquipmentSlotDevSlice struct {
	data []TplCrystalEquipmentSlotDev
}

func (r *readOnlyTplCrystalEquipmentSlotDevSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplCrystalEquipmentSlotDevSlice) Get(index int) TplCrystalEquipmentSlotDev {
	if index < 0 || index >= len(r.data) {
		return TplCrystalEquipmentSlotDev{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplCrystalEquipmentSlotDevSlice) ToSlice() []TplCrystalEquipmentSlotDev {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplCrystalEquipmentSlotDev, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplCrystalEquipmentSlotDev struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplCrystalEquipmentSlotDev
}

func NewTableTplCrystalEquipmentSlotDev(logger *zap.Logger, loadPath string) *TableTplCrystalEquipmentSlotDev {
	return &TableTplCrystalEquipmentSlotDev{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplCrystalEquipmentSlotDev),
	}
}

func (t *TableTplCrystalEquipmentSlotDev) FindByKey(key string) (TplCrystalEquipmentSlotDev, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplCrystalEquipmentSlotDev) FindByFilter(f func(TplCrystalEquipmentSlotDev) bool) ReadOnlyTplCrystalEquipmentSlotDevSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplCrystalEquipmentSlotDev, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplCrystalEquipmentSlotDevSlice{data: result}
}

func (t *TableTplCrystalEquipmentSlotDev) FindAll() ReadOnlyTplCrystalEquipmentSlotDevSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplCrystalEquipmentSlotDev, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplCrystalEquipmentSlotDevSlice{data: result}
}

func (t *TableTplCrystalEquipmentSlotDev) Release() {
	t.tableData = nil
}

func (t *TableTplCrystalEquipmentSlotDev) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplCrystalEquipmentSlotDevMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplCrystalEquipmentSlotDev.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplCrystalEquipmentSlotDevMap(fileContent, t.logger)
}

func DeserializeStringToTplCrystalEquipmentSlotDevMap(jsonStr []byte, logger *zap.Logger) map[string]TplCrystalEquipmentSlotDev {
	if len(jsonStr) == 0 {
		return make(map[string]TplCrystalEquipmentSlotDev)
	}
	var jsonMap map[string]TplCrystalEquipmentSlotDev
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplCrystalEquipmentSlotDev)
	}
	return jsonMap
}
