package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplReward struct {
	Coin   int32  `json:"coin"`
	Coupon int32  `json:"coupon"`
	Gem    int32  `json:"gem"`
	ID     string `json:"id"`
	Items  string `json:"items"`
	Name   string `json:"name"`
}

// ReadOnlyTplRewardSlice 只读TplReward切片接口
type ReadOnlyTplRewardSlice interface {
	Len() int
	Get(index int) TplReward
	ToSlice() []TplReward
}

// readOnlyTplRewardSlice 只读TplReward切片实现
type readOnlyTplRewardSlice struct {
	data []TplReward
}

func (r *readOnlyTplRewardSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplRewardSlice) Get(index int) TplReward {
	if index < 0 || index >= len(r.data) {
		return TplReward{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplRewardSlice) ToSlice() []TplReward {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplReward, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplReward struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplReward
}

func NewTableTplReward(logger *zap.Logger, loadPath string) *TableTplReward {
	return &TableTplReward{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplReward),
	}
}

func (t *TableTplReward) FindByKey(key string) (TplReward, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplReward) FindByFilter(f func(TplReward) bool) ReadOnlyTplRewardSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplReward, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplRewardSlice{data: result}
}

func (t *TableTplReward) FindAll() ReadOnlyTplRewardSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplReward, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplRewardSlice{data: result}
}

func (t *TableTplReward) Release() {
	t.tableData = nil
}

func (t *TableTplReward) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplRewardMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplReward.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplRewardMap(fileContent, t.logger)
}

func DeserializeStringToTplRewardMap(jsonStr []byte, logger *zap.Logger) map[string]TplReward {
	if len(jsonStr) == 0 {
		return make(map[string]TplReward)
	}
	var jsonMap map[string]TplReward
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplReward)
	}

	return jsonMap
}
