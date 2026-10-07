package anker

import (
	"anker/internal/buildinfo"
	"anker/internal/updater"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

func (s *Service) PrepareUpdate() error {
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	if s.connectingHostID != "" {
		return errors.New("Hostanbindung zuerst abschließen")
	}
	if s.schedulerBusy {
		return errors.New("Laufende Aufbewahrungsprüfung zuerst beenden")
	}
	jobs, err := s.Jobs()
	if err != nil {
		return err
	}
	for _, j := range jobs {
		if j.State == "queued" || j.State == "running" {
			return errors.New("Aktive Aufträge zuerst beenden; während einer Sicherung oder Wiederherstellung wird kein Update installiert")
		}
	}
	s.maintenance = true
	return nil
}
func (s *Service) ReleaseUpdate() { s.jobMu.Lock(); s.maintenance = false; s.jobMu.Unlock() }
func (s *Service) Updating() bool { s.jobMu.Lock(); defer s.jobMu.Unlock(); return s.maintenance }
func (s *Service) updates(w http.ResponseWriter, r *http.Request, u User) error {
	if err := require(u, "admin"); err != nil {
		return err
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/updates")
	if path == "" && r.Method == "GET" {
		path = "/status"
	} else if path == "/configuration" && r.Method == "GET" {
		// Public trust information for the administrator.
	} else if (path != "/check" && path != "/install" && path != "/configure") || r.Method != "POST" {
		return fail(405, "Methode nicht unterstützt")
	}
	if s.Demo {
		if path == "/configure" {
			return fail(409, "Updatequelle kann in der Demo nicht geändert werden")
		}
		if path == "/configuration" {
			c := updater.DefaultConfiguration()
			c.Demo = true
			return jsonOut(w, c)
		}
		return jsonOut(w, updater.State{Current: buildinfo.Version, Status: "unconfigured", Message: "Updates sind in der Demo ausgeschaltet. Auf dem eigenen Server lässt sich die Quelle hier einrichten."})
	}
	var body io.Reader
	if path == "/configure" {
		var in updater.ConfigureRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&in); err != nil {
			return fail(400, "Ungültige Updatequelle")
		}
		if !in.Confirmed {
			return fail(400, "Vertrauen in Repository und Signierschlüssel ausdrücklich bestätigen")
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return fail(400, "Zusätzliche Daten nicht erlaubt")
		}
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	if path == "/install" {
		var in struct {
			Version string `json:"version"`
		}
		if err := input(r, &in); err != nil {
			return err
		}
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, "http://updater.local"+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("X-Anker-Request", "1")
	response, err := updater.UnixClient(updater.Socket, 40*time.Second).Do(req)
	if err != nil {
		if path == "/status" {
			return jsonOut(w, updater.State{Current: buildinfo.Version, Status: "unconfigured", Message: "Updater nicht erreichbar. systemd-Dienst prüfen."})
		}
		return fail(503, "Updater nicht erreichbar; systemd-Dienst prüfen")
	}
	defer response.Body.Close()
	b, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return err
	}
	if response.StatusCode < 400 {
		if path == "/configuration" || path == "/configure" {
			var configuration updater.Configuration
			if err = json.Unmarshal(b, &configuration); err != nil {
				return err
			}
			if path == "/configure" {
				s.LogAudit(u.ID, "update.configure", configuration.Repository+" "+configuration.Fingerprint)
			}
			return jsonOut(w, configuration)
		}
		var state updater.State
		if err = json.Unmarshal(b, &state); err != nil {
			return err
		}
		state.Current = buildinfo.Version
		if path == "/install" {
			s.LogAudit(u.ID, "update.install", state.Target)
		}
		return jsonOut(w, state)
	}
	var failure struct {
		Error string `json:"error"`
	}
	if err = json.Unmarshal(b, &failure); err != nil {
		return err
	}
	return fail(response.StatusCode, failure.Error)
}
