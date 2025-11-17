package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SignatureConfig 签名配置
type SignatureConfig struct {
	SecretKey string
	Algorithm string
}

// DefaultSignatureConfig 默认签名配置
var DefaultSignatureConfig = SignatureConfig{
	SecretKey: "sparkinfi-game-secret-key", // 在生产环境中应该从配置文件或环境变量中获取
	Algorithm: "HMAC-SHA256",
}

// SignatureVerificationRequest 签名验证请求参数
type SignatureVerificationRequest struct {
	RecordType string // "leaderboard" or "tournament"
	RecordID   string // leaderboard_id or tournament_id
	Score      int64
	Subscore   int64
	Metadata   string
	UserID     string
	Username   string
	Logger     *zap.Logger
}

// VerifyRecordSignature 统一的记录签名验证函数
func VerifyRecordSignature(req *SignatureVerificationRequest) (string, error) {
	// 解析metadata JSON
	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(req.Metadata), &metadata); err != nil {
		return "", status.Error(codes.InvalidArgument, "Invalid metadata JSON format.")
	}

	// 检查是否包含签名字段
	signature, exists := metadata["signature"]
	if !exists {
		req.Logger.Warn("Record submission without signature rejected - signature field missing",
			zap.String("user_id", req.UserID),
			zap.String("username", req.Username),
			zap.String("record_type", req.RecordType),
			zap.String("record_id", req.RecordID))
		return "", status.Error(codes.InvalidArgument, "Signature field is required in metadata.")
	}

	signatureStr, ok := signature.(string)
	if !ok {
		return "", status.Error(codes.InvalidArgument, "Signature must be a string.")
	}

	// 创建用于验证的 metadata（移除签名字段）
	metadataForVerification := make(map[string]interface{})
	for k, v := range metadata {
		if k != "signature" {
			metadataForVerification[k] = v
		}
	}

	// 将 metadata 转换为 JSON 字符串用于签名验证
	metadataBytes, err := json.Marshal(metadataForVerification)
	if err != nil {
		return "", status.Error(codes.InvalidArgument, "Error processing metadata for signature verification.")
	}

	// 根据记录类型验证签名
	var isValid bool
	switch req.RecordType {
	case "leaderboard":
		isValid = VerifyLeaderboardRecordSignature(req.RecordID, req.Score, req.Subscore, string(metadataBytes), signatureStr)
	case "tournament":
		isValid = VerifyTournamentRecordSignature(req.RecordID, req.Score, req.Subscore, string(metadataBytes), signatureStr)
	default:
		return "", status.Error(codes.Internal, "Invalid record type for signature verification.")
	}

	if !isValid {
		// 记录签名验证失败的日志
		req.Logger.Warn(fmt.Sprintf("%s record signature verification failed", strings.Title(req.RecordType)),
			zap.String("user_id", req.UserID),
			zap.String("username", req.Username),
			zap.String(fmt.Sprintf("%s_id", req.RecordType), req.RecordID),
			zap.Int64("score", req.Score),
			zap.Int64("subscore", req.Subscore),
			zap.String("metadata", string(metadataBytes)),
			zap.String("signature", signatureStr))
		return "", status.Error(codes.InvalidArgument, "Invalid signature. The data may have been tampered with.")
	}

	// 验证成功，返回处理后的metadata（移除签名字段）
	updatedMetadata, _ := json.Marshal(metadataForVerification)
	return string(updatedMetadata), nil
}

// generateSignature 生成签名
func generateSignature(data map[string]interface{}, secretKey string) (string, error) {
	// 将 map 转换为有序的字符串
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, key := range keys {
		value := data[key]
		var valueStr string

		switch v := value.(type) {
		case string:
			valueStr = v
		case int64:
			valueStr = strconv.FormatInt(v, 10)
		case float64:
			valueStr = strconv.FormatFloat(v, 'f', -1, 64)
		case bool:
			valueStr = strconv.FormatBool(v)
		default:
			valueStr = fmt.Sprintf("%v", v)
		}

		parts = append(parts, fmt.Sprintf("%s=%s", key, valueStr))
	}

	// 生成签名字符串
	signStr := strings.Join(parts, "&")

	// 使用 HMAC-SHA256 生成签名
	h := hmac.New(sha256.New, []byte(secretKey))
	h.Write([]byte(signStr))
	signature := hex.EncodeToString(h.Sum(nil))

	return signature, nil
}

// verifySignature 验证签名
func verifySignature(data map[string]interface{}, signature, secretKey string) bool {
	expectedSignature, err := generateSignature(data, secretKey)
	if err != nil {
		return false
	}

	// 使用恒定时间比较以防止时序攻击
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

// VerifyLeaderboardRecordSignature 验证排行榜记录签名
func VerifyLeaderboardRecordSignature(leaderboardID string, score, subscore int64, metadata, signature string) bool {
	data := map[string]interface{}{
		"leaderboard_id": leaderboardID,
		"score":          score,
		"subscore":       subscore,
		"metadata":       metadata,
	}

	return verifySignature(data, signature, DefaultSignatureConfig.SecretKey)
}

// VerifyTournamentRecordSignature 验证锦标赛记录签名
func VerifyTournamentRecordSignature(tournamentID string, score, subscore int64, metadata, signature string) bool {
	data := map[string]interface{}{
		"tournament_id": tournamentID,
		"score":         score,
		"subscore":      subscore,
		"metadata":      metadata,
	}

	return verifySignature(data, signature, DefaultSignatureConfig.SecretKey)
}

// GenerateLeaderboardRecordSignature 生成排行榜记录签名（用于测试或客户端参考）
func GenerateLeaderboardRecordSignature(leaderboardID string, score, subscore int64, metadata string) (string, error) {
	data := map[string]interface{}{
		"leaderboard_id": leaderboardID,
		"score":          score,
		"subscore":       subscore,
		"metadata":       metadata,
	}

	return generateSignature(data, DefaultSignatureConfig.SecretKey)
}

// GenerateTournamentRecordSignature 生成锦标赛记录签名（用于测试或客户端参考）
func GenerateTournamentRecordSignature(tournamentID string, score, subscore int64, metadata string) (string, error) {
	data := map[string]interface{}{
		"tournament_id": tournamentID,
		"score":         score,
		"subscore":      subscore,
		"metadata":      metadata,
	}

	return generateSignature(data, DefaultSignatureConfig.SecretKey)
}

// VerifyWalletSignature 验证钱包操作签名
func VerifyWalletSignature(operationType string, coin, gem, ad int64, reason, signature string) bool {
	data := map[string]interface{}{
		"operation_type": operationType,
		"coin":           coin,
		"gem":            gem,
		"ad":             ad,
		"reason":         reason,
	}
	return verifySignature(data, signature, DefaultSignatureConfig.SecretKey)
}

// VerifyWalletSignatureV2 验证钱包操作签名
func VerifyWalletSignatureV2(operationType string, coin, gem, ad int64, reason, signature, userID, walletID string) bool {
	data := map[string]interface{}{
		"operation_type": operationType,
		"coin":           coin,
		"gem":            gem,
		"ad":             ad,
		"reason":         reason,
		"user_id":        userID,
		"wallet_id":      walletID,
	}
	return verifySignature(data, signature, DefaultSignatureConfig.SecretKey)

}

func VerifyInventorySignature(operationType string, items []*game.Item, reason, signature, userID, inventoryID string) bool {
	data := map[string]interface{}{
		"operation_type": operationType,
		"items":          items,
		"reason":         reason,
		"user_id":        userID,
		"inventory_id":   inventoryID,
	}
	return verifySignature(data, signature, DefaultSignatureConfig.SecretKey)

}

// GenerateWalletSignature 生成钱包操作签名
func GenerateWalletSignature(operationType string, coin, gem, ad int64, reason string) (string, error) {
	data := map[string]interface{}{
		"operation_type": operationType,
		"coin":           coin,
		"gem":            gem,
		"ad":             ad,
		"reason":         reason,
	}

	return generateSignature(data, DefaultSignatureConfig.SecretKey)
}

// GenerateWalletSignatureV2 生成钱包操作签名
func GenerateWalletSignatureV2(operationType string, coin, gem, ad int64, reason, userID, walletID string) (string, error) {
	// 验证walletID是否为有效的UUID（如果提供了walletID）
	if walletID != "" {
		if _, err := uuid.FromString(walletID); err != nil {
			return "", fmt.Errorf("invalid walletID format: %v", err)
		}
	}

	data := map[string]interface{}{
		"operation_type": operationType,
		"coin":           coin,
		"gem":            gem,
		"ad":             ad,
		"reason":         reason,
		"user_id":        userID,
		"wallet_id":      walletID,
	}

	return generateSignature(data, DefaultSignatureConfig.SecretKey)
}

// VerifyStorageSignature 验证storage操作签名
func VerifyStorageSignature(collection, key, value, userID, signature string) bool {
	data := map[string]interface{}{
		"collection": collection,
		"key":        key,
		"value":      value,
		"user_id":    userID,
	}
	return verifySignature(data, signature, DefaultSignatureConfig.SecretKey)
}

// GenerateStorageSignature 生成storage操作签名
func GenerateStorageSignature(collection, key, value, userID string) (string, error) {
	data := map[string]interface{}{
		"collection": collection,
		"key":        key,
		"value":      value,
		"user_id":    userID,
	}
	return generateSignature(data, DefaultSignatureConfig.SecretKey)
}

// GenerateVipSignature 生成VIP操作签名
func GenerateVipSignature(userID string, expiryTime int64) (string, error) {
	data := map[string]interface{}{
		"user_id":     userID,
		"expiry_time": expiryTime,
	}
	return generateSignature(data, DefaultSignatureConfig.SecretKey)
}
