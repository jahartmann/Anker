package storage

import (
	"math"
	"sort"
	"time"
)

func median(values []float64) float64 {
	sort.Float64s(values)
	n := len(values)
	if n%2 == 1 {
		return values[n/2]
	}
	return (values[n/2-1] + values[n/2]) / 2
}
func ForecastFor(v Volume, samples []Sample, at time.Time) Forecast {
	f := Forecast{Status: "learning", Message: "Messreihe wird aufgebaut · mindestens drei Tage und 24 Messungen nötig."}
	if v.Error != "" || v.Total <= 0 {
		f.Status = "unavailable"
		f.Message = "Keine verlässlichen Dateisystemwerte verfügbar."
		return f
	}
	days := map[string][]float64{}
	dayTimes := map[string][]float64{}
	count := 0
	var first, last time.Time
	// Discard all earlier points when capacity changes, even if it later changes back.
	start := 0
	for i, s := range samples {
		if s.Total != v.Total {
			start = i + 1
		}
	}
	for _, s := range samples[start:] {
		t, err := time.Parse(time.RFC3339, s.At)
		if err != nil || t.After(at) || at.Sub(t) > 14*24*time.Hour || s.Used < 0 || s.Available < 0 || s.Used > v.Total || s.Available > v.Total {
			continue
		}
		if first.IsZero() || t.Before(first) {
			first = t
		}
		if t.After(last) {
			last = t
		}
		count++
		days[t.UTC().Format("2006-01-02")] = append(days[t.UTC().Format("2006-01-02")], float64(v.Total-s.Available))
		dayTimes[t.UTC().Format("2006-01-02")] = append(dayTimes[t.UTC().Format("2006-01-02")], float64(t.Unix())/86400)
	}
	if !last.IsZero() && at.Sub(last) > 6*time.Hour {
		f.Status = "stale"
		f.Message = "Die Messreihe ist veraltet. Nach neuen Messungen wird die Prognose erneut berechnet."
		return f
	}
	if count < 24 || len(days) < 4 || last.Sub(first) < 72*time.Hour {
		return f
	}
	keys := []string{}
	for key := range days {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	x, y := []float64{}, []float64{}
	origin := median(dayTimes[keys[0]])
	for _, key := range keys {
		x = append(x, median(dayTimes[key])-origin)
		y = append(y, median(days[key]))
	}
	f.BasedOnDays = int(last.Sub(first).Hours() / 24)
	n := float64(len(x))
	var sx, sy, sxx, sxy float64
	for i := range x {
		sx += x[i]
		sy += y[i]
		sxx += x[i] * x[i]
		sxy += x[i] * y[i]
	}
	slope := (n*sxy - sx*sy) / (n*sxx - sx*sx)
	threshold := math.Max(1<<20, float64(v.Total)*0.000001)
	if slope <= threshold {
		f.Status = "stable"
		f.Message = "Aktuell kein anhaltender Nettozuwachs. Daraus lässt sich kein Voll-Datum ableiten."
		return f
	}
	intercept := (sy - slope*sx) / n
	var residual, variance float64
	for i := range x {
		residual += math.Pow(y[i]-(intercept+slope*x[i]), 2)
		variance += math.Pow(y[i]-sy/n, 2)
	}
	if variance == 0 || 1-residual/variance < 0.65 {
		f.Status = "variable"
		f.Message = "Die Belegung schwankt stark; eine Terminprognose wäre unzuverlässig."
		return f
	}
	f.Status = "growing"
	f.GrowthPerDay = int64(slope)
	remaining := math.Ceil(float64(v.Available) / slope)
	if remaining > 3650 {
		f.Message = "Bei gleichem Nettozuwachs reicht der Platz rechnerisch über zehn Jahre."
		return f
	}
	daysLeft := int(remaining)
	f.DaysToFull = &daysLeft
	f.FullAt = at.Add(time.Duration(daysLeft) * 24 * time.Hour).UTC().Format(time.RFC3339)
	f.Message = "Schätzung bei gleichbleibendem Nettozuwachs; Aufbewahrung und andere Programme können den Verlauf ändern."
	return f
}
