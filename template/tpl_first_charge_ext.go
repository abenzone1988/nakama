package template

// 本文件是 tpl_first_charge.go（由 nakama-tool/json2go 自动生成）的手写扩展。
// 生成器只会覆盖 tpl_first_charge.go，不会删除或改写本文件，因此表专属的查询方法放这里。
// 禁止把方法写进 tpl_first_charge.go —— 重新导表后会被整文件覆盖。

// FindByDay 按天数查找首充奖励配置（day 从 1 开始，对应首充礼包第几天）。
func (t *TableTplFirstCharge) FindByDay(day int32) (TplFirstCharge, bool) {
	if t.tableData == nil {
		return TplFirstCharge{}, false
	}
	for _, item := range t.tableData {
		if item.Day == day {
			return item, true
		}
	}
	return TplFirstCharge{}, false
}

// MaxDay 返回配置中的最大天数，即首充礼包一共可以领几天。
// 配置缺失时返回 0，调用方需按"不可领取"处理。
func (t *TableTplFirstCharge) MaxDay() int32 {
	var maxDay int32
	for _, item := range t.tableData {
		if item.Day > maxDay {
			maxDay = item.Day
		}
	}
	return maxDay
}
