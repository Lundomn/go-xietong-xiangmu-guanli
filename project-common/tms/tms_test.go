package tms

import (
	"testing"
	"time"
)

func TestParseTimeSupportsCommonTimestampFormats(t *testing.T) {
	for _, value := range []string{
		"2026-09-15 09:00",
		"2026-09-15 09:00:00",
		"2026-09-15T09:00:00+08:00",
	} {
		if got := ParseTime(value); got <= 0 {
			t.Fatalf("ParseTime(%q) = %d, want a positive Unix timestamp", value, got)
		}
	}
}

func TestParseTimeRejectsInvalidInput(t *testing.T) {
	if got := ParseTime("not-a-time"); got != 0 {
		t.Fatalf("ParseTime(invalid) = %d, want 0", got)
	}
}

func TestFormatUsesShanghaiTimezone(t *testing.T) {
	value := time.Date(2026, 9, 15, 1, 0, 0, 0, time.UTC)
	if got := Format(value); got != "2026-09-15 09:00:00" {
		t.Fatalf("Format() = %q, want Shanghai local time", got)
	}
}
