// Copyright 2019 The Nakama Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"context"

	"github.com/heroiclabs/nakama/v3/console"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *ConsoleServer) ReloadTemplate(ctx context.Context, in *emptypb.Empty) (*emptypb.Empty, error) {
	s.template.LoadData()
	s.logger.Info("Reload template data.")
	return &emptypb.Empty{}, nil
}

func (s *ConsoleServer) GetChallengeTemplate(ctx context.Context, in *console.ChallengeTemplateRequest) (*console.ChallengeTemplateResponse, error) {
	if in.Id <= 0 {
		return nil, status.Error(codes.InvalidArgument, "Invalid challenge ID")
	}

	// 从模板管理器中获取挑战赛模板
	tplChallenge := s.template.GetTplChallenge()
	challenge, found := tplChallenge.FindByKey(in.Id)
	if !found {
		return nil, status.Error(codes.NotFound, "Challenge template not found")
	}

	response := &console.ChallengeTemplateResponse{
		Template: &console.ChallengeTemplate{
			Id:            challenge.ID,
			Name:          challenge.Name,
			ActivityId:    challenge.ActivityID,
			OpenTime:      challenge.OpenTime,
			CloseTime:     challenge.CloseTime,
			EndTime:       challenge.EndTime,
			MaxPart:       challenge.MaxPart,
			RewardRemains: challenge.RewardRemains,
			Status:        challenge.Status,
		},
	}

	return response, nil
}

func (s *ConsoleServer) GetAllChallengeTemplates(ctx context.Context, in *emptypb.Empty) (*console.GetAllChallengeTemplatesResponse, error) {
	// 从模板管理器中获取所有挑战赛模板
	tplChallenge := s.template.GetTplChallenge()
	allChallenges := tplChallenge.FindAll()

	if allChallenges.Len() == 0 {
		return &console.GetAllChallengeTemplatesResponse{
			Templates: []*console.ChallengeTemplate{},
		}, nil
	}

	// 转换为响应格式
	templates := make([]*console.ChallengeTemplate, 0, allChallenges.Len())
	for i := 0; i < allChallenges.Len(); i++ {
		challenge := allChallenges.Get(i)
		template := &console.ChallengeTemplate{
			Id:            challenge.ID,
			Name:          challenge.Name,
			ActivityId:    challenge.ActivityID,
			OpenTime:      challenge.OpenTime,
			CloseTime:     challenge.CloseTime,
			EndTime:       challenge.EndTime,
			MaxPart:       challenge.MaxPart,
			RewardRemains: challenge.RewardRemains,
			Status:        challenge.Status,
		}
		templates = append(templates, template)
	}

	response := &console.GetAllChallengeTemplatesResponse{
		Templates: templates,
	}

	return response, nil
}
