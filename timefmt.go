package kcontext

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func FormatAlertTime(t time.Time) string {
	if t.IsZero() || t.Year() < 1970 {
		return "—"
	}
	return t.UTC().Format("2006-01-02 15:04:05 UTC")
}

// FormatTimeISO returns an RFC3339 UTC string for HTML datetime attributes.
func FormatTimeISO(t time.Time) string {
	if t.IsZero() || t.Year() < 1970 {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// FormatRelativeTime renders a human-readable "time ago" string from ReceivedAt to now.
func FormatRelativeTime(t time.Time) string {
	if t.IsZero() || t.Year() < 1970 {
		return "—"
	}

	d := time.Now().UTC().Sub(t.UTC())
	if d < 0 {
		d = 0
	}

	switch {
	case d < time.Minute:
		return pluralAgo(int(d.Seconds()), "second")
	case d < 2*time.Hour:
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

// ParseFlexibleTime parses timestamps from Redis JSON (RFC3339 or legacy layouts without timezone).
func ParseFlexibleTime(raw json.RawMessage) time.Time {
	if len(raw) == 0 {
		return time.Time{}
	}

	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return time.Time{}
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}

	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		t, err := time.ParseInLocation(layout, s, time.UTC)
		if err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func normalizeReceivedAt(stored StoredAlert, rawJSON []byte) StoredAlert {
	if !stored.ReceivedAt.IsZero() && stored.ReceivedAt.Year() >= 1970 {
		stored.ReceivedAt = stored.ReceivedAt.UTC()
		return stored
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(rawJSON, &fields); err != nil {
		return stored
	}
	if raw, ok := fields["received_at"]; ok {
		if t := ParseFlexibleTime(raw); !t.IsZero() {
			stored.ReceivedAt = t
		}
	}
	return stored
}

// RelativeTimeRefreshJS updates <time class="relative-time"> labels in the browser.
const RelativeTimeRefreshJS = `
(function () {
  function pluralAgo(n, unit) {
    if (n < 1) n = 1;
    return n === 1 ? '1 ' + unit + ' ago' : n + ' ' + unit + 's ago';
  }
  function formatRelative(ms) {
    if (ms < 0) ms = 0;
    var sec = Math.floor(ms / 1000);
    var min = Math.floor(ms / 60000);
    var hr = Math.floor(ms / 3600000);
    var day = Math.floor(ms / 86400000);
    if (sec < 60) return pluralAgo(sec, 'second');
    if (min < 120) return pluralAgo(min, 'minute');
    if (hr < 24) return pluralAgo(hr, 'hour');
    if (day < 7) return pluralAgo(day, 'day');
    if (day < 30) return pluralAgo(Math.floor(day / 7), 'week');
    if (day < 365) return pluralAgo(Math.floor(day / 30), 'month');
    return pluralAgo(Math.floor(day / 365), 'year');
  }
  function refreshRelativeTimes() {
    document.querySelectorAll('time.relative-time[datetime]').forEach(function (el) {
      var t = new Date(el.getAttribute('datetime'));
      if (isNaN(t.getTime())) return;
      el.textContent = formatRelative(Date.now() - t.getTime());
    });
  }
  refreshRelativeTimes();
  setInterval(refreshRelativeTimes, 30000);
})();
`
