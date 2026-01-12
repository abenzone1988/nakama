package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplTasks struct {
	Liveness    int32  `json:"Liveness"`
	TargetType  string `json:"TargetType"`
	TargetValue int32  `json:"TargetValue"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Reward      string `json:"reward"`
	TaskType    int32  `json:"taskType"`
}

// ReadOnlyTplTasksSlice 只读TplTasks切片接口
type ReadOnlyTplTasksSlice interface {
	Len() int
	Get(index int) TplTasks
	ToSlice() []TplTasks
}

// readOnlyTplTasksSlice 只读TplTasks切片实现
type readOnlyTplTasksSlice struct {
	data []TplTasks
}

func (r *readOnlyTplTasksSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplTasksSlice) Get(index int) TplTasks {
	if index < 0 || index >= len(r.data) {
		return TplTasks{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplTasksSlice) ToSlice() []TplTasks {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplTasks, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplTasks struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplTasks
}

func NewTableTplTasks(logger *zap.Logger, loadPath string) *TableTplTasks {
	return &TableTplTasks{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplTasks),
	}
}

func (t *TableTplTasks) FindByKey(key string) (TplTasks, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplTasks) FindByFilter(f func(TplTasks) bool) ReadOnlyTplTasksSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplTasks, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplTasksSlice{data: result}
}

func (t *TableTplTasks) FindAll() ReadOnlyTplTasksSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplTasks, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplTasksSlice{data: result}
}

func (t *TableTplTasks) Release() {
	t.tableData = nil
}

func (t *TableTplTasks) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplTasksMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplTasks.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplTasksMap(fileContent, t.logger)
}

func DeserializeStringToTplTasksMap(jsonStr []byte, logger *zap.Logger) map[string]TplTasks {
	if len(jsonStr) == 0 {
		return make(map[string]TplTasks)
	}
	var jsonMap map[string]TplTasks
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplTasks)
	}
	return jsonMap
}
