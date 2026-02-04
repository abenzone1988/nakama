package template

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"

	"go.uber.org/zap"
)

type TplChallenge struct {
	ActivityID    string `json:"activity_id"`
	CloseTime     string `json:"close_time"`
	EndTime       string `json:"end_time"`
	ID            int32  `json:"id"`
	MaxPart       int32  `json:"max_part"`
	Name          string `json:"name"`
	OpenTime      string `json:"open_time"`
	RewardRemains int32  `json:"reward_remains"`
	Status        int32  `json:"status"`
}

// ReadOnlyTplChallengeSlice 只读TplChallenge切片接口
type ReadOnlyTplChallengeSlice interface {
	Len() int
	Get(index int) TplChallenge
	ToSlice() []TplChallenge
}

// readOnlyTplChallengeSlice 只读TplChallenge切片实现
type readOnlyTplChallengeSlice struct {
	data []TplChallenge
}

func (r *readOnlyTplChallengeSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplChallengeSlice) Get(index int) TplChallenge {
	if index < 0 || index >= len(r.data) {
		return TplChallenge{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplChallengeSlice) ToSlice() []TplChallenge {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplChallenge, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplChallenge struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[int32]TplChallenge
}

func NewTableTplChallenge(logger *zap.Logger, loadPath string) *TableTplChallenge {
	return &TableTplChallenge{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[int32]TplChallenge),
	}
}

func (t *TableTplChallenge) FindByKey(key int32) (TplChallenge, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplChallenge) FindByFilter(f func(TplChallenge) bool) ReadOnlyTplChallengeSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplChallenge, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplChallengeSlice{data: result}
}

func (t *TableTplChallenge) FindAll() ReadOnlyTplChallengeSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplChallenge, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplChallengeSlice{data: result}
}

func (t *TableTplChallenge) Release() {
	t.tableData = nil
}

func (t *TableTplChallenge) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplChallengeMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplChallenge.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplChallengeMap(fileContent, t.logger)
}

func DeserializeStringToTplChallengeMap(jsonStr []byte, logger *zap.Logger) map[int32]TplChallenge {
	if len(jsonStr) == 0 {
		return make(map[int32]TplChallenge)
	}
	var jsonMap map[string]TplChallenge
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[int32]TplChallenge)
	}
	// 将 string key 转换为 int32 key
	result := make(map[int32]TplChallenge)
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
