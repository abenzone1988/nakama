package server

import (
	"testing"
	"time"
)

// TestPayParseTime 客户端 Newtonsoft 序列化的时间格式必须都能解析
func TestPayParseTime(t *testing.T) {
	cases := []struct {
		name  string
		value interface{}
		want  time.Time
	}{
		{"rfc3339", "2026-11-11T08:30:00Z", time.Date(2026, 11, 11, 8, 30, 0, 0, time.UTC)},
		{"rfc3339 offset", "2026-11-11T16:30:00+08:00", time.Date(2026, 11, 11, 8, 30, 0, 0, time.UTC)},
		{"min value no zone", "0001-01-01T00:00:00", time.Time{}},
		{"min value with zone", "0001-01-01T00:00:00Z", time.Time{}},
		{"empty", "", time.Time{}},
		{"missing", nil, time.Time{}},
	}
	for _, c := range cases {
		got := parsePayTime(c.value)
		if !got.Equal(c.want) {
			t.Fatalf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

// TestPayFormatTime 空时间必须输出客户端可识别的 MinValue 表示
func TestPayFormatTime(t *testing.T) {
	if got := formatPayTime(time.Time{}); got != "0001-01-01T00:00:00Z" {
		t.Fatalf("zero time format: %s", got)
	}
	ts := time.Date(2026, 11, 11, 8, 30, 0, 0, time.UTC)
	if got := formatPayTime(ts); got != "2026-11-11T08:30:00Z" {
		t.Fatalf("utc time format: %s", got)
	}
	// 与 parsePayTime 必须可逆
	if got := parsePayTime(formatPayTime(ts)); !got.Equal(ts) {
		t.Fatalf("round trip: got %v want %v", got, ts)
	}
}

// TestExtendCardExpire 与客户端 ActivateCard 一致：按本地自然日对齐，到期为第 7 天本地 24 点
func TestExtendCardExpire(t *testing.T) {
	const tz = int32(8 * 60) // UTC+8

	// 2026-11-11 15:00 本地（07:00 UTC）首次购买 -> 2026-11-18 00:00 本地（2026-11-17 16:00 UTC）
	now := time.Date(2026, 11, 11, 7, 0, 0, 0, time.UTC)
	got := extendCardExpire(now, time.Time{}, tz)
	want := time.Date(2026, 11, 17, 16, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("fresh: got %v want %v", got, want)
	}

	// 未过期续买：在原到期日上再叠加 7 天 -> 2026-11-25 00:00 本地（2026-11-24 16:00 UTC）
	got = extendCardExpire(now, want, tz)
	wantRenew := time.Date(2026, 11, 24, 16, 0, 0, 0, time.UTC)
	if !got.Equal(wantRenew) {
		t.Fatalf("renew: got %v want %v", got, wantRenew)
	}

	// 已过期续买：从现在所在自然日重新计算
	expired := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	got = extendCardExpire(now, expired, tz)
	if !got.Equal(want) {
		t.Fatalf("expired: got %v want %v", got, want)
	}
}

// TestSameLocalDay 自然日必须按客户端时区判定，而不是 UTC
func TestSameLocalDay(t *testing.T) {
	const tz = int32(8 * 60)
	a := time.Date(2026, 11, 10, 15, 30, 0, 0, time.UTC) // 本地 23:30
	b := time.Date(2026, 11, 10, 16, 30, 0, 0, time.UTC) // 本地次日 00:30

	if sameLocalDay(a, b, tz) {
		t.Fatal("跨本地零点应判定为不同自然日")
	}
	if !sameLocalDay(a, b, 0) {
		t.Fatal("按 UTC 应判定为同一自然日")
	}
}

// TestCanClaimOnDay 同一自然日只能领一次，另加最小间隔
func TestCanClaimOnDay(t *testing.T) {
	const tz = int32(8 * 60)
	s := &ApiServer{}
	now := time.Date(2026, 11, 11, 7, 0, 0, 0, time.UTC) // 本地 15:00

	// 从未领取过
	if !s.canClaimOnDay(map[string]interface{}{}, now, tz) {
		t.Fatal("从未领取应可领取")
	}

	// 今天已领
	doc := map[string]interface{}{"lastClaimTime": formatPayTime(time.Date(2026, 11, 11, 2, 0, 0, 0, time.UTC))}
	if s.canClaimOnDay(doc, now, tz) {
		t.Fatal("同一自然日不应再次可领")
	}

	// 昨天领过
	doc["lastClaimTime"] = formatPayTime(time.Date(2026, 11, 10, 5, 0, 0, 0, time.UTC))
	if !s.canClaimOnDay(doc, now, tz) {
		t.Fatal("跨自然日应可再次领取")
	}

	// 跨日但间隔不足最小间隔
	doc["lastClaimTime"] = formatPayTime(now.Add(-30 * time.Second))
	if s.canClaimOnDay(doc, now, tz) {
		t.Fatal("间隔不足最小间隔不应可领")
	}
}

// TestResolveTzOffset 时区偏移一旦记录就固化，防止篡改伪造新的一天
func TestResolveTzOffset(t *testing.T) {
	s := &ApiServer{}

	if got := s.resolveTzOffset(map[string]interface{}{}, 480); got != 480 {
		t.Fatalf("首次应使用客户端偏移，got %d", got)
	}

	doc := map[string]interface{}{"tzOffset": float64(480)}
	if got := s.resolveTzOffset(doc, -600); got != 480 {
		t.Fatalf("应沿用已记录偏移，got %d", got)
	}

	if got := clampTzOffset(9999); got != 14*60 {
		t.Fatalf("偏移上限，got %d", got)
	}
	if got := clampTzOffset(-9999); got != -12*60 {
		t.Fatalf("偏移下限，got %d", got)
	}
}

// TestIntValue 存档里的数字经 JSON 反序列化后是 float64
func TestIntValue(t *testing.T) {
	cases := []struct {
		value interface{}
		want  int64
	}{
		{float64(3), 3},
		{int64(4), 4},
		{int(5), 5},
		{"6", 0},
		{nil, 0},
	}
	for _, c := range cases {
		if got := intValue(c.value); got != c.want {
			t.Fatalf("intValue(%v): got %d want %d", c.value, got, c.want)
		}
	}
}
