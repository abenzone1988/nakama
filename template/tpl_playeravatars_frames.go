package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplPlayeravatars_frames struct {
	DurationTime    int32  `json:"duration_time"`
	DurationType    int32  `json:"duration_type"`
	ID              int32  `json:"id"`
	Name            string `json:"name"`
	Order           int32  `json:"order"`
	UnlockCondition string `json:"unlock_condition"`
	UnlockType      int32  `json:"unlock_type"`
	URL             string `json:"url"`
}

// ReadOnlyTplPlayeravatars_framesSlice 只读TplPlayeravatars_frames切片接口
type ReadOnlyTplPlayeravatars_framesSlice interface {
	Len() int
	Get(index int) TplPlayeravatars_frames
	ToSlice() []TplPlayeravatars_frames
}

// readOnlyTplPlayeravatars_framesSlice 只读TplPlayeravatars_frames切片实现
type readOnlyTplPlayeravatars_framesSlice struct {
	data []TplPlayeravatars_frames
}

func (r *readOnlyTplPlayeravatars_framesSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplPlayeravatars_framesSlice) Get(index int) TplPlayeravatars_frames {
	if index < 0 || index >= len(r.data) {
		return TplPlayeravatars_frames{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplPlayeravatars_framesSlice) ToSlice() []TplPlayeravatars_frames {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplPlayeravatars_frames, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplPlayeravatars_frames struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplPlayeravatars_frames
}

func NewTableTplPlayeravatars_frames(logger *zap.Logger, loadPath string) *TableTplPlayeravatars_frames {
	return &TableTplPlayeravatars_frames{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplPlayeravatars_frames),
	}
}

func (t *TableTplPlayeravatars_frames) FindByKey(key string) (TplPlayeravatars_frames, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplPlayeravatars_frames) FindByFilter(f func(TplPlayeravatars_frames) bool) ReadOnlyTplPlayeravatars_framesSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplPlayeravatars_frames, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplPlayeravatars_framesSlice{data: result}
}

func (t *TableTplPlayeravatars_frames) FindAll() ReadOnlyTplPlayeravatars_framesSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplPlayeravatars_frames, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplPlayeravatars_framesSlice{data: result}
}

func (t *TableTplPlayeravatars_frames) Release() {
	t.tableData = nil
}

func (t *TableTplPlayeravatars_frames) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplPlayeravatars_framesMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplPlayeravatars_frames.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplPlayeravatars_framesMap(fileContent, t.logger)
}

func DeserializeStringToTplPlayeravatars_framesMap(jsonStr []byte, logger *zap.Logger) map[string]TplPlayeravatars_frames {
	if len(jsonStr) == 0 {
		return make(map[string]TplPlayeravatars_frames)
	}
	var jsonMap map[string]TplPlayeravatars_frames
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplPlayeravatars_frames)
	}
	return jsonMap
}
