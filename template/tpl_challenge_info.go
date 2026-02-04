package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplChallengeInfo struct {
	LevelMonsterDev    string `json:"LevelMonsterDev"`
	RewardGroupID      string `json:"RewardGroupID"`
	Condition01        int32  `json:"condition01"`
	Condition02        int32  `json:"condition02"`
	Condition03        int32  `json:"condition03"`
	ID                 string `json:"id"`
	MonsterLevel       int32  `json:"monsterLevel"`
	MonsterWaveGroupID string `json:"monsterWaveGroupId"`
	Name               string `json:"name"`
	Reward01           string `json:"reward01"`
	Reward02           string `json:"reward02"`
	Reward03           string `json:"reward03"`
	Stamina            int32  `json:"stamina"`
	WinRewards         string `json:"winRewards"`
}

// ReadOnlyTplChallengeInfoSlice 只读TplChallengeInfo切片接口
type ReadOnlyTplChallengeInfoSlice interface {
	Len() int
	Get(index int) TplChallengeInfo
	ToSlice() []TplChallengeInfo
}

// readOnlyTplChallengeInfoSlice 只读TplChallengeInfo切片实现
type readOnlyTplChallengeInfoSlice struct {
	data []TplChallengeInfo
}

func (r *readOnlyTplChallengeInfoSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplChallengeInfoSlice) Get(index int) TplChallengeInfo {
	if index < 0 || index >= len(r.data) {
		return TplChallengeInfo{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplChallengeInfoSlice) ToSlice() []TplChallengeInfo {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplChallengeInfo, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplChallengeInfo struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplChallengeInfo
}

func NewTableTplChallengeInfo(logger *zap.Logger, loadPath string) *TableTplChallengeInfo {
	return &TableTplChallengeInfo{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplChallengeInfo),
	}
}

func (t *TableTplChallengeInfo) FindByKey(key string) (TplChallengeInfo, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplChallengeInfo) FindByFilter(f func(TplChallengeInfo) bool) ReadOnlyTplChallengeInfoSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplChallengeInfo, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplChallengeInfoSlice{data: result}
}

func (t *TableTplChallengeInfo) FindAll() ReadOnlyTplChallengeInfoSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplChallengeInfo, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplChallengeInfoSlice{data: result}
}

func (t *TableTplChallengeInfo) Release() {
	t.tableData = nil
}

func (t *TableTplChallengeInfo) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplChallengeInfoMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplChallengeInfo.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplChallengeInfoMap(fileContent, t.logger)
}

func DeserializeStringToTplChallengeInfoMap(jsonStr []byte, logger *zap.Logger) map[string]TplChallengeInfo {
	if len(jsonStr) == 0 {
		return make(map[string]TplChallengeInfo)
	}
	var jsonMap map[string]TplChallengeInfo
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplChallengeInfo)
	}

	return jsonMap
}
