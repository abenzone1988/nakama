package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplMonster struct {
	AtkRange       int32  `json:"atkRange"`
	AtkSpeed       int32  `json:"atkSpeed"`
	BurnImmune     int32  `json:"burnImmune"`
	ElectResist    int32  `json:"electResist"`
	EnergyResist   int32  `json:"energyResist"`
	FaintImmune    int32  `json:"faintImmune"`
	FireResist     int32  `json:"fireResist"`
	Flee           int32  `json:"flee"`
	HitbackImmune  int32  `json:"hitbackImmune"`
	ID             string `json:"id"`
	PalsyImmune    int32  `json:"palsyImmune"`
	PhysResist     int32  `json:"physResist"`
	RootImmune     int32  `json:"rootImmune"`
	Score          int32  `json:"score"`
	SlowDownImmune int32  `json:"slowDownImmune"`
}

// ReadOnlyTplMonsterSlice 只读TplMonster切片接口
type ReadOnlyTplMonsterSlice interface {
	Len() int
	Get(index int) TplMonster
	ToSlice() []TplMonster
}

// readOnlyTplMonsterSlice 只读TplMonster切片实现
type readOnlyTplMonsterSlice struct {
	data []TplMonster
}

func (r *readOnlyTplMonsterSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplMonsterSlice) Get(index int) TplMonster {
	if index < 0 || index >= len(r.data) {
		return TplMonster{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplMonsterSlice) ToSlice() []TplMonster {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplMonster, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplMonster struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplMonster
}

func NewTableTplMonster(logger *zap.Logger, loadPath string) *TableTplMonster {
	return &TableTplMonster{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplMonster),
	}
}

func (t *TableTplMonster) FindByKey(key string) (TplMonster, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplMonster) FindByFilter(f func(TplMonster) bool) ReadOnlyTplMonsterSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplMonster, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplMonsterSlice{data: result}
}

func (t *TableTplMonster) FindAll() ReadOnlyTplMonsterSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplMonster, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplMonsterSlice{data: result}
}

func (t *TableTplMonster) Release() {
	t.tableData = nil
}

func (t *TableTplMonster) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplMonsterMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplMonster.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplMonsterMap(fileContent, t.logger)
}

func DeserializeStringToTplMonsterMap(jsonStr []byte, logger *zap.Logger) map[string]TplMonster {
	if len(jsonStr) == 0 {
		return make(map[string]TplMonster)
	}
	var jsonMap map[string]TplMonster
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplMonster)
	}

	return jsonMap
}
