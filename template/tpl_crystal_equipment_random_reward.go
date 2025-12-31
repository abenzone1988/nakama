package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplCrystalEquipmentRandomReward struct {
	Dropmax        int32  `json:"dropmax"`
	Dropmin        int32  `json:"dropmin"`
	ID             string `json:"id"`
	Quality1weight int32  `json:"quality1weight"`
	Quality2weight int32  `json:"quality2weight"`
	Quality3weight int32  `json:"quality3weight"`
	Quality4weight int32  `json:"quality4weight"`
	Quality5weight int32  `json:"quality5weight"`
}

// ReadOnlyTplCrystalEquipmentRandomRewardSlice 只读TplCrystalEquipmentRandomReward切片接口
type ReadOnlyTplCrystalEquipmentRandomRewardSlice interface {
	Len() int
	Get(index int) TplCrystalEquipmentRandomReward
	ToSlice() []TplCrystalEquipmentRandomReward
}

// readOnlyTplCrystalEquipmentRandomRewardSlice 只读TplCrystalEquipmentRandomReward切片实现
type readOnlyTplCrystalEquipmentRandomRewardSlice struct {
	data []TplCrystalEquipmentRandomReward
}

func (r *readOnlyTplCrystalEquipmentRandomRewardSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplCrystalEquipmentRandomRewardSlice) Get(index int) TplCrystalEquipmentRandomReward {
	if index < 0 || index >= len(r.data) {
		return TplCrystalEquipmentRandomReward{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplCrystalEquipmentRandomRewardSlice) ToSlice() []TplCrystalEquipmentRandomReward {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplCrystalEquipmentRandomReward, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplCrystalEquipmentRandomReward struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplCrystalEquipmentRandomReward
}

func NewTableTplCrystalEquipmentRandomReward(logger *zap.Logger, loadPath string) *TableTplCrystalEquipmentRandomReward {
	return &TableTplCrystalEquipmentRandomReward{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplCrystalEquipmentRandomReward),
	}
}

func (t *TableTplCrystalEquipmentRandomReward) FindByKey(key string) (TplCrystalEquipmentRandomReward, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplCrystalEquipmentRandomReward) FindByFilter(f func(TplCrystalEquipmentRandomReward) bool) ReadOnlyTplCrystalEquipmentRandomRewardSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplCrystalEquipmentRandomReward, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplCrystalEquipmentRandomRewardSlice{data: result}
}

func (t *TableTplCrystalEquipmentRandomReward) FindAll() ReadOnlyTplCrystalEquipmentRandomRewardSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplCrystalEquipmentRandomReward, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplCrystalEquipmentRandomRewardSlice{data: result}
}

func (t *TableTplCrystalEquipmentRandomReward) Release() {
	t.tableData = nil
}

func (t *TableTplCrystalEquipmentRandomReward) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplCrystalEquipmentRandomRewardMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplCrystalEquipmentRandomReward.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplCrystalEquipmentRandomRewardMap(fileContent, t.logger)
}

func DeserializeStringToTplCrystalEquipmentRandomRewardMap(jsonStr []byte, logger *zap.Logger) map[string]TplCrystalEquipmentRandomReward {
	if len(jsonStr) == 0 {
		return make(map[string]TplCrystalEquipmentRandomReward)
	}
	var jsonMap map[string]TplCrystalEquipmentRandomReward
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplCrystalEquipmentRandomReward)
	}
	return jsonMap
}
