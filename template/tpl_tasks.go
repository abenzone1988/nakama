package template

import (
	"encoding/json"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

type TplTasks struct {
	Liveness    int32  `json:"Liveness"`
	TargetType  string `json:"TargetType"`
	TargetValue int32  `json:"TargetValue"`
	Brief       string `json:"brief"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Reward      string `json:"reward"`
	TaskType    int32  `json:"taskType"`
}

type TableTplTasks struct {
	logger    *zap.Logger
	loadPath  string
	tableData map[string]TplTasks
	result    []TplTasks
}

func NewTableTplTasks(logger *zap.Logger, loadPath string) *TableTplTasks {
	return &TableTplTasks{
		logger:    logger,
		loadPath:  loadPath,
		tableData: make(map[string]TplTasks),
		result:    make([]TplTasks, 0),
	}
}

func (t *TableTplTasks) FindByKey(key string) (TplTasks, bool) {
	val, ok := t.tableData[key]
	return val, ok
}

func (t *TableTplTasks) FindByFilter(f func(TplTasks) bool) []TplTasks {
	t.result = make([]TplTasks, 0)
	for _, item := range t.tableData {
		if f(item) {
			t.result = append(t.result, item)
		}
	}
	return t.result
}

func (t *TableTplTasks) FindAll() []TplTasks {
	t.result = make([]TplTasks, 0)
	for _, item := range t.tableData {
		t.result = append(t.result, item)
	}
	return t.result
}

func (t *TableTplTasks) Release() {
	t.tableData = nil
}

func (t *TableTplTasks) LoadData(content []byte) {
	if content != nil {
		t.tableData = DeserializeStringToTplTasksMap(content, t.logger)
		return
	}
	path := filepath.Join(t.loadPath, "TplTasks.json")
	fileContent, err := os.ReadFile(path)
	if err != nil {
		t.logger.Error("读取文件错误", zap.Error(err))
		return
	}

	t.tableData = DeserializeStringToTplTasksMap(fileContent, t.logger)
}

func DeserializeStringToTplTasksMap(jsonStr []byte, logger *zap.Logger) map[string]TplTasks {
	if len(jsonStr) == 0 {
		return make(map[string]TplTasks)
	}
	var jsonMap map[string]TplTasks
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		logger.Error("JSON 反序列化错误", zap.Error(err))
		return make(map[string]TplTasks)
	}
	return jsonMap
}
