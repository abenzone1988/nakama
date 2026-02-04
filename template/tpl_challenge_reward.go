package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplChallengeReward struct {
	ActiveID   string `json:"activeId"`
	ID         string `json:"id"`
	MaxRank    int32  `json:"maxRank"`
	MinRank    int32  `json:"minRank"`
	Name       string `json:"name"`
	RewardIcon string `json:"rewardIcon"`
	RewardID   string `json:"rewardId"`
}

// ReadOnlyTplChallengeRewardSlice 只读TplChallengeReward切片接口
type ReadOnlyTplChallengeRewardSlice interface {
	Len() int
	Get(index int) TplChallengeReward
	ToSlice() []TplChallengeReward
}

// readOnlyTplChallengeRewardSlice 只读TplChallengeReward切片实现
type readOnlyTplChallengeRewardSlice struct {
	data []TplChallengeReward
}

func (r *readOnlyTplChallengeRewardSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplChallengeRewardSlice) Get(index int) TplChallengeReward {
	if index < 0 || index >= len(r.data) {
		return TplChallengeReward{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplChallengeRewardSlice) ToSlice() []TplChallengeReward {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplChallengeReward, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplChallengeReward struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplChallengeReward
}

func NewTableTplChallengeReward(logger *zap.Logger, loadPath string) *TableTplChallengeReward {
	return &TableTplChallengeReward{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplChallengeReward),
	}
}

func (t *TableTplChallengeReward) FindByKey(key string) (TplChallengeReward, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplChallengeReward) FindByFilter(f func(TplChallengeReward) bool) ReadOnlyTplChallengeRewardSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplChallengeReward, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplChallengeRewardSlice{data: result}
}

func (t *TableTplChallengeReward) FindAll() ReadOnlyTplChallengeRewardSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplChallengeReward, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplChallengeRewardSlice{data: result}
}

func (t *TableTplChallengeReward) Release() {
	t.tableData = nil
}

func (t *TableTplChallengeReward) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplChallengeRewardMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplChallengeReward.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplChallengeRewardMap(fileContent, t.logger)
}

func DeserializeStringToTplChallengeRewardMap(jsonStr []byte, logger *zap.Logger) map[string]TplChallengeReward {
	if len(jsonStr) == 0 {
		return make(map[string]TplChallengeReward)
	}
	var jsonMap map[string]TplChallengeReward
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplChallengeReward)
	}

	return jsonMap
}
