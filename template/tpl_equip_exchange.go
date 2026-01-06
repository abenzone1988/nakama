package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplEquipExchange struct {
	Costs     []int32 `json:"costs"`
	Count     int32   `json:"count"`
	ID        string  `json:"id"`
	ItemID    string  `json:"itemId"`
	Name      string  `json:"name"`
	WeekLimit int32   `json:"weekLimit"`
}

// ReadOnlyTplEquipExchangeSlice 只读TplEquipExchange切片接口
type ReadOnlyTplEquipExchangeSlice interface {
	Len() int
	Get(index int) TplEquipExchange
	ToSlice() []TplEquipExchange
}

// readOnlyTplEquipExchangeSlice 只读TplEquipExchange切片实现
type readOnlyTplEquipExchangeSlice struct {
	data []TplEquipExchange
}

func (r *readOnlyTplEquipExchangeSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplEquipExchangeSlice) Get(index int) TplEquipExchange {
	if index < 0 || index >= len(r.data) {
		return TplEquipExchange{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplEquipExchangeSlice) ToSlice() []TplEquipExchange {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplEquipExchange, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplEquipExchange struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplEquipExchange
}

func NewTableTplEquipExchange(logger *zap.Logger, loadPath string) *TableTplEquipExchange {
	return &TableTplEquipExchange{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplEquipExchange),
	}
}

func (t *TableTplEquipExchange) FindByKey(key string) (TplEquipExchange, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplEquipExchange) FindByFilter(f func(TplEquipExchange) bool) ReadOnlyTplEquipExchangeSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplEquipExchange, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplEquipExchangeSlice{data: result}
}

func (t *TableTplEquipExchange) FindAll() ReadOnlyTplEquipExchangeSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplEquipExchange, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplEquipExchangeSlice{data: result}
}

func (t *TableTplEquipExchange) Release() {
	t.tableData = nil
}

func (t *TableTplEquipExchange) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplEquipExchangeMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplEquipExchange.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplEquipExchangeMap(fileContent, t.logger)
}

func DeserializeStringToTplEquipExchangeMap(jsonStr []byte, logger *zap.Logger) map[string]TplEquipExchange {
	if len(jsonStr) == 0 {
		return make(map[string]TplEquipExchange)
	}
	var jsonMap map[string]TplEquipExchange
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplEquipExchange)
	}
	return jsonMap
}
