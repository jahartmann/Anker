package anker

import (
	"anker/internal/discovery"
	"database/sql"
	"errors"
	"net/http"
)

func (s *Service) initDiscovery() {
	s.discovery.Save = func(cp discovery.Checkpoint) error { return s.Store.Put("discovery", "checkpoint", cp) }
	s.discovery.Seeds = s.discoverySeeds
	var cp discovery.Checkpoint
	if err := s.Store.Get("discovery", "checkpoint", &cp); err == nil {
		if s.discovery.Restore(cp) != nil {
			s.discoveryRestoreError = "Gespeicherte Suche ist nicht lesbar. Neue Suche starten; keine automatische Fortsetzung."
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		s.discoveryRestoreError = "Gespeicherter Suchfortschritt konnte nicht gelesen werden."
	}
}
func (s *Service) hostDiscovery(w http.ResponseWriter, r *http.Request, u User) error {
	if err := require(u, "admin"); err != nil {
		return err
	}
	s.discoveryMu.Lock()
	defer s.discoveryMu.Unlock()
	if r.Method != "GET" && s.Updating() {
		return fail(503, "Anker wird aktualisiert; Suche später starten oder fortsetzen")
	}
	switch r.Method {
	case "GET":
		config := discovery.Config{Ranges: []string{}, WebPort: 8006, PBSPort: 8007, SSHPort: 22, Parallel: 256}
		if err := s.Store.Get("discovery", "networks", &config); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fail(500, "Gespeicherte Netzbereiche nicht lesbar")
		}
		if config.PBSPort == 0 {
			config.PBSPort = 8007
		}
		if config.Parallel == 0 {
			config.Parallel = 256
		}
		scan := s.discovery.Snapshot()
		if s.discoveryRestoreError != "" {
			scan.Error = s.discoveryRestoreError
			scan.Status = "failed"
		}
		return jsonOut(w, map[string]any{"config": config, "suggested_ranges": discovery.LocalRanges(), "scan": scan})
	case "POST", "PUT":
		if s.Demo {
			return fail(409, "Serversuche und Netzbereiche sind in der Demo nicht verfügbar")
		}
		var config discovery.Config
		if err := connectionInput(w, r, &config); err != nil {
			return err
		}
		config, _, err := discovery.Normalize(config)
		if err != nil {
			return fail(400, err.Error())
		}
		if s.discovery.Snapshot().Status == "running" {
			return fail(409, "Serversuche läuft bereits; zuerst abbrechen")
		}
		if err := s.Store.Put("discovery", "networks", config); err != nil {
			return fail(500, "Netzbereiche konnten nicht gespeichert werden")
		}
		if r.Method == "POST" {
			if err := s.discovery.Start(config); err != nil {
				return fail(409, err.Error())
			}
			s.discoveryRestoreError = ""
			s.LogAudit(u.ID, "hosts.discover", "PVE-/PBS-Serversuche gestartet")
		}
		return jsonOut(w, map[string]any{"config": config, "scan": s.discovery.Snapshot()})
	case "PATCH":
		if s.Demo {
			return fail(409, "Serversuche ist in der Demo nicht verfügbar")
		}
		var in struct{}
		if err := connectionInput(w, r, &in); err != nil {
			return err
		}
		if err := s.discovery.Resume(); err != nil {
			return fail(409, err.Error())
		}
		s.LogAudit(u.ID, "hosts.discover.resume", "Gespeicherte Netzsuche fortgesetzt")
		return jsonOut(w, map[string]any{"scan": s.discovery.Snapshot()})
	case "DELETE":
		s.discovery.Cancel()
		return jsonOut(w, s.discovery.Snapshot())
	default:
		return fail(405, "Methode nicht unterstützt")
	}
}
