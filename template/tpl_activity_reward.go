package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplActivityReward struct {
	ActiveID   string `json:"activeId"`
	ID         string `json:"id"`
	MaxRank    int32  `json:"maxRank"`
	MinRank    int32  `json:"minRank"`
	Name       string `json:"name"`
	RewardIcon string `json:"rewardIcon"`
	RewardID   string `json:"rewardId"`
}

// ReadOnlyTplActivityRewardSlice 只读TplActivityReward切片接口
type ReadOnlyTplActivityRewardSlice interface {
	Len() int
	Get(index int) TplActivityReward
	ToSlice() []TplActivityReward
}

// readOnlyTplActivityRewardSlice 只读TplActivityReward切片实现
type readOnlyTplActivityRewardSlice struct {
	data []TplActivityReward
}

func (r *readOnlyTplActivityRewardSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplActivityRewardSlice) Get(index int) TplActivityReward {
	if index < 0 || index >= len(r.data) {
		return TplActivityReward{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplActivityRewardSlice) ToSlice() []TplActivityReward {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplActivityReward, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplActivityReward struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplActivityReward
}

func NewTableTplActivityReward(logger *zap.Logger, loadPath string) *TableTplActivityReward {
	return &TableTplActivityReward{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplActivityReward),
	}
}

func (t *TableTplActivityReward) FindByKey(key string) (TplActivityReward, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplActivityReward) FindByFilter(f func(TplActivityReward) bool) ReadOnlyTplActivityRewardSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplActivityReward, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplActivityRewardSlice{data: result}
}

func (t *TableTplActivityReward) FindAll() ReadOnlyTplActivityRewardSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplActivityReward, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplActivityRewardSlice{data: result}
}

func (t *TableTplActivityReward) Release() {
	t.tableData = nil
}

func (t *TableTplActivityReward) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplActivityRewardMap(content, t.logger)
		t.logger.Info("从数据库加载模板", zap.String("table", "TplActivityReward"), zap.Int("count", len(t.tableData)))
		return
	}
	path := filepath.Join(t.loadPath, "TplActivityReward.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.String("table", "TplActivityReward"), zap.String("path", path), zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplActivityRewardMap(fileContent, t.logger)
	t.logger.Info("从文件加载模板", zap.String("table", "TplActivityReward"), zap.String("path", path), zap.Int("count", len(t.tableData)))
}

func DeserializeStringToTplActivityRewardMap(jsonStr []byte, logger *zap.Logger) map[string]TplActivityReward {
	if len(jsonStr) == 0 {
		return make(map[string]TplActivityReward)
	}
	var jsonMap map[string]TplActivityReward
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplActivityReward)
	}
	return jsonMap
}
