package server

const ZeroLevelId = "L1000"
const BattleCostStamina = 5
const (
	UnlockType_System     int32 = 1 // 功能系统解锁
	UnlockType_Equipment  int32 = 2 // 装备解锁
	UnlockType_TalentSlot int32 = 3 // 天赋石槽位解锁
)

const (
	PayType_Free int32 = 0 // 免费
	PayType_Coin int32 = 1 // 金币
	PayType_Gem  int32 = 2 // 宝石
	PayType_Ad   int32 = 3 // 广告
)
