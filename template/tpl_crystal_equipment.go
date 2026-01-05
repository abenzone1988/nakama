package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplCrystalEquipment struct {
	EquipType    int32  `json:"equipType"`
	ID           string `json:"id"`
	Level        int32  `json:"level"`
	Quality      int32  `json:"quality"`
	SalvageItems string `json:"salvageItems"`
}

// ReadOnlyTplCrystalEquipmentSlice 只读TplCrystalEquipment切片接口
type ReadOnlyTplCrystalEquipmentSlice interface {
	Len() int
	Get(index int) TplCrystalEquipment
	ToSlice() []TplCrystalEquipment
}

// readOnlyTplCrystalEquipmentSlice 只读TplCrystalEquipment切片实现
type readOnlyTplCrystalEquipmentSlice struct {
	data []TplCrystalEquipment
}

func (r *readOnlyTplCrystalEquipmentSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplCrystalEquipmentSlice) Get(index int) TplCrystalEquipment {
	if index < 0 || index >= len(r.data) {
		return TplCrystalEquipment{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplCrystalEquipmentSlice) ToSlice() []TplCrystalEquipment {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplCrystalEquipment, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplCrystalEquipment struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplCrystalEquipment
}

func NewTableTplCrystalEquipment(logger *zap.Logger, loadPath string) *TableTplCrystalEquipment {
	return &TableTplCrystalEquipment{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplCrystalEquipment),
	}
}

func (t *TableTplCrystalEquipment) FindByKey(key string) (TplCrystalEquipment, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplCrystalEquipment) FindByFilter(f func(TplCrystalEquipment) bool) ReadOnlyTplCrystalEquipmentSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplCrystalEquipment, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplCrystalEquipmentSlice{data: result}
}

func (t *TableTplCrystalEquipment) FindAll() ReadOnlyTplCrystalEquipmentSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplCrystalEquipment, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplCrystalEquipmentSlice{data: result}
}

func (t *TableTplCrystalEquipment) Release() {
	t.tableData = nil
}

func (t *TableTplCrystalEquipment) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplCrystalEquipmentMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplCrystalEquipment.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplCrystalEquipmentMap(fileContent, t.logger)
}

func DeserializeStringToTplCrystalEquipmentMap(jsonStr []byte, logger *zap.Logger) map[string]TplCrystalEquipment {
	if len(jsonStr) == 0 {
		return make(map[string]TplCrystalEquipment)
	}
	var jsonMap map[string]TplCrystalEquipment
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplCrystalEquipment)
	}
	return jsonMap
}
