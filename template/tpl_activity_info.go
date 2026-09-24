package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplActivityInfo struct {
	LevelMonsterDev    string `json:"LevelMonsterDev"`
	RewardGroupID      string `json:"RewardGroupID"`
	BattleWaveGroupID  string `json:"battleWaveGroupId"`
	Condition01        int32  `json:"condition01"`
	Condition02        int32  `json:"condition02"`
	Condition03        int32  `json:"condition03"`
	ID                 string `json:"id"`
	MonsterLevel       int32  `json:"monsterLevel"`
	MonsterLimitCount  int32  `json:"monsterLimitCount"`
	MonsterWaveGroupID string `json:"monsterWaveGroupId"`
	Name               string `json:"name"`
	Reward01           string `json:"reward01"`
	Reward02           string `json:"reward02"`
	Reward03           string `json:"reward03"`
	WinRewards         string `json:"winRewards"`
}

// ReadOnlyTplActivityInfoSlice 只读TplActivityInfo切片接口
type ReadOnlyTplActivityInfoSlice interface {
	Len() int
	Get(index int) TplActivityInfo
	ToSlice() []TplActivityInfo
}

// readOnlyTplActivityInfoSlice 只读TplActivityInfo切片实现
type readOnlyTplActivityInfoSlice struct {
	data []TplActivityInfo
}

func (r *readOnlyTplActivityInfoSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplActivityInfoSlice) Get(index int) TplActivityInfo {
	if index < 0 || index >= len(r.data) {
		return TplActivityInfo{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplActivityInfoSlice) ToSlice() []TplActivityInfo {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplActivityInfo, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplActivityInfo struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplActivityInfo
}

func NewTableTplActivityInfo(logger *zap.Logger, loadPath string) *TableTplActivityInfo {
	return &TableTplActivityInfo{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplActivityInfo),
	}
}

func (t *TableTplActivityInfo) FindByKey(key string) (TplActivityInfo, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplActivityInfo) FindByFilter(f func(TplActivityInfo) bool) ReadOnlyTplActivityInfoSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplActivityInfo, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplActivityInfoSlice{data: result}
}

func (t *TableTplActivityInfo) FindAll() ReadOnlyTplActivityInfoSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplActivityInfo, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplActivityInfoSlice{data: result}
}

func (t *TableTplActivityInfo) Release() {
	t.tableData = nil
}

func (t *TableTplActivityInfo) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplActivityInfoMap(content, t.logger)
		t.logger.Info("从数据库加载模板", zap.String("table", "TplActivityInfo"), zap.Int("count", len(t.tableData)))
		return
	}
	path := filepath.Join(t.loadPath, "TplActivityInfo.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.String("table", "TplActivityInfo"), zap.String("path", path), zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplActivityInfoMap(fileContent, t.logger)
	t.logger.Info("从文件加载模板", zap.String("table", "TplActivityInfo"), zap.String("path", path), zap.Int("count", len(t.tableData)))
}

func DeserializeStringToTplActivityInfoMap(jsonStr []byte, logger *zap.Logger) map[string]TplActivityInfo {
	if len(jsonStr) == 0 {
		return make(map[string]TplActivityInfo)
	}
	var jsonMap map[string]TplActivityInfo
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplActivityInfo)
	}
	return jsonMap
}
