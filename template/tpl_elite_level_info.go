package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplEliteLevelInfo struct {
	LevelMonsterDev    string   `json:"LevelMonsterDev"`
	Cost               int32    `json:"cost"`
	DropID             string   `json:"dropId"`
	EquipBuffList      []string `json:"equipBuffList"`
	FirstRewards       string   `json:"firstRewards"`
	ID                 string   `json:"id"`
	MainlevelProgress  string   `json:"mainlevelProgress"`
	MonsterBuffList    []string `json:"monsterBuffList"`
	MonsterLevel       int32    `json:"monsterLevel"`
	MonsterWaveGroupID string   `json:"monsterWaveGroupId"`
}

// ReadOnlyTplEliteLevelInfoSlice 只读TplEliteLevelInfo切片接口
type ReadOnlyTplEliteLevelInfoSlice interface {
	Len() int
	Get(index int) TplEliteLevelInfo
	ToSlice() []TplEliteLevelInfo
}

// readOnlyTplEliteLevelInfoSlice 只读TplEliteLevelInfo切片实现
type readOnlyTplEliteLevelInfoSlice struct {
	data []TplEliteLevelInfo
}

func (r *readOnlyTplEliteLevelInfoSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplEliteLevelInfoSlice) Get(index int) TplEliteLevelInfo {
	if index < 0 || index >= len(r.data) {
		return TplEliteLevelInfo{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplEliteLevelInfoSlice) ToSlice() []TplEliteLevelInfo {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplEliteLevelInfo, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplEliteLevelInfo struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplEliteLevelInfo
}

func NewTableTplEliteLevelInfo(logger *zap.Logger, loadPath string) *TableTplEliteLevelInfo {
	return &TableTplEliteLevelInfo{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplEliteLevelInfo),
	}
}

func (t *TableTplEliteLevelInfo) FindByKey(key string) (TplEliteLevelInfo, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplEliteLevelInfo) FindByFilter(f func(TplEliteLevelInfo) bool) ReadOnlyTplEliteLevelInfoSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplEliteLevelInfo, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplEliteLevelInfoSlice{data: result}
}

func (t *TableTplEliteLevelInfo) FindAll() ReadOnlyTplEliteLevelInfoSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplEliteLevelInfo, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplEliteLevelInfoSlice{data: result}
}

func (t *TableTplEliteLevelInfo) Release() {
	t.tableData = nil
}

func (t *TableTplEliteLevelInfo) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplEliteLevelInfoMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplEliteLevelInfo.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplEliteLevelInfoMap(fileContent, t.logger)
}

func DeserializeStringToTplEliteLevelInfoMap(jsonStr []byte, logger *zap.Logger) map[string]TplEliteLevelInfo {
	if len(jsonStr) == 0 {
		return make(map[string]TplEliteLevelInfo)
	}
	var jsonMap map[string]TplEliteLevelInfo
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplEliteLevelInfo)
	}
	return jsonMap
}
