package template

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplPresetUnitDev struct {
	ID           string `json:"id"`
	Level        int32  `json:"level"`
	P1           string `json:"p1"`
	P2           string `json:"p2"`
	P3           int32  `json:"p3"`
	PresetUnitID string `json:"presetUnitId"`
	V1           int32  `json:"v1"`
	V2           int32  `json:"v2"`
	V3           int32  `json:"v3"`
}

type TableTplPresetUnitDev struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplPresetUnitDev
	result    []TplPresetUnitDev
}

func NewTableTplPresetUnitDev(logger *zap.Logger, loadPath string) *TableTplPresetUnitDev {
	return &TableTplPresetUnitDev{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplPresetUnitDev),
		result:    make([]TplPresetUnitDev, 0),
	}
}

func (t *TableTplPresetUnitDev) FindByKey(key interface{}) (TplPresetUnitDev, bool) {
	val, ok := t.tableData[fmt.Sprintf("%v", key)]
	return val, ok
}

func (t *TableTplPresetUnitDev) FindByFilter(f func(TplPresetUnitDev) bool) []TplPresetUnitDev {
	t.result = make([]TplPresetUnitDev, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplPresetUnitDev) FindAll() []TplPresetUnitDev {
	t.result = make([]TplPresetUnitDev, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplPresetUnitDev) Release() {
	t.tableData = nil
}

func (t *TableTplPresetUnitDev) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplPresetUnitDevMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplPresetUnitDev.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplPresetUnitDevMap(fileContent, t.logger)
}

func DeserializeStringToTplPresetUnitDevMap(jsonStr []byte, logger *zap.Logger) map[string]TplPresetUnitDev {
	if len(jsonStr) == 0 {
		return make(map[string]TplPresetUnitDev)
	}
	var jsonMap map[string]TplPresetUnitDev
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplPresetUnitDev)
	}
	return jsonMap
}
