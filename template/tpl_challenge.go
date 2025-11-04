package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

// ReadOnlyChallengeSlice 只读挑战赛切片接口
type ReadOnlyChallengeSlice interface {
	Len() int
	Get(index int) TplChallenge
	ToSlice() []TplChallenge
}

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

// readOnlyChallengeSlice 只读挑战赛切片实现
type readOnlyChallengeSlice struct {
	data []TplChallenge
}

func (r *readOnlyChallengeSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyChallengeSlice) Get(index int) TplChallenge {
	if index < 0 || index >= len(r.data) {
		return TplChallenge{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyChallengeSlice) ToSlice() []TplChallenge {
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

func (t *TableTplChallenge) FindByFilter(f func(TplChallenge) bool) ReadOnlyChallengeSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplChallenge, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyChallengeSlice{data: result}
}

func (t *TableTplChallenge) FindAll() ReadOnlyChallengeSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplChallenge, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyChallengeSlice{data: result}
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
	var jsonMap map[int32]TplChallenge
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[int32]TplChallenge)
	}
	return jsonMap
}
