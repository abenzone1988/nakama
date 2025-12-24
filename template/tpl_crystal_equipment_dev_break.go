package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplCrystalEquipmentDevBreak struct {
	EquipType int32  `json:"equipType"`
	ID        string `json:"id"`
	Level     int32  `json:"level"`
}

// ReadOnlyTplCrystalEquipmentDevBreakSlice 只读TplCrystalEquipmentDevBreak切片接口
type ReadOnlyTplCrystalEquipmentDevBreakSlice interface {
	Len() int
	Get(index int) TplCrystalEquipmentDevBreak
	ToSlice() []TplCrystalEquipmentDevBreak
}

// readOnlyTplCrystalEquipmentDevBreakSlice 只读TplCrystalEquipmentDevBreak切片实现
type readOnlyTplCrystalEquipmentDevBreakSlice struct {
	data []TplCrystalEquipmentDevBreak
}

func (r *readOnlyTplCrystalEquipmentDevBreakSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplCrystalEquipmentDevBreakSlice) Get(index int) TplCrystalEquipmentDevBreak {
	if index < 0 || index >= len(r.data) {
		return TplCrystalEquipmentDevBreak{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplCrystalEquipmentDevBreakSlice) ToSlice() []TplCrystalEquipmentDevBreak {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplCrystalEquipmentDevBreak, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplCrystalEquipmentDevBreak struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplCrystalEquipmentDevBreak
}

func NewTableTplCrystalEquipmentDevBreak(logger *zap.Logger, loadPath string) *TableTplCrystalEquipmentDevBreak {
	return &TableTplCrystalEquipmentDevBreak{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplCrystalEquipmentDevBreak),
	}
}

func (t *TableTplCrystalEquipmentDevBreak) FindByKey(key string) (TplCrystalEquipmentDevBreak, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplCrystalEquipmentDevBreak) FindByFilter(f func(TplCrystalEquipmentDevBreak) bool) ReadOnlyTplCrystalEquipmentDevBreakSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplCrystalEquipmentDevBreak, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplCrystalEquipmentDevBreakSlice{data: result}
}

func (t *TableTplCrystalEquipmentDevBreak) FindAll() ReadOnlyTplCrystalEquipmentDevBreakSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplCrystalEquipmentDevBreak, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplCrystalEquipmentDevBreakSlice{data: result}
}

func (t *TableTplCrystalEquipmentDevBreak) Release() {
	t.tableData = nil
}

func (t *TableTplCrystalEquipmentDevBreak) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplCrystalEquipmentDevBreakMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplCrystalEquipmentDevBreak.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplCrystalEquipmentDevBreakMap(fileContent, t.logger)
}

func DeserializeStringToTplCrystalEquipmentDevBreakMap(jsonStr []byte, logger *zap.Logger) map[string]TplCrystalEquipmentDevBreak {
	if len(jsonStr) == 0 {
		return make(map[string]TplCrystalEquipmentDevBreak)
	}
	var jsonMap map[string]TplCrystalEquipmentDevBreak
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplCrystalEquipmentDevBreak)
	}
	return jsonMap
}
