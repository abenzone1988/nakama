package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplEquipmentComposite struct {
	ID        string `json:"id"`
	Primary   string `json:"primary"`
	Secondary string `json:"secondary"`
}

// ReadOnlyTplEquipmentCompositeSlice 只读TplEquipmentComposite切片接口
type ReadOnlyTplEquipmentCompositeSlice interface {
	Len() int
	Get(index int) TplEquipmentComposite
	ToSlice() []TplEquipmentComposite
}

// readOnlyTplEquipmentCompositeSlice 只读TplEquipmentComposite切片实现
type readOnlyTplEquipmentCompositeSlice struct {
	data []TplEquipmentComposite
}

func (r *readOnlyTplEquipmentCompositeSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplEquipmentCompositeSlice) Get(index int) TplEquipmentComposite {
	if index < 0 || index >= len(r.data) {
		return TplEquipmentComposite{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplEquipmentCompositeSlice) ToSlice() []TplEquipmentComposite {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplEquipmentComposite, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplEquipmentComposite struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplEquipmentComposite
}

func NewTableTplEquipmentComposite(logger *zap.Logger, loadPath string) *TableTplEquipmentComposite {
	return &TableTplEquipmentComposite{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplEquipmentComposite),
	}
}

func (t *TableTplEquipmentComposite) FindByKey(key string) (TplEquipmentComposite, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplEquipmentComposite) FindByFilter(f func(TplEquipmentComposite) bool) ReadOnlyTplEquipmentCompositeSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplEquipmentComposite, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplEquipmentCompositeSlice{data: result}
}

func (t *TableTplEquipmentComposite) FindAll() ReadOnlyTplEquipmentCompositeSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplEquipmentComposite, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplEquipmentCompositeSlice{data: result}
}

func (t *TableTplEquipmentComposite) Release() {
	t.tableData = nil
}

func (t *TableTplEquipmentComposite) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplEquipmentCompositeMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplEquipmentComposite.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplEquipmentCompositeMap(fileContent, t.logger)
}

func DeserializeStringToTplEquipmentCompositeMap(jsonStr []byte, logger *zap.Logger) map[string]TplEquipmentComposite {
	if len(jsonStr) == 0 {
		return make(map[string]TplEquipmentComposite)
	}
	var jsonMap map[string]TplEquipmentComposite
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplEquipmentComposite)
	}

	return jsonMap
}
