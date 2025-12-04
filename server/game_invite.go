package server

import (
	"context"
	"fmt"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *ApiServer) SubmitBeInvited(ctx context.Context, in *game.SubmitBeInvitedRequest) (*emptypb.Empty, error) {
	userName := ctx.Value(ctxUsernameKey{}).(string)

	owners, err := GetUsers(ctx, s.logger, s.db, s.statusRegistry, nil, []string{userName}, nil)
	if err != nil {
		return nil, nil
	}

	owner := owners.Users[0]
	t := owner.CreateTime.AsTime()
	now := time.Now()
	if t.Year() != now.Year() || t.YearDay() != now.YearDay() {
		s.logger.Info("不是新用户", zap.String("share_id", in.InviterId))
		return &emptypb.Empty{}, nil
	}

	inviteeData := &InviteData{}
	if err := LoadUserData(ctx, s.logger, s.db, inviteeData); err != nil {
		return nil, err
	}

	if inviteeData.BeInvited {
		s.logger.Info("已接受邀请", zap.String("share_id", in.InviterId))
		return &emptypb.Empty{}, nil
	}

	users, err := GetUsers(ctx, s.logger, s.db, s.statusRegistry, nil, []string{in.InviterId}, nil)
	if err != nil {
		return nil, err
	}
	if users == nil || len(users.Users) == 0 {
		s.logger.Error("邀请人不存在", zap.String("share_id", in.InviterId))
		return nil, nil
	}

	inviter := users.Users[0]
	inviterID, _ := uuid.FromString(inviter.Id)

	inviterData := &InviteData{}
	if err := LoadData(ctx, s.logger, s.db, inviterID, inviterData); err != nil {
		return nil, err
	}

	inviterData.List[userName] = &InviteRecord{Invitee: userName, RewardClaimed: false}
	if err = SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, inviterID, inviterData); err != nil {
		return nil, err
	}

	inviteeData.BeInvited = true
	inviteeData.Inviter = inviter.Username
	if err = SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, inviteeData); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *ApiServer) ListInvitee(ctx context.Context, in *emptypb.Empty) (*game.ListInviteeResponse, error) {
	inviterData := &InviteData{}
	if err := LoadUserData(ctx, s.logger, s.db, inviterData); err != nil {
		return nil, err
	}

	resp := &game.ListInviteeResponse{InviteeIds: []string{}}
	for _, v := range inviterData.List {
		if !v.RewardClaimed {
			if users, err := GetUsers(ctx, s.logger, s.db, s.statusRegistry, nil, []string{v.Invitee}, nil); err != nil {
				return nil, err
			} else {
				if users == nil || len(users.Users) == 0 {
					s.logger.Info("邀请人没注册", zap.String("share_id", v.Invitee))
					continue
				}
				// todo 通关第一关的判断
				inviter := users.Users[0]
				inviterID, _ := uuid.FromString(inviter.Id)

				//加载关卡数据
				homeLevelData := &HomeLevelData{}
				err := LoadData(ctx, s.logger, s.db, inviterID, homeLevelData)
				if err != nil {
					s.logger.Info("邀请人HomeLevelData 加载失败", zap.String("share_id", inviter.Id))
					continue
				}
				if homeLevelData.CurLevelId >= "L1001" {
					resp.InviteeIds = append(resp.InviteeIds, v.Invitee)
				}
			}
		}
	}
	return resp, nil
}

func (s *ApiServer) ClaimInviteReward(ctx context.Context, in *game.ClaimInviteRewardRequest) (*emptypb.Empty, error) {
	inviterData := &InviteData{}
	if err := LoadUserData(ctx, s.logger, s.db, inviterData); err != nil {
		return nil, err
	}

	// 收集所有需要发放的奖励
	var allRewards []*game.Reward
	var validInviteeIds []string

	for _, v := range in.InviteeIds {
		entry, exists := inviterData.List[v]
		if !exists {
			return nil, fmt.Errorf("inviter ID %s does not exist in the list", v)
		}
		if !entry.RewardClaimed {
			// 获取邀请奖励配置
			reward := GetReward(InviteRewardID, s.template.GetTplReward(), s.logger)
			if reward != nil {
				allRewards = append(allRewards, reward)
				validInviteeIds = append(validInviteeIds, v)
				inviterData.List[v].RewardClaimed = true
			} else {
				s.logger.Warn("邀请奖励配置不存在", zap.String("reward_id", InviteRewardID))
			}
		}
	}

	// 如果有有效的奖励，合并并发放
	if len(allRewards) > 0 {
		mergedReward := MergeRewards(allRewards)
		if mergedReward != nil {
			source := fmt.Sprintf("invite_reward_count_%d", len(validInviteeIds))
			_, _, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, mergedReward, source)
			if err != nil {
				s.logger.Error("发放邀请奖励失败", zap.Error(err), zap.Int("count", len(validInviteeIds)))
				return nil, fmt.Errorf("发放邀请奖励失败: %w", err)
			}
		}
	}

	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, inviterData); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
