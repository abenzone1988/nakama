package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWalletSignatureWithEmptyValues(t *testing.T) {
	// 测试空值情况
	operationType := "gain"
	coin := int64(0)
	gem := int64(0)
	ad := int64(0)
	reason := ""

	// 生成签名
	signature, err := GenerateWalletSignature(operationType, coin, gem, ad, reason)
	assert.NoError(t, err)
	assert.NotEmpty(t, signature)

	// 验证签名
	isValid := VerifyWalletSignature(operationType, coin, gem, ad, reason, signature)
	assert.True(t, isValid)
}

func TestWalletSignatureConsistency(t *testing.T) {
	// 测试签名的一致性
	operationType := "gain"
	coin := int64(100)
	gem := int64(50)
	ad := int64(25)
	reason := "测试奖励"

	// 生成两次签名，应该相同
	signature1, err1 := GenerateWalletSignature(operationType, coin, gem, ad, reason)
	signature2, err2 := GenerateWalletSignature(operationType, coin, gem, ad, reason)

	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.Equal(t, signature1, signature2)
}

func TestWalletSignatureDifferentUsers(t *testing.T) {
	// 测试不同用户的签名
	operationType := "gain"
	coin := int64(100)
	gem := int64(50)
	ad := int64(25)
	reason := "测试奖励"
	userID1 := "550e8400-e29b-41d4-a716-446655440000"
	userID2 := "550e8400-e29b-41d4-a716-446655440001"
	recordID := "550e8400-e29b-41d4-a716-446655440002"

	// 生成两个不同用户的签名
	signature1, err1 := GenerateWalletSignatureV2(operationType, coin, gem, ad, reason, userID1, recordID)
	signature2, err2 := GenerateWalletSignatureV2(operationType, coin, gem, ad, reason, userID2, recordID)

	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.NotEqual(t, signature1, signature2) // 不同用户应该有不同的签名

	// 验证各自的签名
	isValid1 := VerifyWalletSignatureV2(operationType, coin, gem, ad, reason, signature1, userID1, recordID)
	isValid2 := VerifyWalletSignatureV2(operationType, coin, gem, ad, reason, signature2, userID2, recordID)

	assert.True(t, isValid1)
	assert.True(t, isValid2)

	// 验证交叉验证应该失败
	isValidCross1 := VerifyWalletSignatureV2(operationType, coin, gem, ad, reason, signature1, userID2, recordID)
	isValidCross2 := VerifyWalletSignatureV2(operationType, coin, gem, ad, reason, signature2, userID1, recordID)

	assert.False(t, isValidCross1)
	assert.False(t, isValidCross2)
}

func TestWalletSignatureDifferentRecords(t *testing.T) {
	// 测试不同记录ID的签名
	operationType := "gain"
	coin := int64(100)
	gem := int64(50)
	ad := int64(25)
	reason := "测试奖励"
	userID := "550e8400-e29b-41d4-a716-446655440000"
	recordID1 := "550e8400-e29b-41d4-a716-446655440001"
	recordID2 := "550e8400-e29b-41d4-a716-446655440002"

	// 生成两个不同记录ID的签名
	signature1, err1 := GenerateWalletSignatureV2(operationType, coin, gem, ad, reason, userID, recordID1)
	signature2, err2 := GenerateWalletSignatureV2(operationType, coin, gem, ad, reason, userID, recordID2)

	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.NotEqual(t, signature1, signature2) // 不同记录ID应该有不同的签名

	// 验证各自的签名
	isValid1 := VerifyWalletSignatureV2(operationType, coin, gem, ad, reason, signature1, userID, recordID1)
	isValid2 := VerifyWalletSignatureV2(operationType, coin, gem, ad, reason, signature2, userID, recordID2)

	assert.True(t, isValid1)
	assert.True(t, isValid2)

	// 验证交叉验证应该失败
	isValidCross1 := VerifyWalletSignatureV2(operationType, coin, gem, ad, reason, signature1, userID, recordID2)
	isValidCross2 := VerifyWalletSignatureV2(operationType, coin, gem, ad, reason, signature2, userID, recordID1)

	assert.False(t, isValidCross1)
	assert.False(t, isValidCross2)
}

func TestWalletSignatureCompatibilityMode(t *testing.T) {
	// 测试数据
	operationType := "gain"
	coin := int64(100)
	gem := int64(50)
	ad := int64(25)
	reason := "测试奖励"
	userID := "550e8400-e29b-41d4-a716-446655440000"
	recordID := "550e8400-e29b-41d4-a716-446655440001"

	// 在兼容模式下生成签名（不包含userID和recordID）
	signatureCompatible, err := GenerateWalletSignature(operationType, coin, gem, ad, reason)
	assert.NoError(t, err)
	assert.NotEmpty(t, signatureCompatible)

	// 在兼容模式下验证签名应该成功
	isValid := VerifyWalletSignature(operationType, coin, gem, ad, reason, signatureCompatible)
	assert.True(t, isValid, "兼容模式下的签名应该能验证成功")

	signatureStrict, err := GenerateWalletSignatureV2(operationType, coin, gem, ad, reason, userID, recordID)
	assert.NoError(t, err)
	assert.NotEmpty(t, signatureStrict)

	isValid = VerifyWalletSignatureV2(operationType, coin, gem, ad, reason, signatureStrict, userID, recordID)
	assert.True(t, isValid, "严格模式下的签名应该能验证成功")

	// 验证两种模式生成的签名应该不同
	assert.NotEqual(t, signatureCompatible, signatureStrict, "兼容模式和严格模式的签名应该不同")
}

func TestWalletSignatureForwardCompatibility(t *testing.T) {
	operationType := "gain"
	coin := int64(100)
	gem := int64(50)
	ad := int64(25)
	reason := "测试奖励"
	userID := "550e8400-e29b-41d4-a716-446655440000"
	recordID := "550e8400-e29b-41d4-a716-446655440001"

	// 生成新版本的签名（包含用户ID）
	newSignature, err := GenerateWalletSignatureV2(operationType, coin, gem, ad, reason, userID, recordID)
	assert.NoError(t, err)

	// 使用新版本的验证函数验证新版本签名
	isValid := VerifyWalletSignatureV2(operationType, coin, gem, ad, reason, newSignature, userID, recordID)
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

	recordID1 := "550e8400-e29b-41d4-a716-446655440002"
	recordID2 := "550e8400-e29b-41d4-a716-446655440003"

	// 为用户1生成包含用户ID的签名
	signature1, err := GenerateWalletSignatureV2(operationType, coin, gem, ad, reason, userID1, recordID1)
	assert.NoError(t, err)

	// 为用户2生成包含用户ID的签名
	signature2, err := GenerateWalletSignatureV2(operationType, coin, gem, ad, reason, userID2, recordID2)
	assert.NoError(t, err)

	// 验证各自的签名
	isValid1 := VerifyWalletSignatureV2(operationType, coin, gem, ad, reason, signature1, userID1, recordID1)
	isValid2 := VerifyWalletSignatureV2(operationType, coin, gem, ad, reason, signature2, userID2, recordID2)

	assert.True(t, isValid1)
	assert.True(t, isValid2)

	// 验证交叉验证应该失败
	isValidCross1 := VerifyWalletSignatureV2(operationType, coin, gem, ad, reason, signature1, userID2, recordID1)
	isValidCross2 := VerifyWalletSignatureV2(operationType, coin, gem, ad, reason, signature2, userID1, recordID2)

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
	recordID := "550e8400-e29b-41d4-a716-446655440001"

	// 生成不包含用户ID和记录ID的签名
	signatureWithoutUserIDAndRecordID, err := GenerateWalletSignatureV2(operationType, coin, gem, ad, reason, userID, recordID)
	assert.NoError(t, err)

	// 验证应该成功
	isValid := VerifyWalletSignatureV2(operationType, coin, gem, ad, reason, signatureWithoutUserIDAndRecordID, userID, recordID)
	assert.True(t, isValid)
}

func TestWalletSignatureWithIDFunction(t *testing.T) {
	// 测试GenerateWalletSignatureWithID函数
	operationType := "gain"
	coin := int64(100)
	gem := int64(50)
	ad := int64(25)
	reason := "测试奖励"
	userID := "550e8400-e29b-41d4-a716-446655440000"
	recordID := "550e8400-e29b-41d4-a716-446655440001" // 有效的UUID格式

	// 生成包含ID的签名
	signature, err := GenerateWalletSignatureV2(operationType, coin, gem, ad, reason, userID, recordID)
	assert.NoError(t, err)
	assert.NotEmpty(t, signature)

	// 使用VerifyWalletSignatureWithID验证
	isValid := VerifyWalletSignatureV2(operationType, coin, gem, ad, reason, signature, userID, recordID)
	assert.True(t, isValid)

	// 测试无效的UUID格式
	invalidRecordID := "invalid-uuid"
	_, err = GenerateWalletSignatureV2(operationType, coin, gem, ad, reason, userID, invalidRecordID)
	assert.Error(t, err, "应该因为无效的UUID格式而失败")

	// 测试空的recordID
	emptyRecordSignature, err := GenerateWalletSignatureV2(operationType, coin, gem, ad, reason, userID, "")
	assert.NoError(t, err)
	assert.NotEmpty(t, emptyRecordSignature)

	isValid = VerifyWalletSignatureV2(operationType, coin, gem, ad, reason, emptyRecordSignature, userID, "")
	assert.True(t, isValid)
}
