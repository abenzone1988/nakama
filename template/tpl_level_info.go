package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplLevelInfo struct {
	LevelMonsterDev    string `json:"LevelMonsterDev"`
	Condition01        int32  `json:"condition01"`
	Condition02        int32  `json:"condition02"`
	Condition03        int32  `json:"condition03"`
	ConditionType01    int32  `json:"conditionType01"`
	ConditionType02    int32  `json:"conditionType02"`
	ConditionType03    int32  `json:"conditionType03"`
	Cost               int32  `json:"cost"`
	ID                 string `json:"id"`
	MonsterLevel       int32  `json:"monsterLevel"`
	MonsterWaveGroupID string `json:"monsterWaveGroupId"`
	MoppingReward      string `json:"moppingReward"`
	OnHookRewardID     string `json:"onHookRewardID"`
	Reward01           string `json:"reward01"`
	Reward02           string `json:"reward02"`
	Reward03           string `json:"reward03"`
	WinRewards         string `json:"winRewards"`
}

// ReadOnlyTplLevelInfoSlice 只读TplLevelInfo切片接口
type ReadOnlyTplLevelInfoSlice interface {
	Len() int
	Get(index int) TplLevelInfo
	ToSlice() []TplLevelInfo
}

// readOnlyTplLevelInfoSlice 只读TplLevelInfo切片实现
type readOnlyTplLevelInfoSlice struct {
	data []TplLevelInfo
}

func (r *readOnlyTplLevelInfoSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplLevelInfoSlice) Get(index int) TplLevelInfo {
	if index < 0 || index >= len(r.data) {
		return TplLevelInfo{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplLevelInfoSlice) ToSlice() []TplLevelInfo {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplLevelInfo, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplLevelInfo struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplLevelInfo
}

func NewTableTplLevelInfo(logger *zap.Logger, loadPath string) *TableTplLevelInfo {
	return &TableTplLevelInfo{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplLevelInfo),
	}
}

func (t *TableTplLevelInfo) FindByKey(key string) (TplLevelInfo, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplLevelInfo) FindByFilter(f func(TplLevelInfo) bool) ReadOnlyTplLevelInfoSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplLevelInfo, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplLevelInfoSlice{data: result}
}

func (t *TableTplLevelInfo) FindAll() ReadOnlyTplLevelInfoSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplLevelInfo, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplLevelInfoSlice{data: result}
}

func (t *TableTplLevelInfo) Release() {
	t.tableData = nil
}

func (t *TableTplLevelInfo) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplLevelInfoMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplLevelInfo.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplLevelInfoMap(fileContent, t.logger)
}

func DeserializeStringToTplLevelInfoMap(jsonStr []byte, logger *zap.Logger) map[string]TplLevelInfo {
	if len(jsonStr) == 0 {
		return make(map[string]TplLevelInfo)
	}
	var jsonMap map[string]TplLevelInfo
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplLevelInfo)
	}

	return jsonMap
}
