package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplMine struct {
	ActivityID string `json:"activityId"`
	Count      int32  `json:"count"`
	Crit       int32  `json:"crit"`
	Critcount  int32  `json:"critcount"`
	ID         string `json:"id"`
	Maxcount   int32  `json:"maxcount"`
	Name       string `json:"name"`
	NextID     string `json:"nextID"`
}

// ReadOnlyTplMineSlice 只读TplMine切片接口
type ReadOnlyTplMineSlice interface {
	Len() int
	Get(index int) TplMine
	ToSlice() []TplMine
}

// readOnlyTplMineSlice 只读TplMine切片实现
type readOnlyTplMineSlice struct {
	data []TplMine
}

func (r *readOnlyTplMineSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplMineSlice) Get(index int) TplMine {
	if index < 0 || index >= len(r.data) {
		return TplMine{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplMineSlice) ToSlice() []TplMine {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplMine, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplMine struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplMine
}

func NewTableTplMine(logger *zap.Logger, loadPath string) *TableTplMine {
	return &TableTplMine{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplMine),
	}
}

func (t *TableTplMine) FindByKey(key string) (TplMine, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplMine) FindByFilter(f func(TplMine) bool) ReadOnlyTplMineSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplMine, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplMineSlice{data: result}
}

func (t *TableTplMine) FindAll() ReadOnlyTplMineSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplMine, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplMineSlice{data: result}
}

func (t *TableTplMine) Release() {
	t.tableData = nil
}

func (t *TableTplMine) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplMineMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplMine.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplMineMap(fileContent, t.logger)
}

func DeserializeStringToTplMineMap(jsonStr []byte, logger *zap.Logger) map[string]TplMine {
	if len(jsonStr) == 0 {
		return make(map[string]TplMine)
	}
	var jsonMap map[string]TplMine
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplMine)
	}
	return jsonMap
}
