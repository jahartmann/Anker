package storage

import (
	"testing"
	"time"
)

func TestForecastDoesNotInventDatesWithoutAUsableTrend(t *testing.T) {
	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	volume := Volume{Total: 100 << 30, Available: 40 << 30}
	cases := []struct {
		name    string
		samples []Sample
		want    string
	}{
		{"empty", nil, "learning"},
		{"one day", trend(at, 24, 100<<30, 1<<30), "learning"},
		{"stable", trend(at, 100, 100<<30, 0), "stable"},
		{"falling", trend(at, 100, 100<<30, -1<<30), "stable"},
		{"old", trend(at.Add(-72*time.Hour), 100, 100<<30, 1<<30), "stale"},
		{"new capacity", trend(at, 100, 50<<30, 1<<30), "learning"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := ForecastFor(volume, c.samples, at)
			if f.Status != c.want || f.FullAt != "" {
				t.Fatalf("unexpected forecast: %+v", f)
			}
		})
	}
}

func trend(at time.Time, count int, total, growth int64) []Sample {
	out := []Sample{}
	for i := count - 1; i >= 0; i-- {
		out = append(out, Sample{At: at.Add(-time.Duration(i) * time.Hour).Format(time.RFC3339), Total: total, Used: (60 << 30) - int64(i)*growth/24, Available: (40 << 30) + int64(i)*growth/24})
	}
	return out
}

func TestForecastUsesRealNetConsumptionAndResetsAfterExpansion(t *testing.T) {
	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	v := Volume{Total: 100 << 30, Available: 40 << 30}
	samples := trend(at, 120, 100<<30, 1<<30)
	f := ForecastFor(v, samples, at)
	if f.Status != "growing" || f.DaysToFull == nil || *f.DaysToFull < 39 || *f.DaysToFull > 41 || f.GrowthPerDay < 900<<20 || f.GrowthPerDay > 1100<<20 {
		t.Fatalf("1 GiB/day should exhaust 40 GiB in about 40 days: %+v", f)
	}
	samples = append(samples, Sample{At: at.Format(time.RFC3339), Total: 200 << 30, Used: 60 << 30, Available: 140 << 30})
	v.Total = 200 << 30
	v.Available = 140 << 30
	if f = ForecastFor(v, samples, at); f.Status != "learning" || f.FullAt != "" {
		t.Fatalf("capacity change retained old trend: %+v", f)
	}
}

func TestForecastRefusesErraticUsageAndFutureSamples(t *testing.T) {
	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	v := Volume{Total: 100 << 30, Available: 40 << 30}
	samples := trend(at, 120, 100<<30, 0)
	for i := range samples {
		day := i / 24
		values := []int64{20, 75, 25, 80, 30}
		samples[i].Used = values[day] << 30
		samples[i].Available = v.Total - samples[i].Used
	}
	f := ForecastFor(v, samples, at)
	if f.FullAt != "" {
		t.Fatalf("unstable cleanup cycles must not promise a date: %+v", f)
	}
	if f = ForecastFor(v, trend(at.Add(120*time.Hour), 100, 100<<30, 1<<30), at); f.Status != "learning" {
		t.Fatalf("future samples used: %+v", f)
	}
}
