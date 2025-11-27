package server

import (
	"time"

	"github.com/heroiclabs/nakama/v3/game"
)

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
	CurrentDay    int32  `json:"current_day"`     // 当前已签到到的天数（1-30）
	LastClaimDate string `json:"last_claim_date"` // 最后一次签到日期（YYYY-MM-DD）
}

func (d *SignInData) GetCollection() string {
	return "sign_in"
}

func (d *SignInData) GetKey() string {
	return "data"
}

func (d *SignInData) Init() {
	d.CurrentDay = 0
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
	Shops map[string]*ShopInfo `json:"shops"` // key: ShopType string
}

func (d *ShopData) GetCollection() string {
	return "shop"
}

func (d *ShopData) GetKey() string {
	return "daily"
}

func (d *ShopData) Init() {
	d.Shops = make(map[string]*ShopInfo)
	d.SetVersion("")
}

// ChapterShopData 章节商店数据
type ChapterShopData struct {
	BaseStorable
	BoughtCounts map[string]int32 `json:"bought_counts"` // key: shopItemID, value: boughtCount
}

func (d *ChapterShopData) GetCollection() string {
	return "shop"
}

func (d *ChapterShopData) GetKey() string {
	return "chapter"
}

func (d *ChapterShopData) Init() {
	d.BoughtCounts = make(map[string]int32)
	d.SetVersion("")
}

// GemShopData 钻石商店数据（独立存储，只保存购买次数）
type GemShopData struct {
	BaseStorable
	BoughtCounts map[string]int32 `json:"bought_counts"` // key: shopItemID, value: boughtCount
}

func (d *GemShopData) GetCollection() string {
	return "shop"
}

func (d *GemShopData) GetKey() string {
	return "gem"
}

func (d *GemShopData) Init() {
	d.BoughtCounts = make(map[string]int32)
	d.SetVersion("")
}

// BoxShopData 宝箱商店数据
type BoxShopData struct {
	BaseStorable
	BoxItemID string `json:"box_item_id"` // 当前宝箱商品ID
	BoxLevel  int32  `json:"box_level"`   // 宝箱等级
	BoxExp    int32  `json:"box_exp"`     // 宝箱经验
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
	d.SetVersion("")
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
	DateTime               time.Time `json:"date_time"`                // 最后更新时间
	ClaimedTasks           []string  `json:"claimed_tasks"`            // 已领取的任务ID列表
	ClaimedLivenessRewards []string  `json:"claimed_liveness_rewards"` // 已领取的活跃度奖励ID列表
	CurrentLiveness        int32     `json:"current_liveness"`         // 当前活跃度
}

func (d *TaskData) GetCollection() string {
	return "task"
}

func (d *TaskData) GetKey() string {
	return "data"
}

func (d *TaskData) Init() {
	d.DateTime = time.Now().UTC()
	d.ClaimedTasks = []string{}
	d.ClaimedLivenessRewards = []string{}
	d.CurrentLiveness = 0
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

// ByteDirectPlay 字节跳动直玩数据
type ByteDirectPlay struct {
	BaseStorable
	SceneTimestamps map[int64]string `json:"scene_timestamps"` // 记录每个场景的最后返回时间
}

func (f *ByteDirectPlay) GetCollection() string {
	return "ByteGame"
}

func (f *ByteDirectPlay) GetKey() string {
	return "DirectPlay"
}

func (f *ByteDirectPlay) Init() {
	if f.SceneTimestamps == nil {
		f.SceneTimestamps = make(map[int64]string)
	}
	f.SetVersion("")
}

// ============================================================================
// 家园相关数据结构
// ============================================================================

// HomeData 家园数据
type HomeData struct {
	BaseStorable
	CurLevelId             string `json:"curLevelId"`
	LastGetOnHookTimestamp string `json:"lastGetOnHookTimestamp"`
}

func (f *HomeData) GetCollection() string {
	return "Home"
}

func (f *HomeData) GetKey() string {
	return "HomeData"
}

func (f *HomeData) Init() {
	f.CurLevelId = ""
	f.LastGetOnHookTimestamp = ""
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
// 关卡相关数据结构
// ============================================================================

// LevelData 关卡数据存储结构
type LevelData struct {
	BaseStorable
	CurLevelId             string `json:"cur_level_id"`               // 当前关卡ID
	BestProgress           int32  `json:"best_progress"`              // 最佳进度
	HasMoppingTimes        int32  `json:"has_mopping_times"`          // 第一阶段已使用次数（扣体力）
	HasMoppingTimesForAdv  int32  `json:"has_mopping_times_for_adv"`  // 第二阶段已使用次数（扣体力+广告）
	LastMoppingTimestamp   string `json:"last_mopping_timestamp"`     // 最后一次扫荡时间，ISO 8601 格式
	LastGetOnHookTimestamp string `json:"last_get_on_hook_timestamp"` // 最后一次领取挂机奖励时间，ISO 8601 格式
}

func (d *LevelData) GetCollection() string {
	return "level"
}

func (d *LevelData) GetKey() string {
	return "data"
}

func (d *LevelData) Init() {
	now := time.Now().Format(time.RFC3339)
	d.CurLevelId = ZeroLevelId // "L1000"
	d.BestProgress = 0
	d.HasMoppingTimes = 0
	d.HasMoppingTimesForAdv = 0
	d.LastMoppingTimestamp = now
	d.LastGetOnHookTimestamp = now
	d.SetVersion("")
}

// ============================================================================
// 体力相关数据结构
// ============================================================================

// StaminaData 体力数据存储结构
type StaminaData struct {
	BaseStorable
	Stamina         int32  `json:"stamina"`           // 当前体力值
	LastRefreshTime string `json:"last_refresh_time"` // 最后刷新时间，ISO 8601 格式
}

func (d *StaminaData) GetCollection() string {
	return "stamina"
}

func (d *StaminaData) GetKey() string {
	return "data"
}

func (d *StaminaData) Init() {
	d.Stamina = MaxStamina
	d.LastRefreshTime = time.Now().Format(time.RFC3339)
	d.SetVersion("")
}
