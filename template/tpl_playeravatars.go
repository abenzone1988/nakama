package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplPlayeravatars struct {
	ID              int32  `json:"id"`
	NailURL         string `json:"nail_url"`
	Name            string `json:"name"`
	Order           int32  `json:"order"`
	UnlockCondition string `json:"unlock_condition"`
	UnlockType      int32  `json:"unlock_type"`
	URL             string `json:"url"`
}

// ReadOnlyTplPlayeravatarsSlice 只读TplPlayeravatars切片接口
type ReadOnlyTplPlayeravatarsSlice interface {
	Len() int
	Get(index int) TplPlayeravatars
	ToSlice() []TplPlayeravatars
}

// readOnlyTplPlayeravatarsSlice 只读TplPlayeravatars切片实现
type readOnlyTplPlayeravatarsSlice struct {
	data []TplPlayeravatars
}

func (r *readOnlyTplPlayeravatarsSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplPlayeravatarsSlice) Get(index int) TplPlayeravatars {
	if index < 0 || index >= len(r.data) {
		return TplPlayeravatars{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplPlayeravatarsSlice) ToSlice() []TplPlayeravatars {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplPlayeravatars, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplPlayeravatars struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplPlayeravatars
}

func NewTableTplPlayeravatars(logger *zap.Logger, loadPath string) *TableTplPlayeravatars {
	return &TableTplPlayeravatars{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplPlayeravatars),
	}
}

func (t *TableTplPlayeravatars) FindByKey(key string) (TplPlayeravatars, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplPlayeravatars) FindByFilter(f func(TplPlayeravatars) bool) ReadOnlyTplPlayeravatarsSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplPlayeravatars, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplPlayeravatarsSlice{data: result}
}

func (t *TableTplPlayeravatars) FindAll() ReadOnlyTplPlayeravatarsSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplPlayeravatars, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplPlayeravatarsSlice{data: result}
}

func (t *TableTplPlayeravatars) Release() {
	t.tableData = nil
}

func (t *TableTplPlayeravatars) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplPlayeravatarsMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplPlayeravatars.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplPlayeravatarsMap(fileContent, t.logger)
}

func DeserializeStringToTplPlayeravatarsMap(jsonStr []byte, logger *zap.Logger) map[string]TplPlayeravatars {
	if len(jsonStr) == 0 {
		return make(map[string]TplPlayeravatars)
	}
	var jsonMap map[string]TplPlayeravatars
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplPlayeravatars)
	}
	return jsonMap
}
