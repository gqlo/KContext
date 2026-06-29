package kcontext

import (
	"fmt"
	"time"
)

func FormatAlertTime(t time.Time) string {
	if t.IsZero() || t.Year() < 1970 {
		return "—"
	}
	return t.UTC().Format("2006-01-02 15:04:05 UTC")
}

func FormatRelativeTime(t time.Time) string {
	if t.IsZero() || t.Year() < 1970 {
		return "—"
	}

	d := time.Since(t)
	if d < 0 {
		d = 0
	}

	switch {
	case d < time.Minute:
		return pluralAgo(int(d.Seconds()), "second")
	case d < time.Hour:
		return pluralAgo(int(d.Minutes()), "minute")
	case d < 24*time.Hour:
		return pluralAgo(int(d.Hours()), "hour")
	case d < 7*24*time.Hour:
		return pluralAgo(int(d.Hours()/24), "day")
	case d < 30*24*time.Hour:
		return pluralAgo(int(d.Hours()/24/7), "week")
	case d < 365*24*time.Hour:
		return pluralAgo(int(d.Hours()/24/30), "month")
	default:
		return pluralAgo(int(d.Hours()/24/365), "year")
	}
}

func pluralAgo(n int, unit string) string {
	if n < 1 {
		n = 1
	}
	if n == 1 {
		return fmt.Sprintf("1 %s ago", unit)
	}
	return fmt.Sprintf("%d %ss ago", n, unit)
}
