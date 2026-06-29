package kcontext_test

import (
	"strings"
	"testing"
	"time"

	"github.com/gqlo/kcontext"
)

func TestFormatRelativeTime(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{"zero", time.Time{}, "—"},
		{"30 seconds", now.Add(-30 * time.Second), "second"},
		{"1 second", now.Add(-1 * time.Second), "1 second ago"},
		{"5 minutes", now.Add(-5 * time.Minute), "minute"},
		{"2 hours", now.Add(-2 * time.Hour), "hour"},
		{"3 days", now.Add(-3 * 24 * time.Hour), "day"},
		{"2 weeks", now.Add(-14 * 24 * time.Hour), "week"},
		{"3 months", now.Add(-90 * 24 * time.Hour), "month"},
		{"2 years", now.Add(-730 * 24 * time.Hour), "year"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := kcontext.FormatRelativeTime(tt.at)
			if tt.want == "—" {
				if got != "—" {
					t.Fatalf("got %q", got)
				}
				return
			}
			if !strings.Contains(got, tt.want) || !strings.Contains(got, "ago") {
				t.Fatalf("got %q, want substring %q and 'ago'", got, tt.want)
			}
		})
	}
}

func TestFormatAlertTime(t *testing.T) {
	if got := kcontext.FormatAlertTime(time.Time{}); got != "—" {
		t.Fatalf("zero time = %q", got)
	}
	ts := time.Date(2026, 6, 29, 15, 4, 5, 0, time.UTC)
	if got := kcontext.FormatAlertTime(ts); got != "2026-06-29 15:04:05 UTC" {
		t.Fatalf("got %q", got)
	}
}
