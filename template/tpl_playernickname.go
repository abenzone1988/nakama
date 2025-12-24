package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplPlayernickname struct {
	Desc string `json:"desc"`
	ID   int32  `json:"id"`
	Male int32  `json:"male"`
	Type int32  `json:"type"`
}

// ReadOnlyTplPlayernicknameSlice 只读TplPlayernickname切片接口
type ReadOnlyTplPlayernicknameSlice interface {
	Len() int
	Get(index int) TplPlayernickname
	ToSlice() []TplPlayernickname
}

// readOnlyTplPlayernicknameSlice 只读TplPlayernickname切片实现
type readOnlyTplPlayernicknameSlice struct {
	data []TplPlayernickname
}

func (r *readOnlyTplPlayernicknameSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplPlayernicknameSlice) Get(index int) TplPlayernickname {
	if index < 0 || index >= len(r.data) {
		return TplPlayernickname{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplPlayernicknameSlice) ToSlice() []TplPlayernickname {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplPlayernickname, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplPlayernickname struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplPlayernickname
}

func NewTableTplPlayernickname(logger *zap.Logger, loadPath string) *TableTplPlayernickname {
	return &TableTplPlayernickname{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplPlayernickname),
	}
}

func (t *TableTplPlayernickname) FindByKey(key string) (TplPlayernickname, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplPlayernickname) FindByFilter(f func(TplPlayernickname) bool) ReadOnlyTplPlayernicknameSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplPlayernickname, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplPlayernicknameSlice{data: result}
}

func (t *TableTplPlayernickname) FindAll() ReadOnlyTplPlayernicknameSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplPlayernickname, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplPlayernicknameSlice{data: result}
}

func (t *TableTplPlayernickname) Release() {
	t.tableData = nil
}

func (t *TableTplPlayernickname) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplPlayernicknameMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplPlayernickname.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplPlayernicknameMap(fileContent, t.logger)
}

func DeserializeStringToTplPlayernicknameMap(jsonStr []byte, logger *zap.Logger) map[string]TplPlayernickname {
	if len(jsonStr) == 0 {
		return make(map[string]TplPlayernickname)
	}
	var jsonMap map[string]TplPlayernickname
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplPlayernickname)
	}
	return jsonMap
}
