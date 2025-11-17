package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplActivity struct {
	Icon        string   `json:"icon"`
	ID          string   `json:"id"`
	LimitCount  int32    `json:"limitCount"`
	LimitType   int32    `json:"limitType"`
	Name        string   `json:"name"`
	OpenForDay  []int32  `json:"openForDay"`
	OpenForWeek []int32  `json:"openForWeek"`
	RewardIds   []string `json:"rewardIds"`
	UnlockLevel string   `json:"unlockLevel"`
}

// ReadOnlyTplActivitySlice 只读TplActivity切片接口
type ReadOnlyTplActivitySlice interface {
	Len() int
	Get(index int) TplActivity
	ToSlice() []TplActivity
}

// readOnlyTplActivitySlice 只读TplActivity切片实现
type readOnlyTplActivitySlice struct {
	data []TplActivity
}

func (r *readOnlyTplActivitySlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplActivitySlice) Get(index int) TplActivity {
	if index < 0 || index >= len(r.data) {
		return TplActivity{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplActivitySlice) ToSlice() []TplActivity {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplActivity, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplActivity struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplActivity
}

func NewTableTplActivity(logger *zap.Logger, loadPath string) *TableTplActivity {
	return &TableTplActivity{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplActivity),
	}
}

func (t *TableTplActivity) FindByKey(key string) (TplActivity, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplActivity) FindByFilter(f func(TplActivity) bool) ReadOnlyTplActivitySlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplActivity, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplActivitySlice{data: result}
}

func (t *TableTplActivity) FindAll() ReadOnlyTplActivitySlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplActivity, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplActivitySlice{data: result}
}

func (t *TableTplActivity) Release() {
	t.tableData = nil
}

func (t *TableTplActivity) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplActivityMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplActivity.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplActivityMap(fileContent, t.logger)
}

func DeserializeStringToTplActivityMap(jsonStr []byte, logger *zap.Logger) map[string]TplActivity {
	if len(jsonStr) == 0 {
		return make(map[string]TplActivity)
	}
	var jsonMap map[string]TplActivity
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplActivity)
	}
	return jsonMap
}
