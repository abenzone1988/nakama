package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplCrystalEquipmentRefinement struct {
	CostItem           string `json:"costItem"`
	Entrynum           int32  `json:"entrynum"`
	ID                 string `json:"id"`
	Level              int32  `json:"level"`
	Maxratio           int32  `json:"maxratio"`
	Quality            int32  `json:"quality"`
	Rareentrynum       int32  `json:"rareentrynum"`
	Rareentrynumweight int32  `json:"rareentrynumweight"`
}

// ReadOnlyTplCrystalEquipmentRefinementSlice 只读TplCrystalEquipmentRefinement切片接口
type ReadOnlyTplCrystalEquipmentRefinementSlice interface {
	Len() int
	Get(index int) TplCrystalEquipmentRefinement
	ToSlice() []TplCrystalEquipmentRefinement
}

// readOnlyTplCrystalEquipmentRefinementSlice 只读TplCrystalEquipmentRefinement切片实现
type readOnlyTplCrystalEquipmentRefinementSlice struct {
	data []TplCrystalEquipmentRefinement
}

func (r *readOnlyTplCrystalEquipmentRefinementSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplCrystalEquipmentRefinementSlice) Get(index int) TplCrystalEquipmentRefinement {
	if index < 0 || index >= len(r.data) {
		return TplCrystalEquipmentRefinement{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplCrystalEquipmentRefinementSlice) ToSlice() []TplCrystalEquipmentRefinement {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplCrystalEquipmentRefinement, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplCrystalEquipmentRefinement struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplCrystalEquipmentRefinement
}

func NewTableTplCrystalEquipmentRefinement(logger *zap.Logger, loadPath string) *TableTplCrystalEquipmentRefinement {
	return &TableTplCrystalEquipmentRefinement{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplCrystalEquipmentRefinement),
	}
}

func (t *TableTplCrystalEquipmentRefinement) FindByKey(key string) (TplCrystalEquipmentRefinement, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplCrystalEquipmentRefinement) FindByFilter(f func(TplCrystalEquipmentRefinement) bool) ReadOnlyTplCrystalEquipmentRefinementSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplCrystalEquipmentRefinement, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplCrystalEquipmentRefinementSlice{data: result}
}

func (t *TableTplCrystalEquipmentRefinement) FindAll() ReadOnlyTplCrystalEquipmentRefinementSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplCrystalEquipmentRefinement, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplCrystalEquipmentRefinementSlice{data: result}
}

func (t *TableTplCrystalEquipmentRefinement) Release() {
	t.tableData = nil
}

func (t *TableTplCrystalEquipmentRefinement) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplCrystalEquipmentRefinementMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplCrystalEquipmentRefinement.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplCrystalEquipmentRefinementMap(fileContent, t.logger)
}

func DeserializeStringToTplCrystalEquipmentRefinementMap(jsonStr []byte, logger *zap.Logger) map[string]TplCrystalEquipmentRefinement {
	if len(jsonStr) == 0 {
		return make(map[string]TplCrystalEquipmentRefinement)
	}
	var jsonMap map[string]TplCrystalEquipmentRefinement
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplCrystalEquipmentRefinement)
	}
	return jsonMap
}
