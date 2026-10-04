package schedule

import (
	"testing"
	"time"
)

func TestValidRepeat(t *testing.T) {
	for _, ok := range []string{"", "daily", "weekly", "monthly"} {
		if !ValidRepeat(ok) {
			t.Errorf("ValidRepeat(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"hourly", "yearly", "DAILY "} {
		if ValidRepeat(bad) {
			t.Errorf("ValidRepeat(%q) = true, want false", bad)
		}
	}
}

func TestNextRun(t *testing.T) {
	utc := time.UTC
	cases := []struct {
		name   string
		prev   time.Time
		repeat string
		now    time.Time
		want   time.Time
	}{
		{"none", time.Date(2024, 1, 1, 9, 0, 0, 0, utc), "", time.Date(2024, 1, 1, 10, 0, 0, 0, utc), time.Time{}},
		{"daily", time.Date(2024, 1, 1, 9, 0, 0, 0, utc), "daily", time.Date(2024, 1, 1, 10, 0, 0, 0, utc), time.Date(2024, 1, 2, 9, 0, 0, 0, utc)},
		{"daily skip", time.Date(2024, 1, 1, 9, 0, 0, 0, utc), "daily", time.Date(2024, 1, 5, 8, 0, 0, 0, utc), time.Date(2024, 1, 5, 9, 0, 0, 0, utc)},
		{"weekly", time.Date(2024, 1, 1, 9, 0, 0, 0, utc), "weekly", time.Date(2024, 1, 3, 9, 0, 0, 0, utc), time.Date(2024, 1, 8, 9, 0, 0, 0, utc)},
		{"monthly", time.Date(2024, 1, 15, 9, 0, 0, 0, utc), "monthly", time.Date(2024, 1, 20, 9, 0, 0, 0, utc), time.Date(2024, 2, 15, 9, 0, 0, 0, utc)},
	}
	for _, c := range cases {
		if got := NextRun(c.prev, c.repeat, c.now); !got.Equal(c.want) {
			t.Errorf("%s: NextRun = %v, want %v", c.name, got, c.want)
		}
	}
}
