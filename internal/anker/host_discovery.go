package anker

import (
	"anker/internal/discovery"
	"database/sql"
	"errors"
	"net/http"
)

func (s *Service) hostDiscovery(w http.ResponseWriter, r *http.Request, u User) error {
	if err := require(u, "admin"); err != nil {
		return err
	}
	s.discoveryMu.Lock()
	defer s.discoveryMu.Unlock()
	switch r.Method {
	case "GET":
		config := discovery.Config{Ranges: []string{}, WebPort: 8006, SSHPort: 22}
		if err := s.Store.Get("discovery", "networks", &config); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fail(500, "Gespeicherte Netzbereiche nicht lesbar")
		}
		return jsonOut(w, map[string]any{"config": config, "suggested_ranges": discovery.LocalRanges(), "scan": s.discovery.Snapshot()})
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
			s.LogAudit(u.ID, "hosts.discover", "Serversuche gestartet")
		}
		return jsonOut(w, map[string]any{"config": config, "scan": s.discovery.Snapshot()})
	case "DELETE":
		s.discovery.Cancel()
		return jsonOut(w, s.discovery.Snapshot())
	default:
		return fail(405, "Methode nicht unterstützt")
	}
}
