package test

import (
	"testing"

	"github.com/heroiclabs/nakama/v3/server"
	"github.com/stretchr/testify/assert"
)

func TestWalletSignatureCompatibility(t *testing.T) {
	// 测试数据
	operationType := "gain"
	coin := int64(100)
	gem := int64(50)
	ad := int64(25)
	reason := "测试奖励"
	userID := "550e8400-e29b-41d4-a716-446655440000"

	// 测试1：生成包含用户ID的签名，验证应该成功
	signatureWithUserID, err := server.GenerateWalletSignatureWithUserID(operationType, coin, gem, ad, reason, userID)
	assert.NoError(t, err)
	assert.NotEmpty(t, signatureWithUserID)

	// 验证包含用户ID的签名
	isValidWithUserID := server.VerifyWalletSignature(operationType, coin, gem, ad, reason, signatureWithUserID, userID)
	assert.True(t, isValidWithUserID)

	// 测试2：生成不包含用户ID的签名，验证应该成功
	signatureWithoutUserID, err := server.GenerateWalletSignatureWithoutUserID(operationType, coin, gem, ad, reason)
	assert.NoError(t, err)
	assert.NotEmpty(t, signatureWithoutUserID)

	// 验证不包含用户ID的签名
	isValidWithoutUserID := server.VerifyWalletSignature(operationType, coin, gem, ad, reason, signatureWithoutUserID, userID)
	assert.True(t, isValidWithoutUserID)

	// 测试3：验证两种签名不应该相同
	assert.NotEqual(t, signatureWithUserID, signatureWithoutUserID)
}

func TestWalletSignatureBackwardCompatibility(t *testing.T) {
	// 测试向后兼容性
	operationType := "consume"
	coin := int64(10)
	gem := int64(5)
	ad := int64(2)
	reason := "购买道具"
	userID := "550e8400-e29b-41d4-a716-446655440000"

	// 生成旧版本的签名（不包含用户ID）
	legacySignature, err := server.GenerateWalletSignatureWithoutUserID(operationType, coin, gem, ad, reason)
	assert.NoError(t, err)

	// 使用新版本的验证函数验证旧版本签名
	isValid := server.VerifyWalletSignature(operationType, coin, gem, ad, reason, legacySignature, userID)
	assert.True(t, isValid, "旧版本签名应该能被新版本验证函数验证通过")
}

func TestWalletSignatureForwardCompatibility(t *testing.T) {
	// 测试向前兼容性
	operationType := "gain"
	coin := int64(100)
	gem := int64(50)
	ad := int64(25)
	reason := "测试奖励"
	userID := "550e8400-e29b-41d4-a716-446655440000"

	// 生成新版本的签名（包含用户ID）
	newSignature, err := server.GenerateWalletSignatureWithUserID(operationType, coin, gem, ad, reason, userID)
	assert.NoError(t, err)

	// 使用新版本的验证函数验证新版本签名
	isValid := server.VerifyWalletSignature(operationType, coin, gem, ad, reason, newSignature, userID)
	assert.True(t, isValid, "新版本签名应该能被新版本验证函数验证通过")
}

func TestWalletSignatureCrossValidation(t *testing.T) {
	// 测试交叉验证
	operationType := "gain"
	coin := int64(100)
	gem := int64(50)
	ad := int64(25)
	reason := "测试奖励"
	userID1 := "550e8400-e29b-41d4-a716-446655440000"
	userID2 := "550e8400-e29b-41d4-a716-446655440001"

	// 为用户1生成包含用户ID的签名
	signature1, err := server.GenerateWalletSignatureWithUserID(operationType, coin, gem, ad, reason, userID1)
	assert.NoError(t, err)

	// 为用户2生成包含用户ID的签名
	signature2, err := server.GenerateWalletSignatureWithUserID(operationType, coin, gem, ad, reason, userID2)
	assert.NoError(t, err)

	// 验证各自的签名
	isValid1 := server.VerifyWalletSignature(operationType, coin, gem, ad, reason, signature1, userID1)
	isValid2 := server.VerifyWalletSignature(operationType, coin, gem, ad, reason, signature2, userID2)

	assert.True(t, isValid1)
	assert.True(t, isValid2)

	// 验证交叉验证应该失败
	isValidCross1 := server.VerifyWalletSignature(operationType, coin, gem, ad, reason, signature1, userID2)
	isValidCross2 := server.VerifyWalletSignature(operationType, coin, gem, ad, reason, signature2, userID1)

	assert.False(t, isValidCross1)
	assert.False(t, isValidCross2)
}

func TestWalletSignatureConfigurationSwitch(t *testing.T) {
	// 测试配置开关功能
	operationType := "gain"
	coin := int64(100)
	gem := int64(50)
	ad := int64(25)
	reason := "测试奖励"
	userID := "550e8400-e29b-41d4-a716-446655440000"

	// 保存原始配置
	originalRequireUserID := server.DefaultSignatureConfig.RequireUserID

	// 测试1：RequireUserID = false（兼容模式）
	server.DefaultSignatureConfig.RequireUserID = false

	// 生成不包含用户ID的签名
	signatureWithoutUserID, err := server.GenerateWalletSignatureWithoutUserID(operationType, coin, gem, ad, reason)
	assert.NoError(t, err)

	// 验证应该成功
	isValid := server.VerifyWalletSignature(operationType, coin, gem, ad, reason, signatureWithoutUserID, userID)
	assert.True(t, isValid)

	// 测试2：RequireUserID = true（严格模式）
	server.DefaultSignatureConfig.RequireUserID = true

	// 生成包含用户ID的签名
	signatureWithUserID, err := server.GenerateWalletSignatureWithUserID(operationType, coin, gem, ad, reason, userID)
	assert.NoError(t, err)

	// 验证应该成功
	isValid = server.VerifyWalletSignature(operationType, coin, gem, ad, reason, signatureWithUserID, userID)
	assert.True(t, isValid)

	// 验证旧版本签名应该失败
	isValid = server.VerifyWalletSignature(operationType, coin, gem, ad, reason, signatureWithoutUserID, userID)
	assert.False(t, isValid)

	// 恢复原始配置
	server.DefaultSignatureConfig.RequireUserID = originalRequireUserID
}
