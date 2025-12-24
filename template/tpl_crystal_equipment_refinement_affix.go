package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplCrystalEquipmentRefinementAffix struct {
	ID             string `json:"id"`
	MaxValue       int32  `json:"maxValue"`
	MinValue       int32  `json:"minValue"`
	NeedEequipType int32  `json:"needEequipType"`
	NeedQuality    int32  `json:"needQuality"`
	Weight         int32  `json:"weight"`
}

// ReadOnlyTplCrystalEquipmentRefinementAffixSlice 只读TplCrystalEquipmentRefinementAffix切片接口
type ReadOnlyTplCrystalEquipmentRefinementAffixSlice interface {
	Len() int
	Get(index int) TplCrystalEquipmentRefinementAffix
	ToSlice() []TplCrystalEquipmentRefinementAffix
}

// readOnlyTplCrystalEquipmentRefinementAffixSlice 只读TplCrystalEquipmentRefinementAffix切片实现
type readOnlyTplCrystalEquipmentRefinementAffixSlice struct {
	data []TplCrystalEquipmentRefinementAffix
}

func (r *readOnlyTplCrystalEquipmentRefinementAffixSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplCrystalEquipmentRefinementAffixSlice) Get(index int) TplCrystalEquipmentRefinementAffix {
	if index < 0 || index >= len(r.data) {
		return TplCrystalEquipmentRefinementAffix{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplCrystalEquipmentRefinementAffixSlice) ToSlice() []TplCrystalEquipmentRefinementAffix {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplCrystalEquipmentRefinementAffix, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplCrystalEquipmentRefinementAffix struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplCrystalEquipmentRefinementAffix
}

func NewTableTplCrystalEquipmentRefinementAffix(logger *zap.Logger, loadPath string) *TableTplCrystalEquipmentRefinementAffix {
	return &TableTplCrystalEquipmentRefinementAffix{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplCrystalEquipmentRefinementAffix),
	}
}

func (t *TableTplCrystalEquipmentRefinementAffix) FindByKey(key string) (TplCrystalEquipmentRefinementAffix, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplCrystalEquipmentRefinementAffix) FindByFilter(f func(TplCrystalEquipmentRefinementAffix) bool) ReadOnlyTplCrystalEquipmentRefinementAffixSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplCrystalEquipmentRefinementAffix, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplCrystalEquipmentRefinementAffixSlice{data: result}
}

func (t *TableTplCrystalEquipmentRefinementAffix) FindAll() ReadOnlyTplCrystalEquipmentRefinementAffixSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplCrystalEquipmentRefinementAffix, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplCrystalEquipmentRefinementAffixSlice{data: result}
}

func (t *TableTplCrystalEquipmentRefinementAffix) Release() {
	t.tableData = nil
}

func (t *TableTplCrystalEquipmentRefinementAffix) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplCrystalEquipmentRefinementAffixMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplCrystalEquipmentRefinementAffix.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplCrystalEquipmentRefinementAffixMap(fileContent, t.logger)
}

func DeserializeStringToTplCrystalEquipmentRefinementAffixMap(jsonStr []byte, logger *zap.Logger) map[string]TplCrystalEquipmentRefinementAffix {
	if len(jsonStr) == 0 {
		return make(map[string]TplCrystalEquipmentRefinementAffix)
	}
	var jsonMap map[string]TplCrystalEquipmentRefinementAffix
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplCrystalEquipmentRefinementAffix)
	}
	return jsonMap
}
