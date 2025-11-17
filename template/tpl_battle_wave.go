package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplBattleWave struct {
	ID       int32   `json:"id"`
	LevelID  string  `json:"levelId"`
	LvUp     float64 `json:"lvUp"`
	Money    int32   `json:"money"`
	Progress int32   `json:"progress"`
}

// ReadOnlyTplBattleWaveSlice 只读TplBattleWave切片接口
type ReadOnlyTplBattleWaveSlice interface {
	Len() int
	Get(index int) TplBattleWave
	ToSlice() []TplBattleWave
}

// readOnlyTplBattleWaveSlice 只读TplBattleWave切片实现
type readOnlyTplBattleWaveSlice struct {
	data []TplBattleWave
}

func (r *readOnlyTplBattleWaveSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplBattleWaveSlice) Get(index int) TplBattleWave {
	if index < 0 || index >= len(r.data) {
		return TplBattleWave{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplBattleWaveSlice) ToSlice() []TplBattleWave {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplBattleWave, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplBattleWave struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplBattleWave
}

func NewTableTplBattleWave(logger *zap.Logger, loadPath string) *TableTplBattleWave {
	return &TableTplBattleWave{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplBattleWave),
	}
}

func (t *TableTplBattleWave) FindByKey(key string) (TplBattleWave, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplBattleWave) FindByFilter(f func(TplBattleWave) bool) ReadOnlyTplBattleWaveSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplBattleWave, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplBattleWaveSlice{data: result}
}

func (t *TableTplBattleWave) FindAll() ReadOnlyTplBattleWaveSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplBattleWave, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplBattleWaveSlice{data: result}
}

func (t *TableTplBattleWave) Release() {
	t.tableData = nil
}

func (t *TableTplBattleWave) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplBattleWaveMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplBattleWave.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplBattleWaveMap(fileContent, t.logger)
}

func DeserializeStringToTplBattleWaveMap(jsonStr []byte, logger *zap.Logger) map[string]TplBattleWave {
	if len(jsonStr) == 0 {
		return make(map[string]TplBattleWave)
	}
	var jsonMap map[string]TplBattleWave
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplBattleWave)
	}
	return jsonMap
}
