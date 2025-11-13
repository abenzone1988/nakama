package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplDropReward struct {
	Count       int32  `json:"count"`
	GroupID     string `json:"groupId"`
	ID          string `json:"id"`
	ItemID      string `json:"itemId"`
	Probability int32  `json:"probability"`
	Type        int32  `json:"type"`
}

// ReadOnlyTplDropRewardSlice 只读TplDropReward切片接口
type ReadOnlyTplDropRewardSlice interface {
	Len() int
	Get(index int) TplDropReward
	ToSlice() []TplDropReward
}

// readOnlyTplDropRewardSlice 只读TplDropReward切片实现
type readOnlyTplDropRewardSlice struct {
	data []TplDropReward
}

func (r *readOnlyTplDropRewardSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplDropRewardSlice) Get(index int) TplDropReward {
	if index < 0 || index >= len(r.data) {
		return TplDropReward{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplDropRewardSlice) ToSlice() []TplDropReward {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplDropReward, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplDropReward struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplDropReward
}

func NewTableTplDropReward(logger *zap.Logger, loadPath string) *TableTplDropReward {
	return &TableTplDropReward{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplDropReward),
	}
}

func (t *TableTplDropReward) FindByKey(key string) (TplDropReward, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplDropReward) FindByFilter(f func(TplDropReward) bool) ReadOnlyTplDropRewardSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplDropReward, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplDropRewardSlice{data: result}
}

func (t *TableTplDropReward) FindAll() ReadOnlyTplDropRewardSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplDropReward, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplDropRewardSlice{data: result}
}

func (t *TableTplDropReward) Release() {
	t.tableData = nil
}

func (t *TableTplDropReward) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplDropRewardMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplDropReward.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplDropRewardMap(fileContent, t.logger)
}

func DeserializeStringToTplDropRewardMap(jsonStr []byte, logger *zap.Logger) map[string]TplDropReward {
	if len(jsonStr) == 0 {
		return make(map[string]TplDropReward)
	}
	var jsonMap map[string]TplDropReward
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplDropReward)
	}
	return jsonMap
}
