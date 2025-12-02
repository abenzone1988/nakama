package server

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama/v3/game"
	. "github.com/heroiclabs/nakama/v3/template"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// mockTemplateManager 用于测试的模板管理器
type mockTemplateManager struct{}

func (m *mockTemplateManager) LoadData()                                                 {}
func (m *mockTemplateManager) GetTplRedemption() *TableTplRedemption                     { return nil }
func (m *mockTemplateManager) GetTplLevelInfo() *TableTplLevelInfo                       { return nil }
func (m *mockTemplateManager) GetTplActivityLevelInfo() *TableTplActivityLevelInfo       { return nil }
func (m *mockTemplateManager) GetTplCrystalTechnologyDev() *TableTplCrystalTechnologyDev { return nil }
func (m *mockTemplateManager) GetTplRedemptionInfo() *TableTplRedemption                 { return nil }
func (m *mockTemplateManager) GetTplItem() *TableTplItem                                 { return nil }
func (m *mockTemplateManager) GetTplReward() *TableTplReward                             { return nil }
func (m *mockTemplateManager) GetTplEquipmentDev() *TableTplEquipmentDev                 { return nil }
func (m *mockTemplateManager) GetTplUnlock() *TableTplUnlock                             { return nil }
func (m *mockTemplateManager) GetTplShop() *TableTplShop                                 { return nil }
func (m *mockTemplateManager) GetTplShopDailyItem() *TableTplShopDailyItem               { return nil }
func (m *mockTemplateManager) GetTplShopPermanentItem() *TableTplShopPermanentItem       { return nil }
func (m *mockTemplateManager) GetTplBoxShop() *TableTplBoxShop                           { return nil }
func (m *mockTemplateManager) GetTplBoxShopItem() *TableTplBoxShopItem                   { return nil }
func (m *mockTemplateManager) GetTplShopChapterItem() *TableTplShopChapterItem           { return nil }
func (m *mockTemplateManager) GetTplShopGemItem() *TableTplShopGemItem                   { return nil }
func (m *mockTemplateManager) GetTplTasks() *TableTplTasks                               { return nil }
func (m *mockTemplateManager) GetTplProgressReward() *TableTplProgressReward             { return nil }
func (m *mockTemplateManager) GetTplFirstCharge() *TableTplFirstCharge                   { return nil }
func (m *mockTemplateManager) GetTplSevenDay() *TableTplSevenDay                         { return nil }
func (m *mockTemplateManager) GetTplDailySignIn() *TableTplDailySignIn                   { return nil }

// mockMetrics 用于测试的指标管理器
type mockMetrics struct{}

func (m *mockMetrics) Stop(logger *zap.Logger)    {}
func (m *mockMetrics) SnapshotLatencyMs() float64 { return 0 }
func (m *mockMetrics) SnapshotRateSec() float64   { return 0 }
func (m *mockMetrics) SnapshotRecvKbSec() float64 { return 0 }
func (m *mockMetrics) SnapshotSentKbSec() float64 { return 0 }
func (m *mockMetrics) Api(name string, elapsed time.Duration, recvBytes, sentBytes int64, isErr bool) {
}
func (m *mockMetrics) ApiRpc(id string, elapsed time.Duration, recvBytes, sentBytes int64, isErr bool) {
}
func (m *mockMetrics) ApiBefore(name string, elapsed time.Duration, isErr bool)             {}
func (m *mockMetrics) ApiAfter(name string, elapsed time.Duration, isErr bool)              {}
func (m *mockMetrics) Message(recvBytes int64, isErr bool)                                  {}
func (m *mockMetrics) MessageBytesSent(sentBytes int64)                                     {}
func (m *mockMetrics) GaugeRuntimes(value float64)                                          {}
func (m *mockMetrics) GaugeLuaRuntimes(value float64)                                       {}
func (m *mockMetrics) GaugeJsRuntimes(value float64)                                        {}
func (m *mockMetrics) GaugeAuthoritativeMatches(value float64)                              {}
func (m *mockMetrics) CountDroppedEvents(delta int64)                                       {}
func (m *mockMetrics) CountWebsocketOpened(delta int64)                                     {}
func (m *mockMetrics) CountWebsocketClosed(delta int64)                                     {}
func (m *mockMetrics) CountUntaggedGrpcStatsCalls(delta int64)                              {}
func (m *mockMetrics) GaugeSessions(value float64)                                          {}
func (m *mockMetrics) GaugePresences(value float64)                                         {}
func (m *mockMetrics) GaugeStorageIndexEntries(indexName string, value float64)             {}
func (m *mockMetrics) Matchmaker(tickets, activeTickets float64, processTime time.Duration) {}
func (m *mockMetrics) PresenceEvent(dequeueElapsed, processElapsed time.Duration)           {}
func (m *mockMetrics) StorageWriteRejectCount(tags map[string]string, delta int64)          {}
func (m *mockMetrics) CustomCounter(name string, tags map[string]string, delta int64)       {}
func (m *mockMetrics) CustomGauge(name string, tags map[string]string, value float64)       {}
func (m *mockMetrics) CustomTimer(name string, tags map[string]string, value time.Duration) {}
func (m *mockMetrics) Count(name string, tags map[string]string, delta float64)             {}
func (m *mockMetrics) Gauge(name string, tags map[string]string, value float64)             {}
func (m *mockMetrics) Timing(name string, tags map[string]string, delta float64)            {}

// mockStorageIndex 用于测试的存储索引管理器
type mockStorageIndex struct{}

func (m *mockStorageIndex) Write(ctx context.Context, objects []*api.StorageObject) (creates int, deletes int) {
	return 0, 0
}
func (m *mockStorageIndex) Delete(ctx context.Context, objects StorageOpDeletes) (deletes int) {
	return 0
}
func (m *mockStorageIndex) List(ctx context.Context, callerID uuid.UUID, indexName, query string, limit int, order []string, cursor string) (*api.StorageObjects, string, error) {
	return nil, "", nil
}
func (m *mockStorageIndex) Load(ctx context.Context) error {
	return nil
}
func (m *mockStorageIndex) CreateIndex(ctx context.Context, name, collection, key string, fields []string, sortFields []string, maxEntries int, indexOnly bool) error {
	return nil
}
func (m *mockStorageIndex) RegisterFilters(runtime *Runtime) {}
func (m *mockStorageIndex) RegisterIndexes(ctx context.Context, logger *zap.Logger, db *sql.DB) error {
	return nil
}

func TestGrantReward_WalletOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过需要数据库的测试")
	}

	db := NewDB(t)
	logger := NewConsoleLogger(os.Stdout, false)
	template := &mockTemplateManager{}
	metrics := &mockMetrics{}
	storageIndex := &mockStorageIndex{}

	// 创建测试用户
	userID, _, _, err := AuthenticateCustom(context.Background(), logger, db, uuid.Must(uuid.NewV4()).String(), uuid.Must(uuid.NewV4()).String(), true)
	assert.NoError(t, err)

	userUUID := uuid.FromStringOrNil(userID)
	ctx := context.WithValue(context.Background(), ctxUserIDKey{}, userUUID)

	// 测试1: 只有金币奖励
	t.Run("只有金币奖励", func(t *testing.T) {
		reward := &game.Reward{
			Wallet: &game.Wallet{
				Coin: 100,
			},
		}

		walletResult, inventoryResult, err := GrantReward(ctx, logger, db, template, metrics, storageIndex, reward, "test_wallet_coin")
		assert.NoError(t, err)
		assert.NotNil(t, walletResult)
		assert.NotNil(t, walletResult.Updated)
		assert.Equal(t, int32(100), walletResult.Updated.Coin)
		assert.Nil(t, inventoryResult)
	})

	// 测试2: 多种钱包奖励
	t.Run("多种钱包奖励", func(t *testing.T) {
		reward := &game.Reward{
			Wallet: &game.Wallet{
				Coin:    200,
				Gem:     50,
				Ad:      10,
				Stamina: 20,
			},
		}

		walletResult, inventoryResult, err := GrantReward(ctx, logger, db, template, metrics, storageIndex, reward, "test_wallet_multi")
		assert.NoError(t, err)
		assert.NotNil(t, walletResult)
		assert.NotNil(t, walletResult.Updated)
		assert.Equal(t, int32(200), walletResult.Updated.Coin)
		assert.Equal(t, int32(50), walletResult.Updated.Gem)
		assert.Equal(t, int32(10), walletResult.Updated.Ad)
		assert.Equal(t, int32(20), walletResult.Updated.Stamina)
		assert.Nil(t, inventoryResult)
	})
}

func TestGrantReward_ItemsOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过需要数据库的测试")
	}

	db := NewDB(t)
	logger := NewConsoleLogger(os.Stdout, false)
	template := &mockTemplateManager{}
	metrics := &mockMetrics{}
	storageIndex := &mockStorageIndex{}

	userID, _, _, err := AuthenticateCustom(context.Background(), logger, db, uuid.Must(uuid.NewV4()).String(), uuid.Must(uuid.NewV4()).String(), true)
	assert.NoError(t, err)

	userUUID := uuid.FromStringOrNil(userID)
	ctx := context.WithValue(context.Background(), ctxUserIDKey{}, userUUID)

	// 测试1: 普通道具奖励
	t.Run("普通道具奖励", func(t *testing.T) {
		reward := &game.Reward{
			Items: []*game.Item{
				{Id: "item_001", Num: 10},
				{Id: "item_002", Num: 5},
			},
		}

		walletResult, inventoryResult, err := GrantReward(ctx, logger, db, template, metrics, storageIndex, reward, "test_items_normal")
		assert.NoError(t, err)
		assert.Nil(t, walletResult)
		assert.NotNil(t, inventoryResult)
		assert.NotNil(t, inventoryResult.Updated)
		assert.Len(t, inventoryResult.Updated, 2)

		itemMap := make(map[string]int32)
		for _, item := range inventoryResult.Updated {
			itemMap[item.Id] = item.Num
		}
		assert.Equal(t, int32(10), itemMap["item_001"])
		assert.Equal(t, int32(5), itemMap["item_002"])
	})

	// 测试2: 特殊道具转换为钱包（金币）
	t.Run("特殊道具转换为钱包-金币", func(t *testing.T) {
		reward := &game.Reward{
			Items: []*game.Item{
				{Id: ItemID_Coin, Num: 100},
			},
		}

		walletResult, inventoryResult, err := GrantReward(ctx, logger, db, template, metrics, storageIndex, reward, "test_item_coin")
		assert.NoError(t, err)
		assert.NotNil(t, walletResult)
		assert.NotNil(t, walletResult.Updated)
		assert.Equal(t, int32(100), walletResult.Updated.Coin)
		assert.Nil(t, inventoryResult)
	})

	// 测试3: 特殊道具转换为钱包（钻石）
	t.Run("特殊道具转换为钱包-钻石", func(t *testing.T) {
		reward := &game.Reward{
			Items: []*game.Item{
				{Id: ItemID_Gem, Num: 50},
			},
		}

		walletResult, inventoryResult, err := GrantReward(ctx, logger, db, template, metrics, storageIndex, reward, "test_item_gem")
		assert.NoError(t, err)
		assert.NotNil(t, walletResult)
		assert.NotNil(t, walletResult.Updated)
		assert.Equal(t, int32(50), walletResult.Updated.Gem)
		assert.Nil(t, inventoryResult)
	})

	// 测试4: 特殊道具转换为钱包（广告券）
	t.Run("特殊道具转换为钱包-广告券", func(t *testing.T) {
		reward := &game.Reward{
			Items: []*game.Item{
				{Id: ItemID_AdTicket, Num: 5},
			},
		}

		walletResult, inventoryResult, err := GrantReward(ctx, logger, db, template, metrics, storageIndex, reward, "test_item_ad")
		assert.NoError(t, err)
		assert.NotNil(t, walletResult)
		assert.NotNil(t, walletResult.Updated)
		assert.Equal(t, int32(5), walletResult.Updated.Ad)
		assert.Nil(t, inventoryResult)
	})

	// 测试5: 特殊道具转换为钱包（体力）
	t.Run("特殊道具转换为钱包-体力", func(t *testing.T) {
		reward := &game.Reward{
			Items: []*game.Item{
				{Id: ItemID_Stamina, Num: 10},
			},
		}

		walletResult, inventoryResult, err := GrantReward(ctx, logger, db, template, metrics, storageIndex, reward, "test_item_stamina")
		assert.NoError(t, err)
		assert.NotNil(t, walletResult)
		assert.NotNil(t, walletResult.Updated)
		assert.Equal(t, int32(10), walletResult.Updated.Stamina)
		assert.Nil(t, inventoryResult)
	})
}

func TestGrantReward_RandomCrystal(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过需要数据库的测试")
	}

	db := NewDB(t)
	logger := NewConsoleLogger(os.Stdout, false)
	template := &mockTemplateManager{}
	metrics := &mockMetrics{}
	storageIndex := &mockStorageIndex{}

	userID, _, _, err := AuthenticateCustom(context.Background(), logger, db, uuid.Must(uuid.NewV4()).String(), uuid.Must(uuid.NewV4()).String(), true)
	assert.NoError(t, err)

	userUUID := uuid.FromStringOrNil(userID)
	ctx := context.WithValue(context.Background(), ctxUserIDKey{}, userUUID)

	// 测试随机水晶转换
	t.Run("随机水晶转换", func(t *testing.T) {
		reward := &game.Reward{
			Items: []*game.Item{
				{Id: ItemID_RandomCrystal, Num: 10},
			},
		}

		walletResult, inventoryResult, err := GrantReward(ctx, logger, db, template, metrics, storageIndex, reward, "test_random_crystal")
		assert.NoError(t, err)
		assert.Nil(t, walletResult)
		assert.NotNil(t, inventoryResult)
		assert.NotNil(t, inventoryResult.Updated)
		assert.Len(t, inventoryResult.Updated, 10) // 10个随机水晶应该转换为10个具体水晶

		// 验证所有水晶都是有效的ID
		crystalIds := map[string]bool{
			CrystalID_1: true,
			CrystalID_2: true,
			CrystalID_3: true,
			CrystalID_4: true,
		}

		for _, item := range inventoryResult.Updated {
			assert.True(t, crystalIds[item.Id], "水晶ID应该是30001-30004之一: %s", item.Id)
			assert.Equal(t, int32(1), item.Num, "每个水晶数量应该是1")
		}
	})
}

func TestGrantReward_MixedReward(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过需要数据库的测试")
	}

	db := NewDB(t)
	logger := NewConsoleLogger(os.Stdout, false)
	template := &mockTemplateManager{}
	metrics := &mockMetrics{}
	storageIndex := &mockStorageIndex{}

	userID, _, _, err := AuthenticateCustom(context.Background(), logger, db, uuid.Must(uuid.NewV4()).String(), uuid.Must(uuid.NewV4()).String(), true)
	assert.NoError(t, err)

	userUUID := uuid.FromStringOrNil(userID)
	ctx := context.WithValue(context.Background(), ctxUserIDKey{}, userUUID)

	// 测试混合奖励：钱包 + 道具
	t.Run("混合奖励-钱包和道具", func(t *testing.T) {
		reward := &game.Reward{
			Wallet: &game.Wallet{
				Coin: 100,
				Gem:  50,
			},
			Items: []*game.Item{
				{Id: "item_001", Num: 10},
				{Id: ItemID_Coin, Num: 50}, // 道具形式的金币也会转换为钱包
			},
		}

		walletResult, inventoryResult, err := GrantReward(ctx, logger, db, template, metrics, storageIndex, reward, "test_mixed")
		assert.NoError(t, err)
		assert.NotNil(t, walletResult)
		assert.NotNil(t, walletResult.Updated)
		assert.Equal(t, int32(150), walletResult.Updated.Coin) // 100 + 50
		assert.Equal(t, int32(50), walletResult.Updated.Gem)

		assert.NotNil(t, inventoryResult)
		assert.NotNil(t, inventoryResult.Updated)
		assert.Len(t, inventoryResult.Updated, 1)
		assert.Equal(t, "item_001", inventoryResult.Updated[0].Id)
		assert.Equal(t, int32(10), inventoryResult.Updated[0].Num)
	})
}

func TestGrantReward_NilReward(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过需要数据库的测试")
	}

	db := NewDB(t)
	logger := NewConsoleLogger(os.Stdout, false)
	template := &mockTemplateManager{}
	metrics := &mockMetrics{}
	storageIndex := &mockStorageIndex{}

	userID, _, _, err := AuthenticateCustom(context.Background(), logger, db, uuid.Must(uuid.NewV4()).String(), uuid.Must(uuid.NewV4()).String(), true)
	assert.NoError(t, err)

	userUUID := uuid.FromStringOrNil(userID)
	ctx := context.WithValue(context.Background(), ctxUserIDKey{}, userUUID)

	// 测试 nil 奖励
	t.Run("nil奖励", func(t *testing.T) {
		walletResult, inventoryResult, err := GrantReward(ctx, logger, db, template, metrics, storageIndex, nil, "test_nil")
		assert.NoError(t, err)
		assert.Nil(t, walletResult)
		assert.Nil(t, inventoryResult)
	})
}

func TestGrantReward_EmptyReward(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过需要数据库的测试")
	}

	db := NewDB(t)
	logger := NewConsoleLogger(os.Stdout, false)
	template := &mockTemplateManager{}
	metrics := &mockMetrics{}
	storageIndex := &mockStorageIndex{}

	userID, _, _, err := AuthenticateCustom(context.Background(), logger, db, uuid.Must(uuid.NewV4()).String(), uuid.Must(uuid.NewV4()).String(), true)
	assert.NoError(t, err)

	userUUID := uuid.FromStringOrNil(userID)
	ctx := context.WithValue(context.Background(), ctxUserIDKey{}, userUUID)

	// 测试空奖励
	t.Run("空奖励", func(t *testing.T) {
		reward := &game.Reward{
			Wallet: &game.Wallet{}, // 所有值都是0
			Items:  []*game.Item{},
		}

		walletResult, inventoryResult, err := GrantReward(ctx, logger, db, template, metrics, storageIndex, reward, "test_empty")
		assert.NoError(t, err)
		assert.Nil(t, walletResult)
		assert.Nil(t, inventoryResult)
	})
}

func TestGrantReward_InvalidItems(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过需要数据库的测试")
	}

	db := NewDB(t)
	logger := NewConsoleLogger(os.Stdout, false)
	template := &mockTemplateManager{}
	metrics := &mockMetrics{}
	storageIndex := &mockStorageIndex{}

	userID, _, _, err := AuthenticateCustom(context.Background(), logger, db, uuid.Must(uuid.NewV4()).String(), uuid.Must(uuid.NewV4()).String(), true)
	assert.NoError(t, err)

	userUUID := uuid.FromStringOrNil(userID)
	ctx := context.WithValue(context.Background(), ctxUserIDKey{}, userUUID)

	// 测试无效道具（nil、空ID、数量为0）
	t.Run("无效道具", func(t *testing.T) {
		reward := &game.Reward{
			Items: []*game.Item{
				nil,                       // nil 道具
				{Id: "", Num: 10},         // 空ID
				{Id: "item_001", Num: 0},  // 数量为0
				{Id: "item_002", Num: -5}, // 负数
				{Id: "item_003", Num: 10}, // 有效道具
			},
		}

		walletResult, inventoryResult, err := GrantReward(ctx, logger, db, template, metrics, storageIndex, reward, "test_invalid_items")
		assert.NoError(t, err)
		assert.Nil(t, walletResult)
		assert.NotNil(t, inventoryResult)
		assert.NotNil(t, inventoryResult.Updated)
		assert.Len(t, inventoryResult.Updated, 1) // 只有 item_003 有效
		assert.Equal(t, "item_003", inventoryResult.Updated[0].Id)
		assert.Equal(t, int32(10), inventoryResult.Updated[0].Num)
	})
}

func TestGrantReward_EquipmentUnlockAndConvert(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过需要数据库的测试")
	}

	db := NewDB(t)
	logger := NewConsoleLogger(os.Stdout, false)
	template := &mockTemplateManager{}
	metrics := &mockMetrics{}
	storageIndex := &mockStorageIndex{}

	userID, _, _, err := AuthenticateCustom(context.Background(), logger, db, uuid.Must(uuid.NewV4()).String(), uuid.Must(uuid.NewV4()).String(), true)
	assert.NoError(t, err)

	userUUID := uuid.FromStringOrNil(userID)
	ctx := context.WithValue(context.Background(), ctxUserIDKey{}, userUUID)

	// 测试连续两次抽到同一个装备
	t.Run("连续两次抽到同一装备", func(t *testing.T) {
		equipID := "EQ1001" // 测试装备ID

		// 第一次：解锁装备
		firstReward := &game.Reward{
			Items: []*game.Item{
				{Id: equipID, Num: 1},
			},
		}

		walletResult1, inventoryResult1, err := GrantReward(ctx, logger, db, template, metrics, storageIndex, firstReward, "test_equip_first")
		assert.NoError(t, err)
		assert.Nil(t, walletResult1)
		// 第一次解锁：装备不发到背包，只有1个时也不发碎片
		if inventoryResult1 != nil && inventoryResult1.Updated != nil {
			// 验证没有发放装备到背包
			for _, item := range inventoryResult1.Updated {
				assert.NotEqual(t, equipID, item.Id, "第一次解锁时不应该发放装备到背包")
			}
		}

		// 验证装备已解锁：通过再次发放相同装备来验证
		// 第二次：应该转换为碎片
		secondReward := &game.Reward{
			Items: []*game.Item{
				{Id: equipID, Num: 1},
			},
		}

		walletResult2, inventoryResult2, err := GrantReward(ctx, logger, db, template, metrics, storageIndex, secondReward, "test_equip_second")
		assert.NoError(t, err)
		assert.Nil(t, walletResult2)
		assert.NotNil(t, inventoryResult2)
		assert.NotNil(t, inventoryResult2.Updated)

		// 验证第二次发放：应该转换为碎片（EQ1001 -> 1001，数量 × 10）
		debrisID := "1001"               // EQ1001 去掉 EQ 前缀
		expectedDebrisCount := int32(10) // 1个装备 × 10 = 10个碎片

		foundDebris := false
		for _, item := range inventoryResult2.Updated {
			if item.Id == debrisID {
				assert.Equal(t, expectedDebrisCount, item.Num, "第二次应该转换为10个碎片")
				foundDebris = true
				break
			}
			// 确保没有再次发放装备
			assert.NotEqual(t, equipID, item.Id, "第二次不应该再发放装备")
		}
		assert.True(t, foundDebris, "第二次应该转换为碎片")
	})

	// 测试连续两次抽到多个相同装备
	t.Run("连续两次抽到多个相同装备", func(t *testing.T) {
		equipID := "EQ1002"    // 另一个测试装备ID
		equipCount := int32(2) // 每次抽到2个

		// 第一次：解锁装备（数量大于1时，第1个解锁，剩下的转换为碎片）
		firstReward := &game.Reward{
			Items: []*game.Item{
				{Id: equipID, Num: equipCount},
			},
		}

		_, inventoryResult1, err := GrantReward(ctx, logger, db, template, metrics, storageIndex, firstReward, "test_equip_multi_first")
		assert.NoError(t, err)
		assert.NotNil(t, inventoryResult1)

		// 验证第一次发放：第1个装备解锁（不发到背包），第2个转换为碎片
		foundDebris := false
		debrisID := "1002"
		for _, item := range inventoryResult1.Updated {
			// 验证没有发放装备到背包
			assert.NotEqual(t, equipID, item.Id, "第一次解锁时不应该发放装备到背包")
			if item.Id == debrisID {
				assert.Equal(t, int32(10), item.Num, "第一次应该转换10个碎片（第2个装备转换）")
				foundDebris = true
			}
		}
		assert.True(t, foundDebris, "第一次应该转换10个碎片")

		// 第二次：应该转换为碎片
		secondReward := &game.Reward{
			Items: []*game.Item{
				{Id: equipID, Num: equipCount},
			},
		}

		_, inventoryResult2, err := GrantReward(ctx, logger, db, template, metrics, storageIndex, secondReward, "test_equip_multi_second")
		assert.NoError(t, err)
		assert.NotNil(t, inventoryResult2)

		// 验证第二次发放：应该转换为碎片（2个装备 × 10 = 20个碎片）
		expectedDebrisCount := equipCount * 10 // 2 × 10 = 20

		foundDebris = false
		for _, item := range inventoryResult2.Updated {
			if item.Id == debrisID {
				assert.Equal(t, expectedDebrisCount, item.Num, "第二次应该转换为20个碎片")
				foundDebris = true
				break
			}
		}
		assert.True(t, foundDebris, "第二次应该转换为碎片")
	})
}
