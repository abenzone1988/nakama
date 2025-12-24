package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplSevenDaySignIn struct {
	Day    int32  `json:"day"`
	Gem    int32  `json:"gem"`
	ID     string `json:"id"`
	Name   string `json:"name"`
	Reward string `json:"reward"`
}

// ReadOnlyTplSevenDaySignInSlice 只读TplSevenDaySignIn切片接口
type ReadOnlyTplSevenDaySignInSlice interface {
	Len() int
	Get(index int) TplSevenDaySignIn
	ToSlice() []TplSevenDaySignIn
}

// readOnlyTplSevenDaySignInSlice 只读TplSevenDaySignIn切片实现
type readOnlyTplSevenDaySignInSlice struct {
	data []TplSevenDaySignIn
}

func (r *readOnlyTplSevenDaySignInSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplSevenDaySignInSlice) Get(index int) TplSevenDaySignIn {
	if index < 0 || index >= len(r.data) {
		return TplSevenDaySignIn{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplSevenDaySignInSlice) ToSlice() []TplSevenDaySignIn {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplSevenDaySignIn, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplSevenDaySignIn struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplSevenDaySignIn
}

func NewTableTplSevenDaySignIn(logger *zap.Logger, loadPath string) *TableTplSevenDaySignIn {
	return &TableTplSevenDaySignIn{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplSevenDaySignIn),
	}
}

func (t *TableTplSevenDaySignIn) FindByKey(key string) (TplSevenDaySignIn, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplSevenDaySignIn) FindByFilter(f func(TplSevenDaySignIn) bool) ReadOnlyTplSevenDaySignInSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplSevenDaySignIn, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplSevenDaySignInSlice{data: result}
}

func (t *TableTplSevenDaySignIn) FindAll() ReadOnlyTplSevenDaySignInSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplSevenDaySignIn, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplSevenDaySignInSlice{data: result}
}

func (t *TableTplSevenDaySignIn) Release() {
	t.tableData = nil
}

func (t *TableTplSevenDaySignIn) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplSevenDaySignInMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplSevenDaySignIn.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplSevenDaySignInMap(fileContent, t.logger)
}

func DeserializeStringToTplSevenDaySignInMap(jsonStr []byte, logger *zap.Logger) map[string]TplSevenDaySignIn {
	if len(jsonStr) == 0 {
		return make(map[string]TplSevenDaySignIn)
	}
	var jsonMap map[string]TplSevenDaySignIn
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplSevenDaySignIn)
	}
	return jsonMap
}
