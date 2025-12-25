package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplEliteLevelDrop struct {
	Dropmax        int32  `json:"dropmax"`
	Dropmin        int32  `json:"dropmin"`
	ID             string `json:"id"`
	Quality1weight int32  `json:"quality1weight"`
	Quality2weight int32  `json:"quality2weight"`
	Quality3weight int32  `json:"quality3weight"`
	Quality4weight int32  `json:"quality4weight"`
	Quality5weight int32  `json:"quality5weight"`
}

// ReadOnlyTplEliteLevelDropSlice 只读TplEliteLevelDrop切片接口
type ReadOnlyTplEliteLevelDropSlice interface {
	Len() int
	Get(index int) TplEliteLevelDrop
	ToSlice() []TplEliteLevelDrop
}

// readOnlyTplEliteLevelDropSlice 只读TplEliteLevelDrop切片实现
type readOnlyTplEliteLevelDropSlice struct {
	data []TplEliteLevelDrop
}

func (r *readOnlyTplEliteLevelDropSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplEliteLevelDropSlice) Get(index int) TplEliteLevelDrop {
	if index < 0 || index >= len(r.data) {
		return TplEliteLevelDrop{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplEliteLevelDropSlice) ToSlice() []TplEliteLevelDrop {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplEliteLevelDrop, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplEliteLevelDrop struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplEliteLevelDrop
}

func NewTableTplEliteLevelDrop(logger *zap.Logger, loadPath string) *TableTplEliteLevelDrop {
	return &TableTplEliteLevelDrop{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplEliteLevelDrop),
	}
}

func (t *TableTplEliteLevelDrop) FindByKey(key string) (TplEliteLevelDrop, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplEliteLevelDrop) FindByFilter(f func(TplEliteLevelDrop) bool) ReadOnlyTplEliteLevelDropSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplEliteLevelDrop, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplEliteLevelDropSlice{data: result}
}

func (t *TableTplEliteLevelDrop) FindAll() ReadOnlyTplEliteLevelDropSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplEliteLevelDrop, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplEliteLevelDropSlice{data: result}
}

func (t *TableTplEliteLevelDrop) Release() {
	t.tableData = nil
}

func (t *TableTplEliteLevelDrop) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplEliteLevelDropMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplEliteLevelDrop.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplEliteLevelDropMap(fileContent, t.logger)
}

func DeserializeStringToTplEliteLevelDropMap(jsonStr []byte, logger *zap.Logger) map[string]TplEliteLevelDrop {
	if len(jsonStr) == 0 {
		return make(map[string]TplEliteLevelDrop)
	}
	var jsonMap map[string]TplEliteLevelDrop
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplEliteLevelDrop)
	}
	return jsonMap
}
