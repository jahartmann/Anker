package anker

import (
	"anker/internal/storage"
	"anker/internal/updater"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

type storageHistory struct {
	At      string                      `json:"at"`
	Samples map[string][]storage.Sample `json:"samples"`
}

func (s *Service) RunStorage(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if err := s.RecordStorage(time.Now()); err != nil {
			log.Printf("Anker Speichermessung: %v", err)
			s.jobMu.Lock()
			if !s.maintenance {
				s.Store.Put("health", "storage", map[string]string{"at": now(), "error": "Speichermessung fehlgeschlagen; Dienstprotokoll prüfen."})
			}
			s.jobMu.Unlock()
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) storageHistory() (storageHistory, error) {
	var h storageHistory
	err := s.Store.Get("storage", "history", &h)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if h.Samples == nil {
		h.Samples = map[string][]storage.Sample{}
	}
	return h, err
}
func (s *Service) RecordStorage(at time.Time) error {
	if s.Demo {
		return nil
	}
	s.storageMu.Lock()
	defer s.storageMu.Unlock()
	h, err := s.storageHistory()
	if err != nil {
		return err
	}
	previous, _ := time.Parse(time.RFC3339, h.At)
	if !previous.IsZero() && !at.Before(previous) && at.Sub(previous) < time.Hour {
		return nil
	}
	report, err := storage.Inspect(s.Root)
	if err != nil {
		return err
	}
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	if s.maintenance {
		return nil
	}
	return s.recordStorageLocked(report, at, h)
}
func (s *Service) recordStorage(r storage.Report, at time.Time) error {
	s.storageMu.Lock()
	defer s.storageMu.Unlock()
	h, err := s.storageHistory()
	if err != nil {
		return err
	}
	previous, _ := time.Parse(time.RFC3339, h.At)
	if !previous.IsZero() && !at.Before(previous) && at.Sub(previous) < time.Hour {
		return nil
	}
	return s.recordStorageLocked(r, at, h)
}
func (s *Service) recordStorageLocked(r storage.Report, at time.Time, h storageHistory) error {
	next := storageHistory{At: at.UTC().Format(time.RFC3339), Samples: map[string][]storage.Sample{}}
	for _, v := range r.Volumes {
		if v.Error != "" || v.Total <= 0 {
			continue
		}
		points := h.Samples[v.ID]
		if len(points) > 0 && points[len(points)-1].Total != v.Total {
			points = nil
		}
		valid := []storage.Sample{}
		for _, p := range points {
			t, e := time.Parse(time.RFC3339, p.At)
			if e == nil && !t.After(at) && at.Sub(t) <= 90*24*time.Hour {
				valid = append(valid, p)
			}
		}
		valid = append(valid, storage.Sample{At: next.At, Total: v.Total, Used: v.Used, Available: v.Available})
		if len(valid) > 2160 {
			valid = valid[len(valid)-2160:]
		}
		next.Samples[v.ID] = valid
	}
	if len(next.Samples) > 256 {
		return errors.New("Zu viele Dateisysteme für die Messreihe")
	}
	if err := s.Store.Put("storage", "history", next); err != nil {
		return err
	}
	return s.Store.Put("health", "storage", map[string]string{"at": next.At, "error": ""})
}
func (s *Service) StorageReport() (storage.Report, error) {
	if s.Demo {
		return storage.Report{Demo: true, Volumes: []storage.Volume{}, Devices: []storage.Device{}, Warnings: []string{"Speicherverwaltung ist in der Demo ausgeschaltet. Es werden keine echten Geräte gelesen oder verändert."}}, nil
	}
	r, err := storage.Inspect(s.Root)
	if err != nil {
		return r, err
	}
	h, err := s.storageHistory()
	if err != nil {
		return r, err
	}
	at := time.Now()
	for i := range r.Volumes {
		v := &r.Volumes[i]
		points := h.Samples[v.ID]
		v.Forecast = storage.ForecastFor(*v, points, at)
		// A compact chart keeps at most one real point from each day, plus the latest.
		seen := map[string]bool{}
		for j := len(points) - 1; j >= 0; j-- {
			day := strings.SplitN(points[j].At, "T", 2)[0]
			if !seen[day] {
				v.History = append([]storage.Sample{points[j]}, v.History...)
				seen[day] = true
			}
		}
	}
	var health map[string]string
	if s.Store.Get("health", "storage", &health) == nil && health["error"] != "" {
		r.Warnings = append(r.Warnings, health["error"])
	}
	return r, nil
}
func (s *Service) capacity(w http.ResponseWriter, r *http.Request, u User) error {
	if err := require(u, "admin"); err != nil {
		return err
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/storage")
	if path == "" && r.Method == "GET" {
		report, err := s.StorageReport()
		if err != nil {
			return err
		}
		return jsonOut(w, report)
	}
	if !((path == "/state" && r.Method == "GET") || ((path == "/plan" || path == "/grow") && r.Method == "POST")) {
		return fail(405, "Methode nicht unterstützt")
	}
	if s.Demo {
		return fail(409, "Speichererweiterung ist in der Demo ausgeschaltet")
	}
	var payload struct {
		VolumeID     string `json:"volume_id"`
		PlanID       string `json:"plan_id,omitempty"`
		Confirmation string `json:"confirmation,omitempty"`
	}
	var body io.Reader
	if r.Method == "POST" {
		if err := input(r, &payload); err != nil {
			return err
		}
		if payload.VolumeID == "" || len(payload.VolumeID) > 64 {
			return fail(400, "Dateisystem auswählen")
		}
		data, _ := json.Marshal(payload)
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, "http://updater.local/storage"+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("X-Anker-Request", "1")
	response, err := updater.UnixClient(updater.Socket, 20*time.Second).Do(req)
	if err != nil {
		return fail(503, "Speicherwerkzeuge nicht erreichbar; anker-updater.service prüfen")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if !json.Valid(data) {
		return fail(502, "Ungültige Antwort der Speicherverwaltung")
	}
	if response.StatusCode >= 400 {
		var v struct {
			Error string `json:"error"`
		}
		json.Unmarshal(data, &v)
		return fail(response.StatusCode, v.Error)
	}
	if path == "/grow" {
		s.LogAudit(u.ID, "storage.grow", payload.VolumeID)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(response.StatusCode)
	_, err = w.Write(data)
	return err
}
