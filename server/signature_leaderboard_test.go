package server

import (
	"fmt"
	"testing"
)

func TestGenerateAndVerifyLeaderboardSignature(t *testing.T) {
	// 测试数据
	leaderboardID := "test-leaderboard"
	score := int64(1000)
	subscore := int64(500)
	metadata := `{"level": 1, "character": "player1"}`

	// 生成签名
	signature, err := GenerateLeaderboardRecordSignature(leaderboardID, score, subscore, metadata)
	if err != nil {
		t.Fatalf("Failed to generate signature: %v", err)
	}

	// 验证签名
	isValid := VerifyLeaderboardRecordSignature(leaderboardID, score, subscore, metadata, signature)
	if !isValid {
		t.Errorf("Signature verification failed for valid signature")
	}

	// 测试无效签名
	invalidSignature := "invalid-signature"
	isValid = VerifyLeaderboardRecordSignature(leaderboardID, score, subscore, metadata, invalidSignature)
	if isValid {
		t.Errorf("Signature verification should fail for invalid signature")
	}

	// 测试篡改数据
	tamperedScore := int64(9999)
	isValid = VerifyLeaderboardRecordSignature(leaderboardID, tamperedScore, subscore, metadata, signature)
	if isValid {
		t.Errorf("Signature verification should fail for tampered data")
	}
}

func TestGenerateAndVerifyTournamentSignature(t *testing.T) {
	// 测试数据
	tournamentID := "test-tournament"
	score := int64(2000)
	subscore := int64(1000)
	metadata := `{"round": 3, "player": "tournament_player"}`

	// 生成签名
	signature, err := GenerateTournamentRecordSignature(tournamentID, score, subscore, metadata)
	if err != nil {
		t.Fatalf("Failed to generate signature: %v", err)
	}

	// 验证签名
	isValid := VerifyTournamentRecordSignature(tournamentID, score, subscore, metadata, signature)
	if !isValid {
		t.Errorf("Signature verification failed for valid signature")
	}

	// 测试无效签名
	invalidSignature := "invalid-signature"
	isValid = VerifyTournamentRecordSignature(tournamentID, score, subscore, metadata, invalidSignature)
	if isValid {
		t.Errorf("Signature verification should fail for invalid signature")
	}

	// 测试篡改数据
	tamperedTournamentID := "tampered-tournament"
	isValid = VerifyTournamentRecordSignature(tamperedTournamentID, score, subscore, metadata, signature)
	if isValid {
		t.Errorf("Signature verification should fail for tampered data")
	}
}

func TestEmptyMetadata(t *testing.T) {
	// 测试空 metadata
	leaderboardID := "test-leaderboard"
	score := int64(1000)
	subscore := int64(500)
	metadata := ""

	// 生成签名
	signature, err := GenerateLeaderboardRecordSignature(leaderboardID, score, subscore, metadata)
	if err != nil {
		t.Fatalf("Failed to generate signature for empty metadata: %v", err)
	}

	// 验证签名
	isValid := VerifyLeaderboardRecordSignature(leaderboardID, score, subscore, metadata, signature)
	if !isValid {
		t.Errorf("Signature verification failed for empty metadata")
	}
}

func TestSignatureConsistency(t *testing.T) {
	// 测试相同数据生成的签名是否一致
	leaderboardID := "test-leaderboard"
	score := int64(1000)
	subscore := int64(500)
	metadata := `{"level": 1}`

	signature1, err := GenerateLeaderboardRecordSignature(leaderboardID, score, subscore, metadata)
	if err != nil {
		t.Fatalf("Failed to generate first signature: %v", err)
	}

	signature2, err := GenerateLeaderboardRecordSignature(leaderboardID, score, subscore, metadata)
	if err != nil {
		t.Fatalf("Failed to generate second signature: %v", err)
	}

	if signature1 != signature2 {
		t.Errorf("Signatures should be consistent for same data. Got %s and %s", signature1, signature2)
	}
}

func TestSignatureWithSpecialCharacters(t *testing.T) {
	// 测试包含特殊字符的数据
	leaderboardID := "test-leaderboard"
	score := int64(1000)
	subscore := int64(500)
	metadata := `{"message": "Hello, 世界! @#$%^&*()[]{}|\\:;\"'<>,.?/~"}`

	signature, err := GenerateLeaderboardRecordSignature(leaderboardID, score, subscore, metadata)
	if err != nil {
		t.Fatalf("Failed to generate signature with special characters: %v", err)
	}

	isValid := VerifyLeaderboardRecordSignature(leaderboardID, score, subscore, metadata, signature)
	if !isValid {
		t.Errorf("Signature verification failed for data with special characters")
	}
}

func TestTournamentSignature(t *testing.T) {
	// 测试数据
	tournamentID := "challenge_12_1752476400_001"
	score := int64(100000000)
	subscore := int64(200)
	metadata := `{}`

	// 生成签名
	signature, err := GenerateTournamentRecordSignature(tournamentID, score, subscore, metadata)
	if err != nil {
		t.Fatalf("Failed to generate signature: %v", err)
	}
	t.Logf("{\\\"signature\\\": \\\"%s\\\"}", signature)

	// 验证签名
	isValid := VerifyTournamentRecordSignature(tournamentID, score, subscore, metadata, signature)
	if !isValid {
		t.Errorf("Signature verification failed for valid signature")
	}

	// 测试无效签名
	invalidSignature := "invalid-signature"
	isValid = VerifyTournamentRecordSignature(tournamentID, score, subscore, metadata, invalidSignature)
	if isValid {
		t.Errorf("Signature verification should fail for invalid signature")
	}

	// 测试篡改数据
	tamperedTournamentID := "tampered-tournament"
	isValid = VerifyTournamentRecordSignature(tamperedTournamentID, score, subscore, metadata, signature)
	if isValid {
		t.Errorf("Signature verification should fail for tampered data")
	}
}

func TestGenerateTournamentSignatureForWriteRecord(t *testing.T) {
	// 用户提供的数据
	tournamentID := "challenge_12_1752476400_001"
	score := int64(100000000)
	subscore := int64(2000)
	metadata := "{}"

	// 生成签名
	signature, err := GenerateTournamentRecordSignature(tournamentID, score, subscore, metadata)
	if err != nil {
		t.Fatalf("Failed to generate signature: %v", err)
	}

	// 打印签名好的metadata
	signedMetadata := fmt.Sprintf(`{"signature": "%s"}`, signature)
	t.Logf("Signed metadata: %s", signedMetadata)

	// 验证签名
	isValid := VerifyTournamentRecordSignature(tournamentID, score, subscore, metadata, signature)
	if !isValid {
		t.Errorf("Signature verification failed for generated signature")
	}

	// 打印完整的请求数据
	t.Logf("Complete request data:")
	t.Logf(`{
  "tournamentId": "%s",
  "record": {
    "score": "%d",
    "subscore": "%d",
    "metadata": "%s",
    "operator": 0
  }
}`, tournamentID, score, subscore, signedMetadata)
}
