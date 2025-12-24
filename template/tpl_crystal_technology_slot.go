package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplCrystalTechnologySlot struct {
	ID         string `json:"id"`
	PointIndex int32  `json:"pointIndex"`
}

// ReadOnlyTplCrystalTechnologySlotSlice 只读TplCrystalTechnologySlot切片接口
type ReadOnlyTplCrystalTechnologySlotSlice interface {
	Len() int
	Get(index int) TplCrystalTechnologySlot
	ToSlice() []TplCrystalTechnologySlot
}

// readOnlyTplCrystalTechnologySlotSlice 只读TplCrystalTechnologySlot切片实现
type readOnlyTplCrystalTechnologySlotSlice struct {
	data []TplCrystalTechnologySlot
}

func (r *readOnlyTplCrystalTechnologySlotSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplCrystalTechnologySlotSlice) Get(index int) TplCrystalTechnologySlot {
	if index < 0 || index >= len(r.data) {
		return TplCrystalTechnologySlot{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplCrystalTechnologySlotSlice) ToSlice() []TplCrystalTechnologySlot {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplCrystalTechnologySlot, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplCrystalTechnologySlot struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplCrystalTechnologySlot
}

func NewTableTplCrystalTechnologySlot(logger *zap.Logger, loadPath string) *TableTplCrystalTechnologySlot {
	return &TableTplCrystalTechnologySlot{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplCrystalTechnologySlot),
	}
}

func (t *TableTplCrystalTechnologySlot) FindByKey(key string) (TplCrystalTechnologySlot, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplCrystalTechnologySlot) FindByFilter(f func(TplCrystalTechnologySlot) bool) ReadOnlyTplCrystalTechnologySlotSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplCrystalTechnologySlot, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplCrystalTechnologySlotSlice{data: result}
}

func (t *TableTplCrystalTechnologySlot) FindAll() ReadOnlyTplCrystalTechnologySlotSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplCrystalTechnologySlot, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplCrystalTechnologySlotSlice{data: result}
}

func (t *TableTplCrystalTechnologySlot) Release() {
	t.tableData = nil
}

func (t *TableTplCrystalTechnologySlot) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplCrystalTechnologySlotMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplCrystalTechnologySlot.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplCrystalTechnologySlotMap(fileContent, t.logger)
}

func DeserializeStringToTplCrystalTechnologySlotMap(jsonStr []byte, logger *zap.Logger) map[string]TplCrystalTechnologySlot {
	if len(jsonStr) == 0 {
		return make(map[string]TplCrystalTechnologySlot)
	}
	var jsonMap map[string]TplCrystalTechnologySlot
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplCrystalTechnologySlot)
	}
	return jsonMap
}
