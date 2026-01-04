package server

// 使用示例：测试 GrantReward 接口
//
// 1. 解析奖励字符串
// rewardStr := "10000_1000,10001_500,1100_1,1101_2"
// 格式：itemid_num,itemid_num
//
// 2. 转换为 game.Reward 结构
// items := strings.Split(rewardStr, ",")
// rewardItems := make([]*game.Item, 0)
// for _, item := range items {
//     parts := strings.Split(strings.TrimSpace(item), "_")
//     if len(parts) != 2 {
//         continue
//     }
//     itemID := strings.TrimSpace(parts[0])
//     count, _ := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 32)
//     rewardItems = append(rewardItems, &game.Item{
//         Id:  itemID,
//         Num: int32(count),
//     })
// }
// reward := &game.Reward{
//     Items: rewardItems,
// }
//
// 3. 调用 GrantReward 发放奖励
// walletResult, inventoryResult, err := GrantReward(
//     ctx, logger, db, templateMgr, metrics, storageIndex,
//     reward, "test_grant_reward")
//
// 4. 查看结果
// if walletResult != nil {
//     logger.Info("钱包变化",
//         zap.Int32("coin_before", walletResult.Previous.Coin),
//         zap.Int32("coin_after", walletResult.Updated.Coin))
// }
// if inventoryResult != nil {
//     for _, item := range inventoryResult.Updated {
//         logger.Info("背包物品", zap.String("id", item.Id), zap.Int32("num", item.Num))
//     }
// }
//
// 随机品质炮台说明：
// - ItemType_RandomQualityTurret (16) 类型的道具会根据 TplItem 中的 quality 字段
// - 从 TplEquipment 表中查找对应品质的炮台（type=1，排除水晶基地）
// - 随机选择一个炮台进行发放
// - 如果炮台已解锁，则转换为 10 个对应的碎片（debrisId = equipID 去掉 "EQ" 前缀）
// - 如果炮台未解锁，则直接解锁该炮台
//
// 例如：
// - 道具 ID "50001" 的 quality=4，会从品质为4的炮台中随机选择
// - 如果抽到 EQ1100（已解锁），则获得 10 个 1100 碎片
// - 如果抽到 EQ1102（未解锁），则直接解锁 EQ1102
