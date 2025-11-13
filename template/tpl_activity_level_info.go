package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplActivityLevelInfo struct {
	ActivityID         string `json:"activityId"`
	ID                 string `json:"id"`
	LevelMonsterDev    string `json:"levelMonsterDev"`
	MainlevelProgress  string `json:"mainlevelProgress"`
	MonsterLevel       int32  `json:"monsterLevel"`
	MonsterWaveGroupID string `json:"monsterWaveGroupId"`
	RewardID           string `json:"rewardId"`
	Stamina            int32  `json:"stamina"`
	UnlockLevel        string `json:"unlockLevel"`
}

// ReadOnlyTplActivityLevelInfoSlice 只读TplActivityLevelInfo切片接口
type ReadOnlyTplActivityLevelInfoSlice interface {
	Len() int
	Get(index int) TplActivityLevelInfo
	ToSlice() []TplActivityLevelInfo
}

// readOnlyTplActivityLevelInfoSlice 只读TplActivityLevelInfo切片实现
type readOnlyTplActivityLevelInfoSlice struct {
	data []TplActivityLevelInfo
}

func (r *readOnlyTplActivityLevelInfoSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplActivityLevelInfoSlice) Get(index int) TplActivityLevelInfo {
	if index < 0 || index >= len(r.data) {
		return TplActivityLevelInfo{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplActivityLevelInfoSlice) ToSlice() []TplActivityLevelInfo {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplActivityLevelInfo, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplActivityLevelInfo struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplActivityLevelInfo
}

func NewTableTplActivityLevelInfo(logger *zap.Logger, loadPath string) *TableTplActivityLevelInfo {
	return &TableTplActivityLevelInfo{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplActivityLevelInfo),
	}
}

func (t *TableTplActivityLevelInfo) FindByKey(key string) (TplActivityLevelInfo, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplActivityLevelInfo) FindByFilter(f func(TplActivityLevelInfo) bool) ReadOnlyTplActivityLevelInfoSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplActivityLevelInfo, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplActivityLevelInfoSlice{data: result}
}

func (t *TableTplActivityLevelInfo) FindAll() ReadOnlyTplActivityLevelInfoSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplActivityLevelInfo, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplActivityLevelInfoSlice{data: result}
}

func (t *TableTplActivityLevelInfo) Release() {
	t.tableData = nil
}

func (t *TableTplActivityLevelInfo) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplActivityLevelInfoMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplActivityLevelInfo.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplActivityLevelInfoMap(fileContent, t.logger)
}

func DeserializeStringToTplActivityLevelInfoMap(jsonStr []byte, logger *zap.Logger) map[string]TplActivityLevelInfo {
	if len(jsonStr) == 0 {
		return make(map[string]TplActivityLevelInfo)
	}
	var jsonMap map[string]TplActivityLevelInfo
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplActivityLevelInfo)
	}
	return jsonMap
}
