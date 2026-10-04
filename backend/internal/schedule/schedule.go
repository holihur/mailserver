// Package schedule 提供定时 / 周期性发送的重复规则计算。
package schedule

import "time"

// 支持的重复周期。
const (
	RepeatNone    = ""
	RepeatDaily   = "daily"
	RepeatWeekly  = "weekly"
	RepeatMonthly = "monthly"
)

// ValidRepeat 判断重复周期是否合法。
func ValidRepeat(s string) bool {
	switch s {
	case RepeatNone, RepeatDaily, RepeatWeekly, RepeatMonthly:
		return true
	}
	return false
}

// NextRun 在 prev 基础上按 repeat 递推，返回第一个晚于 now 的时间。
// repeat 非法或为空时返回零值（表示不再重复）。
func NextRun(prev time.Time, repeat string, now time.Time) time.Time {
	if repeat == RepeatNone {
		return time.Time{}
	}
	t := prev
	// 防止服务长期停机后大量循环。
	for i := 0; i < 100000 && !t.After(now); i++ {
		switch repeat {
		case RepeatDaily:
			t = t.AddDate(0, 0, 1)
		case RepeatWeekly:
			t = t.AddDate(0, 0, 7)
		case RepeatMonthly:
			t = t.AddDate(0, 1, 0)
		default:
			return time.Time{}
		}
	}
	if !t.After(now) {
		return now.Add(time.Hour) // 兜底：极端情况推迟一小时
	}
	return t
}
