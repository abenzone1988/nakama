package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplRedemption struct {
	ID       string `json:"id"`
	RewardID string `json:"rewardId"`
}

// ReadOnlyTplRedemptionSlice 只读TplRedemption切片接口
type ReadOnlyTplRedemptionSlice interface {
	Len() int
	Get(index int) TplRedemption
	ToSlice() []TplRedemption
}

// readOnlyTplRedemptionSlice 只读TplRedemption切片实现
type readOnlyTplRedemptionSlice struct {
	data []TplRedemption
}

func (r *readOnlyTplRedemptionSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplRedemptionSlice) Get(index int) TplRedemption {
	if index < 0 || index >= len(r.data) {
		return TplRedemption{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplRedemptionSlice) ToSlice() []TplRedemption {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplRedemption, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplRedemption struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplRedemption
}

func NewTableTplRedemption(logger *zap.Logger, loadPath string) *TableTplRedemption {
	return &TableTplRedemption{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplRedemption),
	}
}

func (t *TableTplRedemption) FindByKey(key string) (TplRedemption, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplRedemption) FindByFilter(f func(TplRedemption) bool) ReadOnlyTplRedemptionSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplRedemption, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplRedemptionSlice{data: result}
}

func (t *TableTplRedemption) FindAll() ReadOnlyTplRedemptionSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplRedemption, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplRedemptionSlice{data: result}
}

func (t *TableTplRedemption) Release() {
	t.tableData = nil
}

func (t *TableTplRedemption) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplRedemptionMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplRedemption.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplRedemptionMap(fileContent, t.logger)
}

func DeserializeStringToTplRedemptionMap(jsonStr []byte, logger *zap.Logger) map[string]TplRedemption {
	if len(jsonStr) == 0 {
		return make(map[string]TplRedemption)
	}
	var jsonMap map[string]TplRedemption
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplRedemption)
	}
	return jsonMap
}
