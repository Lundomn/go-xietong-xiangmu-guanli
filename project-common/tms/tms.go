package tms

import (
	"time"
	_ "time/tzdata"
)

var location = func() *time.Location {
	loaded, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*60*60)
	}
	return loaded
}()

func Format(t time.Time) string {
	return t.In(location).Format("2006-01-02 15:04:05")
}
func FormatYMD(t time.Time) string {
	return t.In(location).Format("2006-01-02")
}
func FormatByMill(t int64) string {
	return time.UnixMilli(t).In(location).Format("2006-01-02 15:04:05")
}

func ParseTime(str string) int64 {
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04", time.RFC3339} {
		parsed, err := time.ParseInLocation(layout, str, location)
		if err == nil {
			return parsed.UnixMilli()
		}
	}
	return 0
}
