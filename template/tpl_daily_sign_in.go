package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplDailySignIn struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Reward string `json:"reward"`
}

// ReadOnlyTplDailySignInSlice 只读TplDailySignIn切片接口
type ReadOnlyTplDailySignInSlice interface {
	Len() int
	Get(index int) TplDailySignIn
	ToSlice() []TplDailySignIn
}

// readOnlyTplDailySignInSlice 只读TplDailySignIn切片实现
type readOnlyTplDailySignInSlice struct {
	data []TplDailySignIn
}

func (r *readOnlyTplDailySignInSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplDailySignInSlice) Get(index int) TplDailySignIn {
	if index < 0 || index >= len(r.data) {
		return TplDailySignIn{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplDailySignInSlice) ToSlice() []TplDailySignIn {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplDailySignIn, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplDailySignIn struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplDailySignIn
}

func NewTableTplDailySignIn(logger *zap.Logger, loadPath string) *TableTplDailySignIn {
	return &TableTplDailySignIn{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplDailySignIn),
	}
}

func (t *TableTplDailySignIn) FindByKey(key string) (TplDailySignIn, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplDailySignIn) FindByFilter(f func(TplDailySignIn) bool) ReadOnlyTplDailySignInSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplDailySignIn, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplDailySignInSlice{data: result}
}

func (t *TableTplDailySignIn) FindAll() ReadOnlyTplDailySignInSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplDailySignIn, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplDailySignInSlice{data: result}
}

func (t *TableTplDailySignIn) Release() {
	t.tableData = nil
}

func (t *TableTplDailySignIn) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplDailySignInMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplDailySignIn.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplDailySignInMap(fileContent, t.logger)
}

func DeserializeStringToTplDailySignInMap(jsonStr []byte, logger *zap.Logger) map[string]TplDailySignIn {
	if len(jsonStr) == 0 {
		return make(map[string]TplDailySignIn)
	}
	var jsonMap map[string]TplDailySignIn
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplDailySignIn)
	}
	return jsonMap
}
