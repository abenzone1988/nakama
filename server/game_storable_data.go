package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"context"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
)

// BaseStorable 提供版本管理的基础实现，其他 Storable 结构体可以嵌入此结构体
type BaseStorable struct {
	version string // 版本号，用于乐观并发控制（不序列化到 JSON）
}

func (b *BaseStorable) GetVersion() string {
	return b.version
}

func (b *BaseStorable) SetVersion(version string) {
	b.version = version
}

type Storable interface {
	GetCollection() string
	GetKey() string
	Init()
	// GetVersion 返回当前数据的版本号，用于乐观并发控制（OCC）
	// 如果返回空字符串，表示使用 "last write wins" 模式
	GetVersion() string
	// SetVersion 设置数据的版本号，在 LoadData 时自动调用
	SetVersion(version string)
}

func LoadUserData(ctx context.Context, logger *zap.Logger, db *sql.DB, storable Storable) error {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	readOp := &api.ReadStorageObjectId{
		Collection: storable.GetCollection(),
		Key:        storable.GetKey(),
		UserId:     userID.String(),
	}

	objectIDs := []*api.ReadStorageObjectId{readOp}

	storageObjects, err := StorageReadObjects(ctx, logger, db, userID, objectIDs)
	if err != nil {
		logger.Error("无法从存储系统读取数据", zap.Error(err))
		return err
	}

	if len(storageObjects.Objects) == 0 {
		storable.Init()
		storable.SetVersion("") // 新数据，version 为空
		return nil
	}

	// 保存 version 用于后续的 OCC 写入
	storageObject := storageObjects.Objects[0]
	storable.SetVersion(storageObject.Version)

	if err := json.Unmarshal([]byte(storageObject.Value), storable); err != nil {
		logger.Error("无法反序列化数据", zap.Error(err))
		return err
	}

	return nil
}

func LoadData(ctx context.Context, logger *zap.Logger, db *sql.DB, userID uuid.UUID, storable Storable) error {
	readOp := &api.ReadStorageObjectId{
		Collection: storable.GetCollection(),
		Key:        storable.GetKey(),
		UserId:     userID.String(),
	}

	objectIDs := []*api.ReadStorageObjectId{readOp}

	storageObjects, err := StorageReadObjects(ctx, logger, db, userID, objectIDs)
	if err != nil {
		logger.Error("无法从存储系统读取数据", zap.Error(err))
		return err
	}

	if len(storageObjects.Objects) == 0 {
		storable.Init()
		storable.SetVersion("") // 新数据，version 为空
		return nil
	}

	// 保存 version 用于后续的 OCC 写入
	storageObject := storageObjects.Objects[0]
	storable.SetVersion(storageObject.Version)

	if err := json.Unmarshal([]byte(storageObject.Value), storable); err != nil {
		logger.Error("无法反序列化数据", zap.Error(err))
		return err
	}

	return nil
}

func SaveUserData(ctx context.Context, logger *zap.Logger, db *sql.DB, metrics Metrics, storageIndex StorageIndex, storable Storable) error {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)
	serializedData, err := json.Marshal(storable)
	if err != nil {
		logger.Error("无法序列化数据", zap.Error(err))
		return err
	}

	// 使用保存的 version 进行 OCC 写入，防止并发覆盖
	version := storable.GetVersion()
	writeOp := CreateStorageOpWriteWithVersion(storable.GetCollection(), storable.GetKey(), string(serializedData), userID.String(), version)

	ops := []*StorageOpWrite{writeOp}

	acks, code, err := StorageWriteObjects(ctx, logger, db, metrics, storageIndex, true, ops)
	if err != nil {
		logger.Error("无法保存数据到存储系统", zap.Error(err))
		return err
	}

	// 如果版本冲突，返回错误
	if code != codes.OK {
		return fmt.Errorf("存储写入被拒绝，可能由于版本冲突或权限问题")
	}

	// 更新 version 为新的版本号
	if len(acks.Acks) > 0 {
		storable.SetVersion(acks.Acks[0].Version)
	}

	return nil
}

func SaveData(ctx context.Context, logger *zap.Logger, db *sql.DB, metrics Metrics, storageIndex StorageIndex, userID uuid.UUID, storable Storable) error {
	serializedData, err := json.Marshal(storable)
	if err != nil {
		logger.Error("无法序列化数据", zap.Error(err))
		return err
	}

	// 使用保存的 version 进行 OCC 写入，防止并发覆盖
	version := storable.GetVersion()
	writeOp := CreateStorageOpWriteWithVersion(storable.GetCollection(), storable.GetKey(), string(serializedData), userID.String(), version)

	ops := []*StorageOpWrite{writeOp}

	acks, code, err := StorageWriteObjects(ctx, logger, db, metrics, storageIndex, true, ops)
	if err != nil {
		logger.Error("无法保存数据到存储系统", zap.Error(err))
		return err
	}

	// 如果版本冲突，返回错误
	if code != codes.OK {
		return fmt.Errorf("存储写入被拒绝，可能由于版本冲突或权限问题")
	}

	// 更新 version 为新的版本号
	if len(acks.Acks) > 0 {
		storable.SetVersion(acks.Acks[0].Version)
	}

	return nil
}

// ============================================================================
// 购买相关数据结构
// ============================================================================

// FirstChargeData 首冲数据存储结构
type FirstChargeData struct {
	BaseStorable
	IsCharged   bool    `json:"is_charged"`   // 是否已首冲
	ChargeTime  string  `json:"charge_time"`  // 首冲时间，ISO 8601 格式
	ClaimedDays []int32 `json:"claimed_days"` // 已领取的天数列表
}

func (d *FirstChargeData) GetCollection() string {
	return "first_charge"
}

func (d *FirstChargeData) GetKey() string {
	return "data"
}

func (d *FirstChargeData) Init() {
	d.IsCharged = false
	d.ChargeTime = ""
	d.ClaimedDays = []int32{}
	d.SetVersion("")
}

// SevenDayData 七日购买数据存储结构
type SevenDayData struct {
	BaseStorable
	LastPurchaseTime string  `json:"last_purchase_time"` // 最后一次购买时间，ISO 8601 格式
	ClaimedDays      []int32 `json:"claimed_days"`       // 当前周期已领取的天数列表
	TotalPurchases   int32   `json:"total_purchases"`    // 总购买次数
}

func (d *SevenDayData) GetCollection() string {
	return "seven_day"
}

func (d *SevenDayData) GetKey() string {
	return "data"
}

func (d *SevenDayData) Init() {
	d.LastPurchaseTime = ""
	d.ClaimedDays = []int32{}
	d.TotalPurchases = 0
	d.SetVersion("")
}

// ============================================================================
// 签到相关数据结构
// ============================================================================

// SignInData 每日签到数据存储结构
type SignInData struct {
	BaseStorable
	ClaimedDay    int32  `json:"current_day"`     // 当前已签到到的天数（1-30）
	LastClaimDate string `json:"last_claim_date"` // 最后一次签到日期（YYYY-MM-DD）
}

func (d *SignInData) GetCollection() string {
	return "sign_in"
}

func (d *SignInData) GetKey() string {
	return "data"
}

func (d *SignInData) Init() {
	d.ClaimedDay = 0
	d.LastClaimDate = ""
	d.SetVersion("")
}

// ============================================================================
// 商店相关数据结构
// ============================================================================

// ShopData 商店数据存储结构（包含所有类型商店）
type ShopInfo struct {
	ShopType        game.ShopType `json:"shop_type"`
	Items           []*ShopItem   `json:"items"`
	CanRefresh      bool          `json:"can_refresh"`
	RefreshCount    int32         `json:"refresh_count"`
	NextRefreshTime time.Time     `json:"next_refresh_time"`
}

type ShopItem struct {
	ShopItemID  string       `json:"shop_item_id"`
	ItemID      string       `json:"item_id"`
	PayType     game.PayType `json:"pay_type"`
	Price       int32        `json:"price"`
	Discount    int32        `json:"discount"`
	ItemCount   int32        `json:"item_count"`
	MaxBuyCount int32        `json:"max_buy_count"`
	BoughtCount int32        `json:"bought_count"`
	// 分段购买配置（用于支持多种支付方式，如先免费再广告）
	PaymentStages []PaymentStage `json:"payment_stages,omitempty"`
}

// PaymentStage 支付阶段配置
type PaymentStage struct {
	PayType    game.PayType `json:"pay_type"`    // 支付类型
	Price      int32        `json:"price"`       // 价格
	LimitCount int32        `json:"limit_count"` // 该阶段购买次数限制
}

type ShopData struct {
	BaseStorable
	Shops map[game.ShopType]*ShopInfo `json:"shops"` // key: ShopType string
}

func (d *ShopData) GetCollection() string {
	return "shop"
}

func (d *ShopData) GetKey() string {
	return "daily"
}

func (d *ShopData) Init() {
	d.Shops = make(map[game.ShopType]*ShopInfo)
	d.SetVersion("")
}

// ChapterShopData 章节商店数据
type ChapterShopData struct {
	BaseStorable
	BoughtCounts  map[string]int32 `json:"bought_counts"`  // key: shopItemID, value: boughtCount
	ClaimedCounts map[string]int32 `json:"claimed_counts"` // key: shopItemID, value: claimedCount
}

func (d *ChapterShopData) GetCollection() string {
	return "shop"
}

func (d *ChapterShopData) GetKey() string {
	return "chapter"
}

func (d *ChapterShopData) Init() {
	d.BoughtCounts = make(map[string]int32)
	d.ClaimedCounts = make(map[string]int32)
	d.SetVersion("")
}

// GemShopData 钻石商店数据（独立存储，只保存购买次数）
type GemShopData struct {
	BaseStorable
	BoughtCounts  map[string]int32 `json:"bought_counts"`  // key: shopItemID, value: boughtCount
	ClaimedCounts map[string]int32 `json:"claimed_counts"` // key: shopItemID, value: claimedCount
}

func (d *GemShopData) GetCollection() string {
	return "shop"
}

func (d *GemShopData) GetKey() string {
	return "gem"
}

func (d *GemShopData) Init() {
	d.BoughtCounts = make(map[string]int32)
	d.ClaimedCounts = make(map[string]int32)
	d.SetVersion("")
}

// BoxShopData 宝箱商店数据
type BoxShopData struct {
	BaseStorable
	BoxItemID           string    `json:"box_item_id"`            // 当前宝箱商品ID
	BoxLevel            int32     `json:"box_level"`              // 宝箱等级
	BoxExp              int32     `json:"box_exp"`                // 宝箱经验
	FreeAdUsed          bool      `json:"free_ad_used"`           // 当日广告免费购买是否已用
	NextFreeRefreshTime time.Time `json:"next_free_refresh_time"` // 下次广告免费刷新时间
}

func (d *BoxShopData) GetCollection() string {
	return "shop"
}

func (d *BoxShopData) GetKey() string {
	return "box"
}

func (d *BoxShopData) Init() {
	d.BoxItemID = ""
	d.BoxLevel = 1
	d.BoxExp = 0
	d.FreeAdUsed = false
	d.NextFreeRefreshTime = time.Time{}
	d.SetVersion("")
}

func (d *BoxShopData) RefreshFreeAdState() {
	now := time.Now().UTC()
	if d == nil {
		return
	}

	if d.NextFreeRefreshTime.IsZero() {
		d.NextFreeRefreshTime = nextDayZero(now)
		d.FreeAdUsed = false
		return
	}

	if now.After(d.NextFreeRefreshTime) || now.Equal(d.NextFreeRefreshTime) {
		d.FreeAdUsed = false
		d.NextFreeRefreshTime = nextDayZero(now)
	}
}

func nextDayZero(t time.Time) time.Time {
	loc := t.Location()
	return time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, loc)
}

// ============================================================================
// VIP相关数据结构
// ============================================================================

// VipRewardData VIP奖励数据
type VipRewardData struct {
	BaseStorable
	RewardClaimed bool `json:"reward_claimed"`
}

func (v *VipRewardData) GetCollection() string {
	return "vip"
}

func (v *VipRewardData) GetKey() string {
	return "reward"
}

func (v *VipRewardData) Init() {
	v.RewardClaimed = false
	v.SetVersion("")
}

// ============================================================================
// 任务相关数据结构
// ============================================================================

// TaskData 任务数据
type TaskData struct {
	BaseStorable
	DateTime                     time.Time `json:"date_time"`                       // 最后更新时间（每日重置）
	WeeklyResetTime              time.Time `json:"weekly_reset_time"`               // 每周重置时间
	ClaimedDailyTasks            []string  `json:"claimed_daily_tasks"`             // 已领取的每日任务ID列表（taskType 2）
	ClaimedWeeklyTasks           []string  `json:"claimed_weekly_tasks"`            // 已领取的每周任务ID列表（taskType 3）
	ClaimedMainTasks             []string  `json:"claimed_main_tasks"`              // 已领取的主线任务ID列表（taskType 1，永久）
	ClaimedDailyLivenessRewards  []string  `json:"claimed_daily_liveness_rewards"`  // 已领取的每日活跃度奖励ID列表
	ClaimedWeeklyLivenessRewards []string  `json:"claimed_weekly_liveness_rewards"` // 已领取的每周活跃度奖励ID列表
	DailyLiveness                int32     `json:"daily_liveness"`                  // 每日活跃度
	WeeklyLiveness               int32     `json:"weekly_liveness"`                 // 每周活跃度
}

func (d *TaskData) GetCollection() string {
	return "task"
}

func (d *TaskData) GetKey() string {
	return "data"
}

func (d *TaskData) Init() {
	d.DateTime = time.Now().UTC()
	// 计算本周开始时间（周一 00:00 UTC）
	now := time.Now().UTC()
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	daysFromMonday := weekday - 1
	d.WeeklyResetTime = now.Truncate(24*time.Hour).AddDate(0, 0, -daysFromMonday)
	d.ClaimedDailyTasks = []string{}
	d.ClaimedWeeklyTasks = []string{}
	d.ClaimedMainTasks = []string{}
	d.ClaimedDailyLivenessRewards = []string{}
	d.ClaimedWeeklyLivenessRewards = []string{}
	d.DailyLiveness = 0
	d.WeeklyLiveness = 0
	d.SetVersion("")
}

// ============================================================================
// 关卡相关数据结构
// ============================================================================

// LevelBoxData 关卡宝箱数据
type LevelBoxData struct {
	BaseStorable
	ClaimedBoxes map[string][]int32 `json:"claimed_boxes"` // key: level_id, value: 已领取的宝箱ID列表
}

func (d *LevelBoxData) GetCollection() string {
	return "level_box"
}

func (d *LevelBoxData) GetKey() string {
	return "data"
}

func (d *LevelBoxData) Init() {
	d.ClaimedBoxes = make(map[string][]int32)
	d.SetVersion("")
}

//客户端的HomeLevel

type HomeLevelData struct {
	BaseStorable
	CurLevelId string `json:"curLevelId"`
}

func (d *HomeLevelData) Init() {
}

func (d *HomeLevelData) GetCollection() string {
	return "Home"
}

func (d *HomeLevelData) GetKey() string {
	return "HomeData"
}

// ============================================================================
// 邀请相关数据结构
// ============================================================================

// InviteRecord 邀请记录
type InviteRecord struct {
	Invitee       string `json:"invitee"`
	RewardClaimed bool   `json:"reward_claimed"`
}

// InviteData 邀请数据
type InviteData struct {
	BaseStorable
	List      map[string]*InviteRecord `json:"invite_list"`
	BeInvited bool                     `json:"be_invited"`
	Inviter   string                   `json:"inviter"`
}

func (f *InviteData) GetCollection() string {
	return "user_data"
}

func (f *InviteData) GetKey() string {
	return "invite"
}

func (f *InviteData) Init() {
	f.List = make(map[string]*InviteRecord)
	f.BeInvited = false
	f.Inviter = ""
	f.SetVersion("")
}

// ============================================================================
// 反馈相关数据结构
// ============================================================================

// FeedbackRecord 反馈记录
type FeedbackRecord struct {
	Description string    `json:"content"`
	Issues      int32     `json:"issues"`
	ReportedAt  time.Time `json:"time"`
}

// FeedbackHistory 反馈历史
type FeedbackHistory struct {
	BaseStorable
	List []*FeedbackRecord `json:"list"`
}

func (f *FeedbackHistory) GetCollection() string {
	return "manager_data"
}

func (f *FeedbackHistory) GetKey() string {
	return "feedback"
}

func (f *FeedbackHistory) Init() {
	f.List = []*FeedbackRecord{}
	f.SetVersion("")
}

// ============================================================================
// 兑换码相关数据结构
// ============================================================================

// RedeemRecord 兑换记录
type RedeemRecord struct {
	Code       string    `json:"code"`
	RedeemTime time.Time `json:"time"`
}

// RedeemHistory 兑换历史
type RedeemHistory struct {
	BaseStorable
	Records map[string]*RedeemRecord `json:"records"`
}

func (f *RedeemHistory) GetCollection() string {
	return "user_data"
}

func (f *RedeemHistory) GetKey() string {
	return "redeem"
}

func (f *RedeemHistory) Init() {
	f.Records = make(map[string]*RedeemRecord)
	f.SetVersion("")
}

// ============================================================================
// 抖音小游戏相关数据结构
// ============================================================================

// ByteRewardData 抖音奖励数据
type ByteRewardData struct {
	BaseStorable
	ShortcutRewardClaimed bool              `json:"shortcut_reward_claimed"` // 添加到桌面奖励是否已领取（仅一次）
	EntryRewardLastDate   string            `json:"entry_reward_last_date"`  // 入口有奖最后领取日期（YYYY-MM-DD）
	SceneTimestamps       map[string]string `json:"scene_timestamps"`        // 记录每个场景的最后返回时间
	GetDirectPlayReward   bool              `json:"get_direct_play_reward"`  // 记录是否领取直玩显示礼包
}

func (f *ByteRewardData) GetCollection() string {
	return "ByteGame"
}

func (f *ByteRewardData) GetKey() string {
	return "Reward"
}

func (f *ByteRewardData) Init() {
	f.ShortcutRewardClaimed = false
	f.EntryRewardLastDate = ""
	f.SceneTimestamps = make(map[string]string)
	f.GetDirectPlayReward = false
	f.SetVersion("")
}

// ============================================================================
// 装备相关数据结构
// ============================================================================

// EquipData 装备数据存储结构
type EquipData struct {
	BaseStorable
	BattleEquips         []string         `json:"battle_equips"`           // 战斗装备列表
	UnlockEquips         map[string]int32 `json:"unlock_equips"`           // 已解锁装备及其等级
	CrystalTechTreeIndex int32            `json:"crystal_tech_tree_index"` // 水晶科技树索引
}

func (d *EquipData) GetCollection() string {
	return "equip"
}

func (d *EquipData) GetKey() string {
	return "data"
}

func (d *EquipData) Init() {
	d.BattleEquips = []string{}
	d.UnlockEquips = make(map[string]int32)
	d.CrystalTechTreeIndex = 0
	d.SetVersion("")
}

// ============================================================================
// 战斗奖励相关数据结构
// ============================================================================

// BattleData 战斗数据存储结构
type BattleData struct {
	BaseStorable
	CurLevelId             string          `json:"cur_level_id"`               // 当前正在战斗的关卡ID
	MaxLevelId             string          `json:"max_level_id"`               // 最大关卡ID
	MaxActivityLevelId     string          `json:"max_activity_level_id"`      // 最大活动关卡ID
	MaxEliteLevelId        string          `json:"max_elite_level_id"`         // 最大精英关卡ID
	BattleType             game.BattleType `json:"battle_type"`                // 最后一次战斗类型（0=普通，1=黄金）
	BattleEnded            bool            `json:"battle_ended"`               // 战斗是否已结束（防止重复领取奖励）
	ShareRewardClaimed     bool            `json:"share_reward_claimed"`       // 是否已领取分享奖励（每次战斗可重新领取一次）
	RewardJSON             string          `json:"reward_json"`                // 保存的奖励 JSON 字符串
	HasMoppingTimes        int32           `json:"has_mopping_times"`          // 第一阶段已使用次数（扣体力）
	HasMoppingTimesForAdv  int32           `json:"has_mopping_times_for_adv"`  // 第二阶段已使用次数（扣体力+广告）
	LastMoppingTimestamp   string          `json:"last_mopping_timestamp"`     // 最后一次扫荡时间，ISO 8601 格式
	LastGetOnHookTimestamp string          `json:"last_get_on_hook_timestamp"` // 最后一次领取挂机奖励时间，ISO 8601 格式
}

func (d *BattleData) resetMoppingTimes() {
	d.HasMoppingTimes = 0
	d.HasMoppingTimesForAdv = 0
}

func (d *BattleData) GetCollection() string {
	return "battle"
}

func (d *BattleData) GetKey() string {
	return "data"
}

func (d *BattleData) Init() {
	d.CurLevelId = ""
	d.MaxLevelId = ""
	d.MaxActivityLevelId = ""
	d.MaxEliteLevelId = ""
	d.BattleType = 0
	d.BattleEnded = true
	d.ShareRewardClaimed = false
	d.RewardJSON = ""
	d.HasMoppingTimes = 0
	d.HasMoppingTimesForAdv = 0
	d.SetVersion("")
}

// ============================================================================
// 玩家等级相关数据结构
// ============================================================================

// PlayerLevelData 玩家等级数据存储结构
type PlayerLevelData struct {
	BaseStorable
	Level    int32 `json:"level"`     // 当前等级
	TotalExp int32 `json:"total_exp"` // 总获得的经验值
}

func (d *PlayerLevelData) GetCollection() string {
	return "player_level"
}

func (d *PlayerLevelData) GetKey() string {
	return "data"
}

func (d *PlayerLevelData) Init() {
	d.Level = 1
	d.TotalExp = 0
	d.SetVersion("")
}

// ============================================================================
// 挖矿相关数据结构
// ============================================================================

// MineData 挖矿数据存储结构
type MineData struct {
	BaseStorable
	DailyMineCount    int32  `json:"daily_mine_count"`    // 每日剩余挖矿次数
	LastRefreshDate   string `json:"last_refresh_date"`   // 最后刷新日期（YYYY-MM-DD）
	CurrentLevel      int32  `json:"current_level"`       // 当前矿产等级
	CurrentMinedCount int32  `json:"current_mined_count"` // 当前矿产已挖出的数量
	IsCompleted       bool   `json:"is_completed"`        // 当前矿产是否已挖完
	BuyCount          int32  `json:"buy_count"`           // 今日已购买次数
}

func (d *MineData) GetCollection() string {
	return "mine"
}

func (d *MineData) GetKey() string {
	return "data"
}

func (d *MineData) Init() {
	d.DailyMineCount = 30 // DailyMineCount 常量已移到 game_mine.go
	d.LastRefreshDate = ""
	d.CurrentLevel = 1
	d.CurrentMinedCount = 0
	d.IsCompleted = false
	d.BuyCount = 0
	d.SetVersion("")
}
