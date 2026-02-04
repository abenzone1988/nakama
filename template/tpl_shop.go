package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

type TplShop struct {
	AdProductCount          int32  `json:"adProductCount"`
	CoinProductCount        int32  `json:"coinProductCount"`
	CoinProductDiscountRate string `json:"coinProductDiscountRate"`
	FreeProductCount        int32  `json:"freeProductCount"`
	GemProductCount         int32  `json:"gemProductCount"`
	GemProductDiscountRate  string `json:"gemProductDiscountRate"`
	HandleRefreshTimes      int32  `json:"handleRefreshTimes"`
	ID                      string `json:"id"`
	MoneyProductCount       int32  `json:"moneyProductCount"`
	Name                    string `json:"name"`
	PermanentProduct        string `json:"permanentProduct"`
}

// ReadOnlyTplShopSlice 只读TplShop切片接口
type ReadOnlyTplShopSlice interface {
	Len() int
	Get(index int) TplShop
	ToSlice() []TplShop
}

// readOnlyTplShopSlice 只读TplShop切片实现
type readOnlyTplShopSlice struct {
	data []TplShop
}

func (r *readOnlyTplShopSlice) Len() int {
	return len(r.data)
}

func (r *readOnlyTplShopSlice) Get(index int) TplShop {
	if index < 0 || index >= len(r.data) {
		return TplShop{} // 返回零值
	}
	return r.data[index]
}

func (r *readOnlyTplShopSlice) ToSlice() []TplShop {
	// 返回副本，确保外部无法修改原始数据
	result := make([]TplShop, len(r.data))
	copy(result, r.data)
	return result
}

type TableTplShop struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplShop
}

func NewTableTplShop(logger *zap.Logger, loadPath string) *TableTplShop {
	return &TableTplShop{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplShop),
	}
}

func (t *TableTplShop) FindByKey(key string) (TplShop, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplShop) FindByFilter(f func(TplShop) bool) ReadOnlyTplShopSlice {
	// 创建新的切片，避免共享字段竞争
	result := make([]TplShop, 0)
	for _, item := range t.tableData {
		if f(item) {
			result = append(result, item)
		}
	}
	return &readOnlyTplShopSlice{data: result}
}

func (t *TableTplShop) FindAll() ReadOnlyTplShopSlice {
	// 直接从 tableData 创建切片，避免共享字段竞争
	// 返回只读切片，调用者无法修改原始数据
	result := make([]TplShop, 0, len(t.tableData))
	for _, item := range t.tableData {
		result = append(result, item)
	}
	return &readOnlyTplShopSlice{data: result}
}

func (t *TableTplShop) Release() {
	t.tableData = nil
}

func (t *TableTplShop) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplShopMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplShop.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplShopMap(fileContent, t.logger)
}

func DeserializeStringToTplShopMap(jsonStr []byte, logger *zap.Logger) map[string]TplShop {
	if len(jsonStr) == 0 {
		return make(map[string]TplShop)
	}
	var jsonMap map[string]TplShop
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplShop)
	}

	return jsonMap
}
