package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplEquipmentBase struct {
	EquipID string `json:"equipId"`
	ID      int32  `json:"id"`
	Param   int32  `json:"param"`
	Type    string `json:"type"`
}

// ReadOnlyTplEquipmentBaseSlice 只读TplEquipmentBase切片接口
type ReadOnlyTplEquipmentBaseSlice interface {
	Len() int
	Get(index int) TplEquipmentBase
	ToSlice() []TplEquipmentBase
}

// readOnlyTplEquipmentBaseSlice 只读TplEquipmentBase切片实现
type readOnlyTplEquipmentBaseSlice struct {
	data []TplEquipmentBase
}

func (r *readOnlyTplEquipmentBaseSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplEquipmentBaseSlice) Get(index int) TplEquipmentBase {
	if index < 0 || index >= len(r.data) {
		return TplEquipmentBase{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplEquipmentBaseSlice) ToSlice() []TplEquipmentBase {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplEquipmentBase, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplEquipmentBase struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplEquipmentBase
}

func NewTableTplEquipmentBase(logger *zap.Logger, loadPath string) *TableTplEquipmentBase {
	return &TableTplEquipmentBase{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplEquipmentBase),
	}
}

func (t *TableTplEquipmentBase) FindByKey(key string) (TplEquipmentBase, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplEquipmentBase) FindByFilter(f func(TplEquipmentBase) bool) ReadOnlyTplEquipmentBaseSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplEquipmentBase, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplEquipmentBaseSlice{data: result}
}

func (t *TableTplEquipmentBase) FindAll() ReadOnlyTplEquipmentBaseSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplEquipmentBase, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplEquipmentBaseSlice{data: result}
}

func (t *TableTplEquipmentBase) Release() {
	t.tableData = nil
}

func (t *TableTplEquipmentBase) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplEquipmentBaseMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplEquipmentBase.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplEquipmentBaseMap(fileContent, t.logger)
}

func DeserializeStringToTplEquipmentBaseMap(jsonStr []byte, logger *zap.Logger) map[string]TplEquipmentBase {
	if len(jsonStr) == 0 {
		return make(map[string]TplEquipmentBase)
	}
	var jsonMap map[string]TplEquipmentBase
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplEquipmentBase)
	}
	return jsonMap
}
