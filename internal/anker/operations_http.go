package anker

import (
	"net/http"
	"strings"
)

func (s *Service) operationsHTTP(u User, w http.ResponseWriter, r *http.Request) (bool, error) {
	path := strings.TrimPrefix(r.URL.Path, "/api/operations/")
	if path == r.URL.Path {
		return false, nil
	}
	if r.Method == "GET" {
		switch {
		case path == "incidents":
			v, e := s.Incidents()
			if e != nil {
				return true, e
			}
			return true, jsonOut(w, redactRecovery(v))
		case path == "helpers":
			v, e := s.HelperStatuses()
			if e != nil {
				return true, e
			}
			return true, jsonOut(w, redactRecovery(v))
		case strings.HasPrefix(path, "hosts/"):
			v, e := s.HostOperationRun(u.ID, strings.TrimPrefix(path, "hosts/"))
			if e != nil {
				return true, e
			}
			return true, jsonOut(w, redactRecovery(v))
		case strings.HasPrefix(path, "drift/"):
			v, e := s.HostDrift(strings.TrimPrefix(path, "drift/"))
			if e != nil {
				return true, e
			}
			return true, jsonOut(w, redactRecovery(v))
		}
	}
	if r.Method == "POST" && path == "hosts" {
		var in HostOperationRequest
		if e := input(r, &in); e != nil {
			return true, e
		}
		role := "admin"
		if operationalRead(in.Action) {
			role = "restore"
		}
		if e := require(u, role); e != nil {
			return true, e
		}
		v, e := s.StartHostOperation(u.ID, in)
		if e != nil {
			return true, e
		}
		s.LogAudit(u.ID, "host_operations."+in.Action, v.ID)
		return true, jsonOut(w, redactRecovery(v))
	}
	if r.Method == "POST" && strings.HasPrefix(path, "incidents/") {
		if e := require(u, "restore"); e != nil {
			return true, e
		}
		var in struct {
			Acknowledged bool `json:"acknowledged"`
		}
		if e := input(r, &in); e != nil {
			return true, e
		}
		v, e := s.AcknowledgeIncident(strings.TrimPrefix(path, "incidents/"), u.ID, in.Acknowledged)
		if e != nil {
			return true, e
		}
		return true, jsonOut(w, redactRecovery(v))
	}
	if r.Method == "POST" && path == "helpers/refresh" {
		return true, s.refreshHostHelper(u, w, r)
	}
	return true, fail(404, "Betriebsaktion nicht gefunden")
}
