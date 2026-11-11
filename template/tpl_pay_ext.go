package template

// 本文件是 tpl_pay.go（由 nakama-tool/json2go 自动生成）的手写扩展。
// 生成器只会覆盖 tpl_pay.go，不会删除或改写本文件，因此表专属的查询方法放这里。
// 禁止把方法写进 tpl_pay.go —— 重新导表后会被整文件覆盖。

// FindByGoogleID 按 Google Play 商品ID（TplPay.googleId）反查支付配置。
// 支付回调里客户端给的是 Google 档位 ID，需要映射回 TplPay.id 才能区分商品。
func (t *TableTplPay) FindByGoogleID(googleID string) (TplPay, bool) {
	if googleID == "" {
		return TplPay{}, false
	}
	if t.tableData == nil {
		return TplPay{}, false
	}
	for _, item := range t.tableData {
		if item.GoogleID == googleID {
			return item, true
		}
	}
	return TplPay{}, false
}
