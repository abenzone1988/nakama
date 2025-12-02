package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplEquipmentDev struct {
	CostDebris  int32  `json:"costDebris"`
	CostItemID  string `json:"costItemID"`
	CostItemNum int32  `json:"costItemNum"`
	CostPayType int32  `json:"costPayType"`
	EquipID     string `json:"equipId"`
	ID          string `json:"id"`
	Level       int32  `json:"level"`
	Price       int32  `json:"price"`
}

// ReadOnlyTplEquipmentDevSlice 只读TplEquipmentDev切片接口
type ReadOnlyTplEquipmentDevSlice interface {
	Len() int
	Get(index int) TplEquipmentDev
	ToSlice() []TplEquipmentDev
}

// readOnlyTplEquipmentDevSlice 只读TplEquipmentDev切片实现
type readOnlyTplEquipmentDevSlice struct {
	data []TplEquipmentDev
}

func (r *readOnlyTplEquipmentDevSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplEquipmentDevSlice) Get(index int) TplEquipmentDev {
	if index < 0 || index >= len(r.data) {
		return TplEquipmentDev{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplEquipmentDevSlice) ToSlice() []TplEquipmentDev {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplEquipmentDev, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplEquipmentDev struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplEquipmentDev
}

func NewTableTplEquipmentDev(logger *zap.Logger, loadPath string) *TableTplEquipmentDev {
	return &TableTplEquipmentDev{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplEquipmentDev),
	}
}

func (t *TableTplEquipmentDev) FindByKey(key string) (TplEquipmentDev, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplEquipmentDev) FindByFilter(f func(TplEquipmentDev) bool) ReadOnlyTplEquipmentDevSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplEquipmentDev, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplEquipmentDevSlice{data: result}
}

func (t *TableTplEquipmentDev) FindAll() ReadOnlyTplEquipmentDevSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplEquipmentDev, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplEquipmentDevSlice{data: result}
}

func (t *TableTplEquipmentDev) Release() {
	t.tableData = nil
}

func (t *TableTplEquipmentDev) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplEquipmentDevMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplEquipmentDev.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplEquipmentDevMap(fileContent, t.logger)
}

func DeserializeStringToTplEquipmentDevMap(jsonStr []byte, logger *zap.Logger) map[string]TplEquipmentDev {
	if len(jsonStr) == 0 {
		return make(map[string]TplEquipmentDev)
	}
	var jsonMap map[string]TplEquipmentDev
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplEquipmentDev)
	}
	return jsonMap
}
