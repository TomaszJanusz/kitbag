package timefmt

import (
	"testing"
	"time"
	_ "time/tzdata" // zones must resolve on hosts without a zoneinfo database.
)

func zone(t *testing.T, name string) *time.Location {
	t.Helper()
	location, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return location
}

func TestStampRendersInTheClockZone(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 26, 14, 47, 51, 123456789, time.UTC)
	tests := []struct {
		zone *time.Location
		name string
		want string
	}{
		{nil, "nil location is UTC", "2026-10-26 14:47:51 UTC"},
		{time.UTC, "UTC", "2026-10-26 14:47:51 UTC"},
		{zone(t, "Europe/Warsaw"), "Warsaw winter time", "2026-10-26 15:47:51 CET"},
		{zone(t, "Asia/Kathmandu"), "quarter-hour zone", "2026-10-26 20:32:51 +0545"},
		{zone(t, "Pacific/Auckland"), "date rolls over", "2026-10-27 03:47:51 NZDT"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := (Clock{Location: tc.zone}).Stamp(at); got != tc.want {
				t.Fatalf("Stamp=%q want %q", got, tc.want)
			}
		})
	}
}

func TestStampAcrossDaylightSavingChange(t *testing.T) {
	t.Parallel()
	warsaw := Clock{Location: zone(t, "Europe/Warsaw")}
	// Europe/Warsaw falls back on 2026-10-25 at 01:00 UTC: 02:30 happens twice.
	first := time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC)
	second := time.Date(2026, 10, 25, 1, 30, 0, 0, time.UTC)
	if got, want := warsaw.Stamp(first), "2026-10-25 02:30:00 CEST"; got != want {
		t.Fatalf("before change=%q want %q", got, want)
	}
	if got, want := warsaw.Stamp(second), "2026-10-25 02:30:00 CET"; got != want {
		t.Fatalf("after change=%q want %q", got, want)
	}
	spring := time.Date(2026, 3, 29, 1, 0, 0, 0, time.UTC)
	if got, want := warsaw.Stamp(spring), "2026-03-29 03:00:00 CEST"; got != want {
		t.Fatalf("spring forward=%q want %q", got, want)
	}
	// The input instant is untouched: only the rendering depends on the zone.
	if !first.Equal(time.Date(2026, 10, 25, 2, 30, 0, 0, time.FixedZone("CEST", 7200))) {
		t.Fatal("fixture drifted")
	}
}

func TestTimeOfDay(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 26, 23, 30, 5, 0, time.UTC)
	if got, want := (Clock{Location: zone(t, "Europe/Warsaw")}).TimeOfDay(at), "00:30:05"; got != want {
		t.Fatalf("TimeOfDay=%q want %q", got, want)
	}
}

func TestAgoAndDescribe(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 26, 14, 50, 0, 0, time.UTC)
	clock := Clock{Location: zone(t, "Europe/Warsaw"), Now: func() time.Time { return now }}
	tests := []struct {
		want string
		age  time.Duration
	}{
		{"just now", -time.Hour},
		{"just now", 30 * time.Second},
		{"1 min ago", time.Minute},
		{"3 min ago", 3*time.Minute + 9*time.Second},
		{"59 min ago", 59*time.Minute + 59*time.Second},
		{"1 h ago", time.Hour},
		{"23 h ago", 23*time.Hour + 59*time.Minute},
		{"1 d ago", 24 * time.Hour},
		{"2 d ago", 49 * time.Hour},
	}
	for _, tc := range tests {
		if got := clock.Ago(now.Add(-tc.age)); got != tc.want {
			t.Errorf("Ago(%v)=%q want %q", tc.age, got, tc.want)
		}
	}
	at := now.Add(-3 * time.Minute)
	if got, want := clock.Describe(at), "2026-10-26 15:47:00 CET (3 min ago)"; got != want {
		t.Fatalf("Describe=%q want %q", got, want)
	}
}

func TestZeroClockHasNoRelativeHint(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 26, 14, 47, 51, 0, time.UTC)
	if got := (Clock{}).Ago(at); got != "" {
		t.Fatalf("Ago=%q want empty", got)
	}
	if got, want := (Clock{}).Describe(at), "2026-10-26 14:47:51 UTC"; got != want {
		t.Fatalf("Describe=%q want %q", got, want)
	}
	if _, _, ok := (Clock{}).Since(at); ok {
		t.Fatal("Since reported ok without Now")
	}
}
