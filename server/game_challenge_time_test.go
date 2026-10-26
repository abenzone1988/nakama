package server

import (
	"testing"
	"time"

	"github.com/heroiclabs/nakama/v3/template"
)

func mustLocal(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation("2006-1-2 15:04", value, time.Local)
	if err != nil {
		t.Fatalf("解析测试时间失败 %q: %v", value, err)
	}
	return parsed
}

func TestParseDateTimeWithAndWithoutYear(t *testing.T) {
	withYear, err := parseDateTime("2026-9-27 22:15")
	if err != nil {
		t.Fatalf("解析带年份时间失败: %v", err)
	}
	if withYear.Year() != 2026 || withYear.Month() != time.September || withYear.Day() != 27 {
		t.Fatalf("带年份时间解析错误: %v", withYear)
	}

	withoutYear, err := parseDateTime("9-27 22:15")
	if err != nil {
		t.Fatalf("解析不带年份时间失败: %v", err)
	}
	if withoutYear.Year() != 0 || withoutYear.Month() != time.September || withoutYear.Day() != 27 {
		t.Fatalf("不带年份时间解析错误: %v", withoutYear)
	}
}

func TestParseChallengeTimesIgnoreTemplateYear(t *testing.T) {
	cases := []struct {
		name      string
		tpl       template.TplChallenge
		now       time.Time
		wantOpen  time.Time
		wantClose time.Time
		wantEnd   time.Time
	}{
		{
			name: "同年赛程",
			tpl: template.TplChallenge{
				OpenTime:  "9-26 10:00",
				CloseTime: "9-27 22:00",
				EndTime:   "9-27 22:15",
			},
			now:       mustLocal(t, "2027-9-26 12:00"),
			wantOpen:  mustLocal(t, "2027-9-26 10:00"),
			wantClose: mustLocal(t, "2027-9-27 22:00"),
			wantEnd:   mustLocal(t, "2027-9-27 22:15"),
		},
		{
			name: "模板带旧年份也按当前年循环",
			tpl: template.TplChallenge{
				OpenTime:  "2026-9-26 10:00",
				CloseTime: "2026-9-27 22:00",
				EndTime:   "2026-9-27 22:15",
			},
			now:       mustLocal(t, "2027-9-26 12:00"),
			wantOpen:  mustLocal(t, "2027-9-26 10:00"),
			wantClose: mustLocal(t, "2027-9-27 22:00"),
			wantEnd:   mustLocal(t, "2027-9-27 22:15"),
		},
		{
			name: "本轮已结束则顺延到下一年",
			tpl: template.TplChallenge{
				OpenTime:  "9-26 10:00",
				CloseTime: "9-27 22:00",
				EndTime:   "9-27 22:15",
			},
			now:       mustLocal(t, "2027-9-28 00:00"),
			wantOpen:  mustLocal(t, "2028-9-26 10:00"),
			wantClose: mustLocal(t, "2028-9-27 22:00"),
			wantEnd:   mustLocal(t, "2028-9-27 22:15"),
		},
		{
			name: "跨年赛程",
			tpl: template.TplChallenge{
				OpenTime:  "12-31 10:00",
				CloseTime: "1-1 22:00",
				EndTime:   "1-1 22:15",
			},
			now:       mustLocal(t, "2027-12-31 12:00"),
			wantOpen:  mustLocal(t, "2027-12-31 10:00"),
			wantClose: mustLocal(t, "2028-1-1 22:00"),
			wantEnd:   mustLocal(t, "2028-1-1 22:15"),
		},
		{
			name: "跨年赛程在次年1月1日仍进行中",
			tpl: template.TplChallenge{
				OpenTime:  "12-31 10:00",
				CloseTime: "1-1 22:00",
				EndTime:   "1-1 22:15",
			},
			now:       mustLocal(t, "2028-1-1 12:00"),
			wantOpen:  mustLocal(t, "2027-12-31 10:00"),
			wantClose: mustLocal(t, "2028-1-1 22:00"),
			wantEnd:   mustLocal(t, "2028-1-1 22:15"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			window, err := parseChallengeTimes(tc.tpl, tc.now)
			if err != nil {
				t.Fatalf("解析挑战赛时间失败: %v", err)
			}
			if !window.Open.Equal(tc.wantOpen) {
				t.Errorf("Open = %v, want %v", window.Open, tc.wantOpen)
			}
			if !window.Close.Equal(tc.wantClose) {
				t.Errorf("Close = %v, want %v", window.Close, tc.wantClose)
			}
			if !window.End.Equal(tc.wantEnd) {
				t.Errorf("End = %v, want %v", window.End, tc.wantEnd)
			}
		})
	}
}
