package server

const ZeroLevelId = "L1000"
const BattleCostStamina = 5
const TurretDebrisCount = 10

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
	ItemType_Stamina                   = 0  // 体力
	ItemType_AdTicket                  = 1  // 免广告券
	ItemType_Coin                      = 2  // 金币
	ItemType_Gem                       = 3  // 钻石
	ItemType_EquipmentFragment         = 4  // 装备碎片
	ItemType_Ticket                    = 5  // 抽奖券
	ItemType_ElementCrystal            = 6  // 元素晶体
	ItemType_TechPoint                 = 7  // 科技点
	ItemType_Turret                    = 8  // 炮台
	ItemType_CrystalEquipmentBlueprint = 9  // 水晶装备图纸
	ItemType_RefiningStone             = 10 // 洗练石
	ItemType_RandomCrystalEquipment    = 11 // 随机水晶装备
	ItemType_CrystalSkinFragment       = 12 // 水晶皮肤碎片
	ItemType_RandomElementCrystal      = 13 // 随机元素晶体
	ItemType_RandomEquipmentBlueprint  = 14 // 随机装备图纸
	ItemType_RandomTurretFragment      = 15 // 随机炮台碎片
	ItemType_RandomQualityTurret       = 16 // 随机品质炮台
	ItemType_AvatarFrame               = 17 // 头像框
	ItemType_CommanderExp              = 18 // 指挥官经验
	ItemType_CrystalCore               = 19 // 晶核
)

// 水晶ID常量
const (
	CrystalID_1 = "30001"
	CrystalID_2 = "30002"
	CrystalID_3 = "30003"
	CrystalID_4 = "30004"
)

// 直购ID
const (
	VipProductID         = "foreverAdvCard"       // VIP商品ID
	FirstChargeProductID = "firstCharge"          // 首冲商品ID
	SevenDayProductID    = "sevenDay"             // 七日购买商品ID
	VipRewardID          = "ForeverAdvCardReward" // VIP额外奖励ID
)

const (
	// 邀请奖励
	InviteRewardID = "InviteReward"
	// 加桌奖励
	TTShortcutRewardID = "TTShortcutReword" // 添加到桌面奖励（仅一次）
	// 入口有奖
	TTEntryRewardID = "TTEntryReword" // 入口有奖（每天一次）

	TTDirectPlayRewardID = "LimitedTimePackageReword" // 直玩显示礼包
)

// 每日签到相关常量
const (
	DailySignInTotalDays = 30
	DailySignInBaseID    = 10000
)

// 7天登录奖励相关常量
const (
	SevenDaySignInTotalDays = 7
	SevenDaySignInBaseID    = 10000
)

// 装备相关常量
const (
	EquipID_Crystal    = "EQ1999"
	ItemID_CrystalCore = "1999" // 晶核ID
)

// 玩家等级相关常量
const (
	ExpPerStamina = 6 // 每消耗1个体力获得6个经验值
)

// 休息站配置常量
const (
	RestStationEveryStamina int32 = 15 // 每次领取的体力数量
	RestStationMaxCount     int32 = 60 // 休息站最多存储的体力个数上限
)

// 每日领取体力的时间点（小时）
var RestStationDailyTimes = []int{9, 12, 18, 21}
