package server

import (
	"context"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestSignatureVerificationLogging(t *testing.T) {
	// 创建一个观察者用于捕获日志
	observedZapCore, observedLogs := observer.New(zapcore.InfoLevel)
	logger := zap.New(observedZapCore)

	// 创建一个模拟的 ApiServer
	server := &ApiServer{
		logger: logger,
	}

	// 测试用户信息
	userID := uuid.Must(uuid.NewV4())
	username := "test_user"

	// 创建带有用户信息的 context
	ctx := context.WithValue(context.Background(), ctxUserIDKey{}, userID)
	ctx = context.WithValue(ctx, ctxUsernameKey{}, username)
	ctx = context.WithValue(ctx, ctxVarsKey{}, map[string]string{})
	ctx = context.WithValue(ctx, ctxExpiryKey{}, time.Now().Unix()+3600)

	// 测试排行榜记录签名验证失败的日志
	t.Run("Leaderboard signature verification failure logging", func(t *testing.T) {
		// 准备请求数据，使用无效的签名
		request := &api.WriteLeaderboardRecordRequest{
			LeaderboardId: "test-leaderboard",
			Record: &api.WriteLeaderboardRecordRequest_LeaderboardRecordWrite{
				Score:    1000,
				Subscore: 500,
				Metadata: `{"level": 1, "signature": "invalid_signature"}`,
			},
		}

		// 调用函数（这会失败并记录日志）
		_, err := server.WriteLeaderboardRecord(ctx, request)
		if err == nil {
			t.Error("Expected error for invalid signature, but got none")
		}

		// 检查是否记录了相应的日志
		logEntries := observedLogs.FilterMessage("Leaderboard record signature verification failed")
		if len(logEntries.All()) == 0 {
			t.Error("Expected signature verification failure log, but found none")
		}

		// 验证日志中包含了正确的字段
		if len(logEntries.All()) > 0 {
			logEntry := logEntries.All()[0]

			// 检查必要的字段
			fields := logEntry.Context
			foundUserID := false
			foundUsername := false
			foundLeaderboardID := false

			for _, field := range fields {
				switch field.Key {
				case "user_id":
					if field.String == userID.String() {
						foundUserID = true
					}
				case "username":
					if field.String == username {
						foundUsername = true
					}
				case "leaderboard_id":
					if field.String == "test-leaderboard" {
						foundLeaderboardID = true
					}
				}
			}

			if !foundUserID {
				t.Error("Log entry should contain user_id field")
			}
			if !foundUsername {
				t.Error("Log entry should contain username field")
			}
			if !foundLeaderboardID {
				t.Error("Log entry should contain leaderboard_id field")
			}
		}
	})

	// 清除之前的日志
	observedLogs.TakeAll()

	// 测试锦标赛记录签名验证失败的日志
	t.Run("Tournament signature verification failure logging", func(t *testing.T) {
		// 准备请求数据，使用无效的签名
		request := &api.WriteTournamentRecordRequest{
			TournamentId: "test-tournament",
			Record: &api.WriteTournamentRecordRequest_TournamentRecordWrite{
				Score:    2000,
				Subscore: 1000,
				Metadata: `{"round": 1, "signature": "invalid_signature"}`,
			},
		}

		// 调用函数（这会失败并记录日志）
		_, err := server.WriteTournamentRecord(ctx, request)
		if err == nil {
			t.Error("Expected error for invalid signature, but got none")
		}

		// 检查是否记录了相应的日志
		logEntries := observedLogs.FilterMessage("Tournament record signature verification failed")
		if len(logEntries.All()) == 0 {
			t.Error("Expected signature verification failure log, but found none")
		}

		// 验证日志中包含了正确的字段
		if len(logEntries.All()) > 0 {
			logEntry := logEntries.All()[0]

			// 检查必要的字段
			fields := logEntry.Context
			foundUserID := false
			foundUsername := false
			foundTournamentID := false

			for _, field := range fields {
				switch field.Key {
				case "user_id":
					if field.String == userID.String() {
						foundUserID = true
					}
				case "username":
					if field.String == username {
						foundUsername = true
					}
				case "tournament_id":
					if field.String == "test-tournament" {
						foundTournamentID = true
					}
				}
			}

			if !foundUserID {
				t.Error("Log entry should contain user_id field")
			}
			if !foundUsername {
				t.Error("Log entry should contain username field")
			}
			if !foundTournamentID {
				t.Error("Log entry should contain tournament_id field")
			}
		}
	})
}
