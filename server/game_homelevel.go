package server

import (
	"context"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// HomeLevelData 关卡数据结构 - 所有关卡统一存储
type HomeLevelData struct {
	LevelData  map[string]*HomeLevelInfo `json:"level_data"`   // 关卡ID -> 关卡信息
	MaxLevelId string                    `json:"max_level_id"` // 用户达到的最大关卡ID
}

// HomeLevelInfo 单个关卡信息
type HomeLevelInfo struct {
	LevelID       string    `json:"level_id"`
	HasChallenged bool      `json:"has_challenged"`
	IsPassed      bool      `json:"is_passed"`
	RemainHp      float32   `json:"remain_hp"`
	RewardGot     int32     `json:"reward_got"`
	UpdateTime    time.Time `json:"update_time"`
}

// GetCollection 实现 Storable 接口
func (h *HomeLevelData) GetCollection() string {
	return "Home"
}

// GetKey 实现 Storable 接口
func (h *HomeLevelData) GetKey() string {
	return "Level"
}

// Init 实现 Storable 接口
func (h *HomeLevelData) Init() {
	h.LevelData = make(map[string]*HomeLevelInfo)
	h.MaxLevelId = ""
}

// compareLevelIds 比较两个关卡ID的大小，格式如 L10011
// 返回 true 如果 levelId1 > levelId2
func compareLevelIds(levelId1, levelId2 string) bool {
	// 如果其中一个为空，直接返回
	if levelId1 == "" {
		return false
	}
	if levelId2 == "" {
		return true
	}

	// 提取数字部分进行比较
	num1 := extractLevelNumber(levelId1)
	num2 := extractLevelNumber(levelId2)

	return num1 > num2
}

// extractLevelNumber 从关卡ID中提取数字部分
func extractLevelNumber(levelId string) int {
	// 移除 "L" 前缀
	if strings.HasPrefix(levelId, "L") {
		numStr := levelId[1:]
		if num, err := strconv.Atoi(numStr); err == nil {
			return num
		}
	}
	return 0
}

// SaveHomeLevelData 保存单个关卡数据（业务层面增量更新）
func (s *ApiServer) SaveHomeLevelData(ctx context.Context, in *game.SaveHomeLevelDataRequest) (*game.SaveHomeLevelDataResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 加载现有的关卡数据
	homeLevelData := &HomeLevelData{}
	err := LoadData(ctx, s.logger, s.db, userID, homeLevelData)
	if err != nil {
		s.logger.Error("加载关卡数据失败", zap.Error(err), zap.String("user_id", userID.String()))
		return &game.SaveHomeLevelDataResponse{
			Code: 1,
			Msg:  "加载关卡数据失败",
		}, nil
	}

	levelID := in.GetLevelId()
	currentTime := time.Now()

	// 检查是否为新记录
	isNewRecord := false
	if _, exists := homeLevelData.LevelData[levelID]; !exists {
		isNewRecord = true
	}

	// 更新关卡数据
	homeLevelData.LevelData[levelID] = &HomeLevelInfo{
		LevelID:       levelID,
		HasChallenged: in.GetHasChallenged(),
		IsPassed:      in.GetIsPassed(),
		RemainHp:      in.GetRemainHp(),
		RewardGot:     in.GetRewardGot(),
		UpdateTime:    currentTime,
	}

	// 更新最大关卡ID
	if in.GetIsPassed() && compareLevelIds(levelID, homeLevelData.MaxLevelId) {
		oldMaxLevelId := homeLevelData.MaxLevelId
		homeLevelData.MaxLevelId = levelID
		s.logger.Info("更新最大关卡ID",
			zap.String("user_id", userID.String()),
			zap.String("old_max_level_id", oldMaxLevelId),
			zap.String("new_max_level_id", levelID),
		)
	}

	// 保存更新后的关卡数据
	err = SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, homeLevelData)
	if err != nil {
		s.logger.Error("保存关卡数据失败", zap.Error(err), zap.String("user_id", userID.String()), zap.String("level_id", levelID))
		return &game.SaveHomeLevelDataResponse{
			Code: 2,
			Msg:  "保存关卡数据失败",
		}, nil
	}

	s.logger.Info("关卡数据保存成功",
		zap.String("user_id", userID.String()),
		zap.String("level_id", levelID),
		zap.String("max_level_id", homeLevelData.MaxLevelId),
		zap.Bool("is_new_record", isNewRecord),
		zap.Bool("is_passed", in.GetIsPassed()),
	)

	return &game.SaveHomeLevelDataResponse{
		Code:        0,
		Msg:         "保存成功",
		IsNewRecord: isNewRecord,
		MaxLevelId:  homeLevelData.MaxLevelId,
	}, nil
}

// GetHomeLevelData 获取关卡数据
func (s *ApiServer) GetHomeLevelData(ctx context.Context, in *game.GetHomeLevelDataRequest) (*game.GetHomeLevelDataResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 加载关卡数据
	homeLevelData := &HomeLevelData{}
	err := LoadData(ctx, s.logger, s.db, userID, homeLevelData)
	if err != nil {
		s.logger.Error("加载关卡数据失败", zap.Error(err), zap.String("user_id", userID.String()))
		return &game.GetHomeLevelDataResponse{
			Code: 1,
			Msg:  "加载关卡数据失败",
		}, nil
	}

	var resultLevelData []*game.HomeLevelData
	requestedLevelIds := in.GetLevelIds()
	limit := in.GetLimit()

	// 如果指定了特定的关卡ID列表
	if len(requestedLevelIds) > 0 {
		for _, levelID := range requestedLevelIds {
			if levelInfo, exists := homeLevelData.LevelData[levelID]; exists {
				resultLevelData = append(resultLevelData, &game.HomeLevelData{
					LevelId:       levelInfo.LevelID,
					HasChallenged: levelInfo.HasChallenged,
					IsPassed:      levelInfo.IsPassed,
					RemainHp:      levelInfo.RemainHp,
					RewardGot:     levelInfo.RewardGot,
					UpdateTime:    timestamppb.New(levelInfo.UpdateTime),
				})
			}
		}
	} else {
		// 返回所有关卡数据
		count := 0
		for _, levelInfo := range homeLevelData.LevelData {
			if limit > 0 && count >= int(limit) {
				break
			}

			resultLevelData = append(resultLevelData, &game.HomeLevelData{
				LevelId:       levelInfo.LevelID,
				HasChallenged: levelInfo.HasChallenged,
				IsPassed:      levelInfo.IsPassed,
				RemainHp:      levelInfo.RemainHp,
				RewardGot:     levelInfo.RewardGot,
				UpdateTime:    timestamppb.New(levelInfo.UpdateTime),
			})
			count++
		}
	}

	s.logger.Debug("获取关卡数据成功",
		zap.String("user_id", userID.String()),
		zap.Int("returned_count", len(resultLevelData)),
		zap.Int("total_count", len(homeLevelData.LevelData)),
		zap.Strings("requested_level_ids", requestedLevelIds),
	)

	return &game.GetHomeLevelDataResponse{
		Code:       0,
		Msg:        "获取成功",
		LevelData:  resultLevelData,
		TotalCount: int32(len(homeLevelData.LevelData)),
		MaxLevelId: homeLevelData.MaxLevelId,
	}, nil
}

func (s *ApiServer) ResetHomeLevelData(ctx context.Context, in *emptypb.Empty) (*emptypb.Empty, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 创建一个新的空的关卡数据结构
	homeLevelData := &HomeLevelData{}
	homeLevelData.Init() // 初始化为空数据

	// 保存重置后的关卡数据
	err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, homeLevelData)
	if err != nil {
		s.logger.Error("重置关卡数据失败", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, err
	}

	s.logger.Info("关卡数据重置成功",
		zap.String("user_id", userID.String()),
		zap.String("operation", "reset_home_level_data"),
	)

	return &emptypb.Empty{}, nil
}
