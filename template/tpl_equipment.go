package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplEquipment struct {
	ID      string `json:"id"`
	Occupy  int32  `json:"occupy"`
	Quality int32  `json:"quality"`
	Type    int32  `json:"type"`
}

// ReadOnlyTplEquipmentSlice 只读TplEquipment切片接口
type ReadOnlyTplEquipmentSlice interface {
	Len() int
	Get(index int) TplEquipment
	ToSlice() []TplEquipment
}

// readOnlyTplEquipmentSlice 只读TplEquipment切片实现
type readOnlyTplEquipmentSlice struct {
	data []TplEquipment
}

func (r *readOnlyTplEquipmentSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplEquipmentSlice) Get(index int) TplEquipment {
	if index < 0 || index >= len(r.data) {
		return TplEquipment{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplEquipmentSlice) ToSlice() []TplEquipment {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplEquipment, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplEquipment struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplEquipment
}

func NewTableTplEquipment(logger *zap.Logger, loadPath string) *TableTplEquipment {
	return &TableTplEquipment{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplEquipment),
	}
}

func (t *TableTplEquipment) FindByKey(key string) (TplEquipment, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplEquipment) FindByFilter(f func(TplEquipment) bool) ReadOnlyTplEquipmentSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplEquipment, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplEquipmentSlice{data: result}
}

func (t *TableTplEquipment) FindAll() ReadOnlyTplEquipmentSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplEquipment, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplEquipmentSlice{data: result}
}

func (t *TableTplEquipment) Release() {
	t.tableData = nil
}

func (t *TableTplEquipment) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplEquipmentMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplEquipment.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplEquipmentMap(fileContent, t.logger)
}

func DeserializeStringToTplEquipmentMap(jsonStr []byte, logger *zap.Logger) map[string]TplEquipment {
	if len(jsonStr) == 0 {
		return make(map[string]TplEquipment)
	}
	var jsonMap map[string]TplEquipment
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplEquipment)
	}
	return jsonMap
}
