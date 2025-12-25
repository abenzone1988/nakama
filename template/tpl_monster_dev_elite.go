package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplMonsterDev_Elite struct {
	AtkScale float64 `json:"atkScale"`
	HpScale  float64 `json:"hpScale"`
	ID       int32   `json:"id"`
}

// ReadOnlyTplMonsterDev_EliteSlice 只读TplMonsterDev_Elite切片接口
type ReadOnlyTplMonsterDev_EliteSlice interface {
	Len() int
	Get(index int) TplMonsterDev_Elite
	ToSlice() []TplMonsterDev_Elite
}

// readOnlyTplMonsterDev_EliteSlice 只读TplMonsterDev_Elite切片实现
type readOnlyTplMonsterDev_EliteSlice struct {
	data []TplMonsterDev_Elite
}

func (r *readOnlyTplMonsterDev_EliteSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplMonsterDev_EliteSlice) Get(index int) TplMonsterDev_Elite {
	if index < 0 || index >= len(r.data) {
		return TplMonsterDev_Elite{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplMonsterDev_EliteSlice) ToSlice() []TplMonsterDev_Elite {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplMonsterDev_Elite, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplMonsterDev_Elite struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplMonsterDev_Elite
}

func NewTableTplMonsterDev_Elite(logger *zap.Logger, loadPath string) *TableTplMonsterDev_Elite {
	return &TableTplMonsterDev_Elite{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplMonsterDev_Elite),
	}
}

func (t *TableTplMonsterDev_Elite) FindByKey(key string) (TplMonsterDev_Elite, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplMonsterDev_Elite) FindByFilter(f func(TplMonsterDev_Elite) bool) ReadOnlyTplMonsterDev_EliteSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplMonsterDev_Elite, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplMonsterDev_EliteSlice{data: result}
}

func (t *TableTplMonsterDev_Elite) FindAll() ReadOnlyTplMonsterDev_EliteSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplMonsterDev_Elite, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplMonsterDev_EliteSlice{data: result}
}

func (t *TableTplMonsterDev_Elite) Release() {
	t.tableData = nil
}

func (t *TableTplMonsterDev_Elite) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplMonsterDev_EliteMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplMonsterDev_Elite.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplMonsterDev_EliteMap(fileContent, t.logger)
}

func DeserializeStringToTplMonsterDev_EliteMap(jsonStr []byte, logger *zap.Logger) map[string]TplMonsterDev_Elite {
	if len(jsonStr) == 0 {
		return make(map[string]TplMonsterDev_Elite)
	}
	var jsonMap map[string]TplMonsterDev_Elite
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplMonsterDev_Elite)
	}
	return jsonMap
}
