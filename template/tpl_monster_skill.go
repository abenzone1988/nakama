package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplMonsterSkill struct {
	ID    string `json:"id"`
	Param int32  `json:"param"`
	Type  int32  `json:"type"`
}

// ReadOnlyTplMonsterSkillSlice 只读TplMonsterSkill切片接口
type ReadOnlyTplMonsterSkillSlice interface {
	Len() int
	Get(index int) TplMonsterSkill
	ToSlice() []TplMonsterSkill
}

// readOnlyTplMonsterSkillSlice 只读TplMonsterSkill切片实现
type readOnlyTplMonsterSkillSlice struct {
	data []TplMonsterSkill
}

func (r *readOnlyTplMonsterSkillSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplMonsterSkillSlice) Get(index int) TplMonsterSkill {
	if index < 0 || index >= len(r.data) {
		return TplMonsterSkill{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplMonsterSkillSlice) ToSlice() []TplMonsterSkill {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplMonsterSkill, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplMonsterSkill struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplMonsterSkill
}

func NewTableTplMonsterSkill(logger *zap.Logger, loadPath string) *TableTplMonsterSkill {
	return &TableTplMonsterSkill{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplMonsterSkill),
	}
}

func (t *TableTplMonsterSkill) FindByKey(key string) (TplMonsterSkill, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplMonsterSkill) FindByFilter(f func(TplMonsterSkill) bool) ReadOnlyTplMonsterSkillSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplMonsterSkill, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplMonsterSkillSlice{data: result}
}

func (t *TableTplMonsterSkill) FindAll() ReadOnlyTplMonsterSkillSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplMonsterSkill, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplMonsterSkillSlice{data: result}
}

func (t *TableTplMonsterSkill) Release() {
	t.tableData = nil
}

func (t *TableTplMonsterSkill) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplMonsterSkillMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplMonsterSkill.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplMonsterSkillMap(fileContent, t.logger)
}

func DeserializeStringToTplMonsterSkillMap(jsonStr []byte, logger *zap.Logger) map[string]TplMonsterSkill {
	if len(jsonStr) == 0 {
		return make(map[string]TplMonsterSkill)
	}
	var jsonMap map[string]TplMonsterSkill
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplMonsterSkill)
	}

	return jsonMap
}
