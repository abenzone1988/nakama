package server

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/gofrs/uuid/v5"
	"go.uber.org/zap"
)

type TimeZoneProvider func(ctx context.Context) *time.Location

var PlayerTimeZoneProvider TimeZoneProvider = func(ctx context.Context) *time.Location {
	if loc, ok := ctx.Value(ctxPlayerTimeZoneKey{}).(*time.Location); ok && loc != nil {
		return loc
	}
	return time.Local
}

var GetGameTimeZone = PlayerTimeZoneProvider

type ctxPlayerTimeZoneKey struct{}

// GetGameTimeNow 获取游戏时区的当前时间
func GetGameTimeNow(ctx context.Context) time.Time {
	loc := GetGameTimeZone(ctx)
	return time.Now().In(loc)
}

// GetNextDailyTime 获取下一个每日固定时间点
func GetNextDailyTime(ctx context.Context, hours []int) time.Time {
	now := GetGameTimeNow(ctx)
	loc := GetGameTimeZone(ctx)
	var nextTime time.Time

	for _, hour := range hours {
		checkTime := time.Date(now.Year(), now.Month(), now.Day(),
			hour, 0, 0, 0, loc)

		if checkTime.After(now) {
			if nextTime.IsZero() || checkTime.Before(nextTime) {
				nextTime = checkTime
			}
		}
	}

	if nextTime.IsZero() && len(hours) > 0 {
		tomorrow := now.AddDate(0, 0, 1)
		nextTime = time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(),
			hours[0], 0, 0, 0, loc)
	}

	return nextTime
}

// CountPassedDailyTimes 统计从某个时间到现在经过了多少个固定时间点
func CountPassedDailyTimes(ctx context.Context, lastTime time.Time, hours []int) (int32, time.Time) {
	now := GetGameTimeNow(ctx)
	loc := GetGameTimeZone(ctx)

	lastTimeInZone := lastTime.In(loc)
	count := int32(0)
	latestPassedTime := lastTimeInZone

	for _, hour := range hours {
		checkTime := time.Date(lastTimeInZone.Year(), lastTimeInZone.Month(), lastTimeInZone.Day(),
			hour, 0, 0, 0, loc)

		if checkTime.Before(lastTimeInZone) || checkTime.Equal(lastTimeInZone) {
			checkTime = checkTime.AddDate(0, 0, 1)
		}

		for checkTime.Before(now) || checkTime.Equal(now) {
			count++
			latestPassedTime = checkTime
			checkTime = checkTime.AddDate(0, 0, 1)
		}
	}

	return count, latestPassedTime
}

// ParseTimeString 解析 RFC3339 格式的时间字符串
func ParseTimeString(timeStr string) (time.Time, error) {
	if timeStr == "" {
		return time.Time{}, fmt.Errorf("empty time string")
	}
	return time.Parse(time.RFC3339, timeStr)
}

// FormatTimeToRFC3339 将时间格式化为 RFC3339 字符串（UTC）
func FormatTimeToRFC3339(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// LoadPlayerTimeZone 从用户账号加载玩家时区并存入 context
func LoadPlayerTimeZone(ctx context.Context, logger *zap.Logger, db *sql.DB, statusRegistry StatusRegistry) context.Context {
	userID, ok := ctx.Value(ctxUserIDKey{}).(uuid.UUID)
	if !ok {
		return ctx
	}

	account, err := GetAccount(ctx, logger, db, statusRegistry, userID)
	if err != nil {
		logger.Debug("无法获取用户账号信息，使用服务器时区", zap.Error(err))
		return ctx
	}

	timezone := ""
	if account.User != nil {
		timezone = account.User.Timezone
	}

	if timezone == "" {
		return context.WithValue(ctx, ctxPlayerTimeZoneKey{}, time.Local)
	}

	loc, err := time.LoadLocation(timezone)
	if err != nil {
		logger.Warn("无法解析玩家时区，使用服务器时区",
			zap.String("timezone", timezone),
			zap.Error(err))
		return context.WithValue(ctx, ctxPlayerTimeZoneKey{}, time.Local)
	}

	return context.WithValue(ctx, ctxPlayerTimeZoneKey{}, loc)
}

// GetCurrentTimeUTC 获取当前 UTC 时间字符串
func GetCurrentTimeUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}
