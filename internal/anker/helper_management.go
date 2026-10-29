package anker

import (
	"anker/internal/hostconnect"
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type HelperStatus struct {
	HostID           string         `json:"host_id"`
	Name             string         `json:"name"`
	State            string         `json:"state"`
	InstalledVersion string         `json:"installed_version,omitempty"`
	InstalledHash    string         `json:"installed_hash,omitempty"`
	DesiredVersion   string         `json:"desired_version"`
	DesiredHash      string         `json:"desired_hash"`
	LastProbe        string         `json:"last_probe,omitempty"`
	Capabilities     map[string]any `json:"capabilities,omitempty"`
}

func (s *Service) HelperStatuses() ([]HelperStatus, error) {
	hosts, e := s.Hosts()
	if e != nil {
		return nil, e
	}
	out := []HelperStatus{}
	for _, h := range hosts {
		v := HelperStatus{HostID: h.ID, Name: h.Name, State: "unknown", DesiredVersion: hostconnect.HelperVersion, DesiredHash: hostconnect.HelperHash(), LastProbe: h.LastProbe}
		if h.Inventory != nil {
			var caps map[string]any
			json.Unmarshal(h.Inventory.Details["capabilities"], &caps)
			v.Capabilities = map[string]any{}
			for _, key := range []string{"config_backup", "restore_protocol", "remove_files", "journal", "recovery_read_protocol", "recovery_verify_protocol", "pbs_restore_protocol"} {
				if value, ok := caps[key]; ok {
					switch value.(type) {
					case bool, float64:
						v.Capabilities[key] = value
					}
				}
			}
			v.InstalledVersion = h.Inventory.HelperVersion
			v.InstalledHash = h.Inventory.HelperSHA256
			if v.InstalledHash != "" {
				v.State = "different"
				if v.InstalledHash == v.DesiredHash {
					v.State = "current"
				}
			}
		}
		out = append(out, v)
	}
	return out, nil
}
func (unixHostConnector) Refresh(ctx context.Context, in hostconnect.EnrollRequest) (out hostconnect.RefreshResult, err error) {
	err = hostHelperCall(ctx, "refresh", in, &out, 3*time.Minute)
	return
}
func (s *Service) refreshHostHelper(u User, w http.ResponseWriter, r *http.Request) error {
	if e := require(u, "admin"); e != nil {
		return e
	}
	if s.Demo {
		return fail(409, "Helferaktualisierung ist in der Demo deaktiviert")
	}
	var in struct {
		HostID      string `json:"host_id"`
		Revision    string `json:"revision"`
		Username    string `json:"username"`
		Password    string `json:"password"`
		Fingerprint string `json:"fingerprint"`
		Confirmed   bool   `json:"confirmed"`
	}
	if e := connectionInput(w, r, &in); e != nil {
		return e
	}
	defer func() { in.Password = "" }()
	h, e := s.Host(in.HostID)
	if e != nil {
		return e
	}
	if hostRevision(h) != in.Revision {
		return fail(409, "Hosteinstellungen geändert; Auswahl neu laden")
	}
	// Privileged manager deliberately uses only its canonical, protected trust file.
	if h.KnownHostsPath != "/etc/anker/known_hosts" {
		return fail(409, "Automatische Helferaktualisierung benötigt den verwalteten Hostschlüsselspeicher")
	}
	if e = s.beginHostConnection(h, true); e != nil {
		return e
	}
	defer func() { s.jobMu.Lock(); s.connectingHostID = ""; s.jobMu.Unlock() }()
	current, e := s.Host(h.ID)
	if e != nil || hostRevision(current) != in.Revision {
		return fail(409, "Hosteinstellungen geändert; Auswahl neu laden")
	}
	connector := s.hostConnector
	if connector == nil {
		connector = unixHostConnector{}
	}
	refresher, ok := connector.(interface {
		Refresh(context.Context, hostconnect.EnrollRequest) (hostconnect.RefreshResult, error)
	})
	if !ok {
		return fail(503, "Helferaktualisierung nicht verfügbar")
	}
	result, e := refresher.Refresh(r.Context(), hostconnect.EnrollRequest{Address: h.Address, Port: h.SSHPort, Username: in.Username, Password: in.Password, Fingerprint: in.Fingerprint, Confirmed: in.Confirmed})
	in.Password = ""
	if e != nil {
		return fail(400, "Helferaktualisierung nicht bestätigt; Hostidentität, Administratorzugang und Hostjournal prüfen")
	}
	s.LogAudit(u.ID, "host.helper.refresh", h.ID)
	return jsonOut(w, result)
}
