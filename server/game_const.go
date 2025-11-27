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

// 道具ID常量
const (
	ItemID_Coin          = "10000" // 金币
	ItemID_Gem           = "10001" // 钻石
	ItemID_Stamina       = "10002" // 体力
	ItemID_AdTicket      = "20000" // 广告券
	ItemID_RandomTurret  = "50000" // 随机炮台（转为碎片）
	ItemID_RandomCrystal = "60000" // 随机水晶
	ItemID_TurretDebris  = "90000" // 炮台碎片
)

const (
	ItemType_EquipmentFragment = 4 // 装备碎片
)

// 水晶ID常量
const (
	CrystalID_1 = "30001"
	CrystalID_2 = "30002"
	CrystalID_3 = "30003"
	CrystalID_4 = "30004"
)

// VIP相关常量
const (
	VipProductID  = "vip_card_68"          // VIP商品ID
	VipPrice      = "68"                   // VIP价格（字符串表示）
	VipPriceFloat = 68.0                   // VIP价格（浮点）
	VipRewardID   = "ForeverAdvCardReward" // VIP额外奖励ID
)

// 首冲相关常量
const (
	FirstChargeProductID  = "first_charge_6" // 首冲商品ID
	FirstChargePrice      = "6"              // 首冲价格（元）
	FirstChargePriceFloat = 6.0              // 首冲价格（浮点数）
)

// 七日购买相关常量
const (
	SevenDayProductID  = "seven_day_68" // 七日购买商品ID
	SevenDayPrice      = "68"           // 七日购买价格（元）
	SevenDayPriceFloat = 68.0           // 七日购买价格（浮点数）
)

// 每日签到相关常量
const (
	DailySignInTotalDays = 30
	DailySignInBaseID    = 10000
)

// 装备相关常量
const (
	EquipID_Crystal = "EQ1999"
)
