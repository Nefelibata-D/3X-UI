package gateway

import (
	"errors"
	"time"
)

// Day 0 preserves the original first-of-month policy for existing records.
type ResetSchedule struct {
	Day      int  `json:"day"`
	Minute   int  `json:"minute"`
	Disabled bool `json:"disabled"`
}

func sameSchedule(a, b ResetSchedule, aZone, bZone string) bool {
	if a.Day == 0 {
		a.Day = 1
	}
	if b.Day == 0 {
		b.Day = 1
	}
	// Older ledgers used the policy timezone and did not store it separately.
	return a == b && (aZone == "" || aZone == bZone)
}

func (s ResetSchedule) Validate() error {
	if s.Day < 0 || s.Day > 31 || s.Minute < 0 || s.Minute >= 1440 {
		return errors.New("重置日须为 1–31，重置时间须为 00:00–23:59")
	}
	return nil
}

// A billing day absent from a month falls on that month's last day.
func (s ResetSchedule) Bounds(at time.Time, loc *time.Location) (int64, int64) {
	if s.Disabled {
		return at.Unix(), 0
	}
	t := at.In(loc)
	boundary := func(month time.Time) time.Time {
		day := s.Day
		if day == 0 {
			day = 1
		}
		last := time.Date(month.Year(), month.Month()+1, 0, 0, 0, 0, 0, loc).Day()
		if day > last {
			day = last
		}
		return time.Date(month.Year(), month.Month(), day, s.Minute/60, s.Minute%60, 0, 0, loc)
	}
	month := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc)
	start := boundary(month)
	if at.Before(start) {
		month = month.AddDate(0, -1, 0)
		start = boundary(month)
	}
	return start.Unix(), boundary(month.AddDate(0, 1, 0)).Unix()
}
