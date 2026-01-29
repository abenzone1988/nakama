package server

//
//import (
//	"context"
//
//	"github.com/heroiclabs/nakama/v3/game"
//	"go.uber.org/zap"
//)
//
//func (s *ApiServer) PurchaseTest(ctx context.Context, in *game.PurchaseRequest) (*game.PurchaseResponse, error) {
//	// 将 protobuf 消息转换为 PurchaseNotifyRequest
//	req := &PurchaseNotifyRequest{
//		Site:       in.Site,
//		OrderID:    in.OrderId,
//		UID:        in.Uid,
//		SID:        in.Sid,
//		CPOrderID:  in.CpOrderId,
//		RoleID:     in.Roleid,
//		RoleName:   in.Rolename,
//		OrderMoney: in.OrderMoney,
//		ProductID:  in.Productid,
//		PayType:    int(in.PayType),
//		Ext:        in.Ext,
//		Time:       in.Time,
//		Sign:       in.Sign,
//	}
//
//	s.logger.Info("收到测试购买请求（跳过验证）",
//		zap.String("site", req.Site),
//		zap.String("order_id", req.OrderID),
//		zap.String("uid", req.UID),
//		zap.String("order_money", req.OrderMoney),
//		zap.String("cp_order_id", req.CPOrderID))
//
//	// 跳过所有验证，直接处理发货逻辑
//	if err := s.processPurchaseDelivery(ctx, req); err != nil {
//		s.logger.Error("测试处理发货失败",
//			zap.Error(err),
//			zap.String("order_id", req.OrderID))
//		return &game.PurchaseResponse{
//			Code: 1,
//			Msg:  err.Error(),
//		}, nil
//	}
//
//	// 返回成功
//	s.logger.Info("测试购买通知处理成功", zap.String("cp_order_id", req.CPOrderID))
//	return &game.PurchaseResponse{
//		Code: 0,
//		Msg:  "success",
//	}, nil
//}
