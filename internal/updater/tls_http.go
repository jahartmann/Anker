package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func (s *Server) tlsHandler(w http.ResponseWriter, r *http.Request) {
	fail := func(code int, message string) {
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]string{"error": message})
	}
	if s.tlsManager == nil {
		fail(503, "Zertifikatsverwaltung nicht eingerichtet")
		return
	}
	if r.URL.Path == "/tls" && r.Method == "GET" {
		status, err := s.tlsManager.Status()
		if err != nil {
			fail(503, "TLS-Konfiguration prüfen: "+err.Error())
			return
		}
		json.NewEncoder(w).Encode(status)
		return
	}
	if r.URL.Path == "/tls/certificate" && r.Method == "GET" {
		data, err := s.tlsManager.Certificate()
		if err != nil {
			fail(400, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Header().Set("Content-Disposition", `attachment; filename="anker-server.crt"`)
		w.Write(data)
		return
	}
	if r.Method != "POST" || (r.URL.Path != "/tls" && r.URL.Path != "/tls/renew") {
		fail(405, "Methode nicht unterstützt")
		return
	}
	if r.Header.Get("X-Anker-Request") != "1" {
		fail(403, "Anfrageherkunft ungültig")
		return
	}
	var in struct {
		Automatic       bool `json:"automatic"`
		RenewBeforeDays int  `json:"renew_before_days"`
	}
	var empty struct{}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	decoder.DisallowUnknownFields()
	var target any = &in
	if r.URL.Path == "/tls/renew" {
		target = &empty
	}
	if err := decoder.Decode(target); err != nil {
		fail(400, "Ungültige TLS-Einstellungen")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		fail(400, "Zusätzliche Daten nicht erlaubt")
		return
	}
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		fail(409, "Update oder Zertifikatsänderung läuft; erneut versuchen")
		return
	}
	s.busy = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.busy = false; s.mu.Unlock() }()
	var status TLSStatus
	var err error
	if r.URL.Path == "/tls/renew" {
		status, err = s.tlsManager.Renew()
	} else {
		status, err = s.tlsManager.SavePolicy(TLSPolicy{Automatic: in.Automatic, RenewBeforeDays: in.RenewBeforeDays})
	}
	if err != nil {
		if errors.Is(err, ErrTLSBusy) {
			fail(409, err.Error())
			return
		}
		fail(400, err.Error())
		return
	}
	json.NewEncoder(w).Encode(status)
}

func (s *Server) maintainTLS(ctx context.Context) {
	maintain := func() {
		s.mu.Lock()
		if s.busy {
			s.mu.Unlock()
			return
		}
		s.busy = true
		s.mu.Unlock()
		err := s.tlsManager.Maintain()
		s.mu.Lock()
		s.busy = false
		s.mu.Unlock()
		if err != nil && !errors.Is(err, ErrTLSBusy) {
			fmt.Fprintln(os.Stderr, "Automatische TLS-Prüfung fehlgeschlagen:", err)
		}
	}
	maintain()
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			maintain()
		}
	}
}
