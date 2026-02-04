package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplUnlock struct {
	ConditionParameter string `json:"conditionParameter"`
	ConditionType      int32  `json:"conditionType"`
	Content            string `json:"content"`
	ID                 string `json:"id"`
	Parameter          string `json:"parameter"`
	Title              string `json:"title"`
	Type               int32  `json:"type"`
}

// ReadOnlyTplUnlockSlice 只读TplUnlock切片接口
type ReadOnlyTplUnlockSlice interface {
	Len() int
	Get(index int) TplUnlock
	ToSlice() []TplUnlock
}

// readOnlyTplUnlockSlice 只读TplUnlock切片实现
type readOnlyTplUnlockSlice struct {
	data []TplUnlock
}

func (r *readOnlyTplUnlockSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplUnlockSlice) Get(index int) TplUnlock {
	if index < 0 || index >= len(r.data) {
		return TplUnlock{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplUnlockSlice) ToSlice() []TplUnlock {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplUnlock, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplUnlock struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplUnlock
}

func NewTableTplUnlock(logger *zap.Logger, loadPath string) *TableTplUnlock {
	return &TableTplUnlock{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplUnlock),
	}
}

func (t *TableTplUnlock) FindByKey(key string) (TplUnlock, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplUnlock) FindByFilter(f func(TplUnlock) bool) ReadOnlyTplUnlockSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplUnlock, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplUnlockSlice{data: result}
}

func (t *TableTplUnlock) FindAll() ReadOnlyTplUnlockSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplUnlock, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplUnlockSlice{data: result}
}

func (t *TableTplUnlock) Release() {
	t.tableData = nil
}

func (t *TableTplUnlock) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplUnlockMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplUnlock.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplUnlockMap(fileContent, t.logger)
}

func DeserializeStringToTplUnlockMap(jsonStr []byte, logger *zap.Logger) map[string]TplUnlock {
	if len(jsonStr) == 0 {
		return make(map[string]TplUnlock)
	}
	var jsonMap map[string]TplUnlock
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplUnlock)
	}

	return jsonMap
}
