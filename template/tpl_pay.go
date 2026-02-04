package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplPay struct {
	ID          string `json:"id"`
	Money       string `json:"money"`
	ProductName string `json:"productName"`
}

// ReadOnlyTplPaySlice 只读TplPay切片接口
type ReadOnlyTplPaySlice interface {
	Len() int
	Get(index int) TplPay
	ToSlice() []TplPay
}

// readOnlyTplPaySlice 只读TplPay切片实现
type readOnlyTplPaySlice struct {
	data []TplPay
}

func (r *readOnlyTplPaySlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplPaySlice) Get(index int) TplPay {
	if index < 0 || index >= len(r.data) {
		return TplPay{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplPaySlice) ToSlice() []TplPay {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplPay, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplPay struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplPay
}

func NewTableTplPay(logger *zap.Logger, loadPath string) *TableTplPay {
	return &TableTplPay{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplPay),
	}
}

func (t *TableTplPay) FindByKey(key string) (TplPay, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplPay) FindByFilter(f func(TplPay) bool) ReadOnlyTplPaySlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplPay, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplPaySlice{data: result}
}

func (t *TableTplPay) FindAll() ReadOnlyTplPaySlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplPay, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplPaySlice{data: result}
}

func (t *TableTplPay) Release() {
	t.tableData = nil
}

func (t *TableTplPay) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplPayMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplPay.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplPayMap(fileContent, t.logger)
}

func DeserializeStringToTplPayMap(jsonStr []byte, logger *zap.Logger) map[string]TplPay {
	if len(jsonStr) == 0 {
		return make(map[string]TplPay)
	}
	var jsonMap map[string]TplPay
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplPay)
	}

	return jsonMap
}
