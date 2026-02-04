package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplProgressReward struct {
	ID        string `json:"id"`
	NeedValue int32  `json:"needValue"`
	Reward    string `json:"reward"`
	Type      int32  `json:"type"`
}

// ReadOnlyTplProgressRewardSlice 只读TplProgressReward切片接口
type ReadOnlyTplProgressRewardSlice interface {
	Len() int
	Get(index int) TplProgressReward
	ToSlice() []TplProgressReward
}

// readOnlyTplProgressRewardSlice 只读TplProgressReward切片实现
type readOnlyTplProgressRewardSlice struct {
	data []TplProgressReward
}

func (r *readOnlyTplProgressRewardSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplProgressRewardSlice) Get(index int) TplProgressReward {
	if index < 0 || index >= len(r.data) {
		return TplProgressReward{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplProgressRewardSlice) ToSlice() []TplProgressReward {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplProgressReward, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplProgressReward struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplProgressReward
}

func NewTableTplProgressReward(logger *zap.Logger, loadPath string) *TableTplProgressReward {
	return &TableTplProgressReward{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplProgressReward),
	}
}

func (t *TableTplProgressReward) FindByKey(key string) (TplProgressReward, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplProgressReward) FindByFilter(f func(TplProgressReward) bool) ReadOnlyTplProgressRewardSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplProgressReward, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplProgressRewardSlice{data: result}
}

func (t *TableTplProgressReward) FindAll() ReadOnlyTplProgressRewardSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplProgressReward, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplProgressRewardSlice{data: result}
}

func (t *TableTplProgressReward) Release() {
	t.tableData = nil
}

func (t *TableTplProgressReward) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplProgressRewardMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplProgressReward.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplProgressRewardMap(fileContent, t.logger)
}

func DeserializeStringToTplProgressRewardMap(jsonStr []byte, logger *zap.Logger) map[string]TplProgressReward {
	if len(jsonStr) == 0 {
		return make(map[string]TplProgressReward)
	}
	var jsonMap map[string]TplProgressReward
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplProgressReward)
	}

	return jsonMap
}
