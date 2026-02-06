package template

import (
	"encoding/json"
	"go.uber.org/zap"
	"os"
	"path/filepath"
	"strconv"
)

type TplMonsterDev struct {
	AtkScale float64 `json:"atkScale"`
	HpScale  float64 `json:"hpScale"`
	ID       int32   `json:"id"`
}

// ReadOnlyTplMonsterDevSlice 只读TplMonsterDev切片接口
type ReadOnlyTplMonsterDevSlice interface {
	Len() int
	Get(index int) TplMonsterDev
	ToSlice() []TplMonsterDev
}

// readOnlyTplMonsterDevSlice 只读TplMonsterDev切片实现
type readOnlyTplMonsterDevSlice struct {
	data []TplMonsterDev
}

func (r *readOnlyTplMonsterDevSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplMonsterDevSlice) Get(index int) TplMonsterDev {
	if index < 0 || index >= len(r.data) {
		return TplMonsterDev{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplMonsterDevSlice) ToSlice() []TplMonsterDev {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplMonsterDev, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplMonsterDev struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[int32]TplMonsterDev
}

func NewTableTplMonsterDev(logger *zap.Logger, loadPath string) *TableTplMonsterDev {
	return &TableTplMonsterDev{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[int32]TplMonsterDev),
	}
}

func (t *TableTplMonsterDev) FindByKey(key int32) (TplMonsterDev, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplMonsterDev) FindByFilter(f func(TplMonsterDev) bool) ReadOnlyTplMonsterDevSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplMonsterDev, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplMonsterDevSlice{data: result}
}

func (t *TableTplMonsterDev) FindAll() ReadOnlyTplMonsterDevSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplMonsterDev, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplMonsterDevSlice{data: result}
}

func (t *TableTplMonsterDev) Release() {
	t.tableData = nil
}

func (t *TableTplMonsterDev) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplMonsterDevMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplMonsterDev.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplMonsterDevMap(fileContent, t.logger)
}

func DeserializeStringToTplMonsterDevMap(jsonStr []byte, logger *zap.Logger) map[int32]TplMonsterDev {
	if len(jsonStr) == 0 {
		return make(map[int32]TplMonsterDev)
	}
	var jsonMap map[string]TplMonsterDev
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[int32]TplMonsterDev)
	}
	// 将 string key 转换为 int32 key
	result := make(map[int32]TplMonsterDev)
	for k, v := range jsonMap {
		intKey, err := strconv.Atoi(k)
		if err != nil {
			logger.Error("转换键为 int32 失败", zap.String("key", k), zap.Error(err))
			continue
		}
		result[int32(intKey)] = v
	}
	return result
}
