package updater

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

func (s *Server) storageHandler(w http.ResponseWriter, r *http.Request) {
	fail := func(code int, message string) {
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]string{"error": message})
	}
	if s.storageManager == nil {
		fail(503, "Speicherverwaltung nicht verfügbar; Dienstprotokoll und Operationsjournal prüfen")
		return
	}
	if r.Method == "GET" && r.URL.Path == "/storage/state" {
		json.NewEncoder(w).Encode(s.storageManager.Status())
		return
	}
	if r.Method != "POST" || (r.URL.Path != "/storage/plan" && r.URL.Path != "/storage/grow") {
		fail(405, "Methode nicht unterstützt")
		return
	}
	if r.Header.Get("X-Anker-Request") != "1" {
		fail(403, "Anfrageherkunft ungültig")
		return
	}
	var in struct {
		VolumeID     string `json:"volume_id"`
		PlanID       string `json:"plan_id,omitempty"`
		Confirmation string `json:"confirmation,omitempty"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		fail(400, "Ungültige Speicheranfrage")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF || len(in.VolumeID) > 64 || in.VolumeID == "" || len(in.PlanID) > 64 || len(in.Confirmation) > 256 {
		fail(400, "Ungültige oder zusätzliche Daten")
		return
	}
	if r.URL.Path == "/storage/plan" {
		p, err := s.storageManager.Plan(in.VolumeID)
		if err != nil {
			fail(400, err.Error())
			return
		}
		json.NewEncoder(w).Encode(p)
		return
	}
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		fail(409, "Ein Update oder eine Systemänderung läuft; erneut versuchen")
		return
	}
	s.busy = true
	s.mu.Unlock()
	state, complete, err := s.storageManager.Begin(in.VolumeID, in.PlanID, in.Confirmation)
	if err != nil {
		s.mu.Lock()
		s.busy = false
		s.mu.Unlock()
		code := 400
		if errors.Is(err, ErrTLSBusy) {
			code = 409
		}
		fail(code, err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(state)
	go func() { complete(); s.mu.Lock(); s.busy = false; s.mu.Unlock() }()
}
