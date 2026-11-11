package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplFirstCharge struct {
	Day    int32  `json:"day"`
	ID     string `json:"id"`
	Name   string `json:"name"`
	Reward string `json:"reward"`
}

// ReadOnlyTplFirstChargeSlice 只读TplFirstCharge切片接口
type ReadOnlyTplFirstChargeSlice interface {
	Len() int
	Get(index int) TplFirstCharge
	ToSlice() []TplFirstCharge
}

// readOnlyTplFirstChargeSlice 只读TplFirstCharge切片实现
type readOnlyTplFirstChargeSlice struct {
	data []TplFirstCharge
}

func (r *readOnlyTplFirstChargeSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplFirstChargeSlice) Get(index int) TplFirstCharge {
	if index < 0 || index >= len(r.data) {
		return TplFirstCharge{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplFirstChargeSlice) ToSlice() []TplFirstCharge {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplFirstCharge, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplFirstCharge struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplFirstCharge
}

func NewTableTplFirstCharge(logger *zap.Logger, loadPath string) *TableTplFirstCharge {
	return &TableTplFirstCharge{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplFirstCharge),
	}
}

func (t *TableTplFirstCharge) FindByKey(key string) (TplFirstCharge, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplFirstCharge) FindByFilter(f func(TplFirstCharge) bool) ReadOnlyTplFirstChargeSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplFirstCharge, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplFirstChargeSlice{data: result}
}

func (t *TableTplFirstCharge) FindAll() ReadOnlyTplFirstChargeSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplFirstCharge, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplFirstChargeSlice{data: result}
}

func (t *TableTplFirstCharge) Release() {
	t.tableData = nil
}

func (t *TableTplFirstCharge) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplFirstChargeMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplFirstCharge.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplFirstChargeMap(fileContent, t.logger)
}

func DeserializeStringToTplFirstChargeMap(jsonStr []byte, logger *zap.Logger) map[string]TplFirstCharge {
	if len(jsonStr) == 0 {
		return make(map[string]TplFirstCharge)
	}
	var jsonMap map[string]TplFirstCharge
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplFirstCharge)
	}
	return jsonMap
}
