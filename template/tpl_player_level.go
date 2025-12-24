package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplPlayerLevel struct {
	CumulativeExp int32  `json:"CumulativeExp"`
	ExpRequire    int32  `json:"ExpRequire"`
	ID            int32  `json:"id"`
	Reward        string `json:"reward"`
}

// ReadOnlyTplPlayerLevelSlice 只读TplPlayerLevel切片接口
type ReadOnlyTplPlayerLevelSlice interface {
	Len() int
	Get(index int) TplPlayerLevel
	ToSlice() []TplPlayerLevel
}

// readOnlyTplPlayerLevelSlice 只读TplPlayerLevel切片实现
type readOnlyTplPlayerLevelSlice struct {
	data []TplPlayerLevel
}

func (r *readOnlyTplPlayerLevelSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplPlayerLevelSlice) Get(index int) TplPlayerLevel {
	if index < 0 || index >= len(r.data) {
		return TplPlayerLevel{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplPlayerLevelSlice) ToSlice() []TplPlayerLevel {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplPlayerLevel, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplPlayerLevel struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplPlayerLevel
}

func NewTableTplPlayerLevel(logger *zap.Logger, loadPath string) *TableTplPlayerLevel {
	return &TableTplPlayerLevel{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplPlayerLevel),
	}
}

func (t *TableTplPlayerLevel) FindByKey(key string) (TplPlayerLevel, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplPlayerLevel) FindByFilter(f func(TplPlayerLevel) bool) ReadOnlyTplPlayerLevelSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplPlayerLevel, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplPlayerLevelSlice{data: result}
}

func (t *TableTplPlayerLevel) FindAll() ReadOnlyTplPlayerLevelSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplPlayerLevel, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplPlayerLevelSlice{data: result}
}

func (t *TableTplPlayerLevel) Release() {
	t.tableData = nil
}

func (t *TableTplPlayerLevel) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplPlayerLevelMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplPlayerLevel.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplPlayerLevelMap(fileContent, t.logger)
}

func DeserializeStringToTplPlayerLevelMap(jsonStr []byte, logger *zap.Logger) map[string]TplPlayerLevel {
	if len(jsonStr) == 0 {
		return make(map[string]TplPlayerLevel)
	}
	var jsonMap map[string]TplPlayerLevel
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplPlayerLevel)
	}
	return jsonMap
}
