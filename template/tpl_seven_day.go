package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplSevenDay struct {
	Day         int32  `json:"day"`
	DicountRate string `json:"dicount_rate"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Reward      string `json:"reward"`
}

// ReadOnlyTplSevenDaySlice 只读TplSevenDay切片接口
type ReadOnlyTplSevenDaySlice interface {
	Len() int
	Get(index int) TplSevenDay
	ToSlice() []TplSevenDay
}

// readOnlyTplSevenDaySlice 只读TplSevenDay切片实现
type readOnlyTplSevenDaySlice struct {
	data []TplSevenDay
}

func (r *readOnlyTplSevenDaySlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplSevenDaySlice) Get(index int) TplSevenDay {
	if index < 0 || index >= len(r.data) {
		return TplSevenDay{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplSevenDaySlice) ToSlice() []TplSevenDay {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplSevenDay, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplSevenDay struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplSevenDay
}

func NewTableTplSevenDay(logger *zap.Logger, loadPath string) *TableTplSevenDay {
	return &TableTplSevenDay{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplSevenDay),
	}
}

func (t *TableTplSevenDay) FindByKey(key string) (TplSevenDay, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplSevenDay) FindByFilter(f func(TplSevenDay) bool) ReadOnlyTplSevenDaySlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplSevenDay, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplSevenDaySlice{data: result}
}

func (t *TableTplSevenDay) FindAll() ReadOnlyTplSevenDaySlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplSevenDay, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplSevenDaySlice{data: result}
}

func (t *TableTplSevenDay) Release() {
	t.tableData = nil
}

func (t *TableTplSevenDay) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplSevenDayMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplSevenDay.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplSevenDayMap(fileContent, t.logger)
}

func DeserializeStringToTplSevenDayMap(jsonStr []byte, logger *zap.Logger) map[string]TplSevenDay {
	if len(jsonStr) == 0 {
		return make(map[string]TplSevenDay)
	}
	var jsonMap map[string]TplSevenDay
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplSevenDay)
	}
	return jsonMap
}
