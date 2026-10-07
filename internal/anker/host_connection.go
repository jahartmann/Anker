package anker

import (
	"anker/internal/hostconnect"
	"anker/internal/updater"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

type hostConnector interface {
	Inspect(context.Context, hostconnect.InspectRequest) (hostconnect.Identity, error)
	Enroll(context.Context, hostconnect.EnrollRequest) (hostconnect.Enrollment, error)
}
type unixHostConnector struct{}

func hostHelperCall(ctx context.Context, path string, in, out any, timeout time.Duration) error {
	data, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "http://updater.local/hosts/"+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("X-Anker-Request", "1")
	res, err := updater.UnixClient(updater.Socket, timeout).Do(req)
	if err != nil {
		return fail(503, "Einrichtungsdienst nicht erreichbar; anker-updater prüfen")
	}
	defer res.Body.Close()
	data, err = io.ReadAll(io.LimitReader(res.Body, (17<<20)+1))
	if err != nil || len(data) > 17<<20 {
		return fail(502, "Antwort des Einrichtungsdienstes ungültig")
	}
	if res.StatusCode >= 400 {
		var failure struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &failure) != nil || failure.Error == "" {
			return fail(502, "Hostanbindung fehlgeschlagen")
		}
		return fail(res.StatusCode, failure.Error)
	}
	if json.Unmarshal(data, out) != nil {
		return fail(502, "Antwort des Einrichtungsdienstes ungültig")
	}
	return nil
}
func (unixHostConnector) Inspect(ctx context.Context, in hostconnect.InspectRequest) (out hostconnect.Identity, err error) {
	err = hostHelperCall(ctx, "inspect", in, &out, 30*time.Second)
	return
}
func (unixHostConnector) Enroll(ctx context.Context, in hostconnect.EnrollRequest) (out hostconnect.Enrollment, err error) {
	err = hostHelperCall(ctx, "enroll", in, &out, 6*time.Minute)
	return
}
func connectionInput(w http.ResponseWriter, r *http.Request, in any) error {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	d.DisallowUnknownFields()
	if d.Decode(in) != nil || d.Decode(new(any)) != io.EOF {
		return fail(400, "Angaben zur Hostanbindung ungültig")
	}
	return nil
}
func (s *Service) hostConnection(w http.ResponseWriter, r *http.Request, u User) error {
	if err := require(u, "admin"); err != nil {
		return err
	}
	if r.Method != "POST" {
		return fail(405, "POST erforderlich")
	}
	if s.Demo {
		return fail(409, "Echte Hostzugänge können in der Demo nicht eingerichtet werden")
	}
	connector := s.hostConnector
	if connector == nil {
		connector = unixHostConnector{}
	}
	switch r.URL.Path {
	case "/api/hosts/connection/inspect":
		var in hostconnect.InspectRequest
		if err := connectionInput(w, r, &in); err != nil {
			return err
		}
		in.Address = strings.TrimSpace(in.Address)
		if in.Port == 0 {
			in.Port = 22
		}
		if _, err := prepareHost(Host{Name: "connection", Address: in.Address, SSHPort: in.Port}); err != nil {
			return fail(400, err.Error())
		}
		identity, err := connector.Inspect(r.Context(), in)
		if err != nil {
			return err
		}
		return jsonOut(w, identity)
	case "/api/hosts/connection/enroll":
		var in struct {
			Host        Host   `json:"host"`
			Username    string `json:"username"`
			Password    string `json:"password"`
			Fingerprint string `json:"fingerprint"`
			Confirmed   bool   `json:"confirmed"`
		}
		if err := connectionInput(w, r, &in); err != nil {
			return err
		}
		if !in.Confirmed || in.Password == "" || len(in.Password) > 4096 || strings.ContainsAny(in.Password, "\x00\r\n") || !safeHost.MatchString(in.Username) || strings.Contains(in.Username, ":") || in.Fingerprint == "" {
			return fail(400, "SSH-Zugang und bestätigter Fingerprint erforderlich")
		}
		in.Host.Address = strings.TrimSpace(in.Host.Address)
		unnamed := in.Host.Name == ""
		if unnamed {
			in.Host.Name = "connection"
		}
		// Operator metadata is validated before any remote installation. Paths and roles are always selected by the root helper.
		in.Host.SSHUser = "anker"
		in.Host.KeyPath = "/etc/anker/keys/backup"
		in.Host.RestoreSSHUser = "anker-restore"
		in.Host.RestoreKeyPath = "/etc/anker/keys/restore"
		in.Host.KnownHostsPath = "/etc/anker/known_hosts"
		in.Host.Inventory = nil
		in.Host.LastProbe = ""
		in.Host.ProbeError = ""
		h, err := prepareHost(in.Host)
		if err != nil {
			return fail(400, err.Error())
		}
		if len(h.Group) > 200 || len(h.ClusterID) > 200 || len(h.ExtraPaths) > 100 {
			return fail(400, "Hostangaben zu umfangreich")
		}
		if err = s.beginHostConnection(h, in.Host.ID != ""); err != nil {
			return err
		}
		defer func() { s.jobMu.Lock(); s.connectingHostID = ""; s.jobMu.Unlock(); in.Password = "" }()
		result, err := connector.Enroll(r.Context(), hostconnect.EnrollRequest{Address: h.Address, Port: h.SSHPort, Username: in.Username, Password: in.Password, Fingerprint: in.Fingerprint, Confirmed: true})
		if err != nil {
			var e apiError
			if errors.As(err, &e) {
				return fail(e.code, strings.ReplaceAll(e.message, in.Password, "[geschützt]"))
			}
			return fail(400, "Hostanbindung fehlgeschlagen; Zugang und Rechte prüfen und erneut versuchen")
		}
		if result.Address != h.Address || result.Port != h.SSHPort || result.Fingerprint != in.Fingerprint || result.BackupUser != "anker" || result.RestoreUser != "anker-restore" || result.BackupKeyPath != h.KeyPath || result.RestoreKeyPath != h.RestoreKeyPath || result.KnownHostsPath != h.KnownHostsPath || bytes.Contains(result.Inventory, []byte(in.Password)) {
			return fail(502, "Geprüfter Hostzugang unvollständig; Einrichtung erneut versuchen")
		}
		var inv Inventory
		if json.Unmarshal(result.Inventory, &inv) != nil || inv.PVEVersion == "" || !safeHost.MatchString(inv.Hostname) {
			return fail(502, "Proxmox-Inventar unvollständig; Einrichtung erneut versuchen")
		}
		inv.Fingerprint = Fingerprint(inv)
		h.Inventory = &inv
		h.LastProbe = now()
		if unnamed {
			h.Name = inv.Hostname
		}
		s.hostMu.Lock()
		s.jobMu.Lock()
		err = s.saveHost(h)
		s.jobMu.Unlock()
		s.hostMu.Unlock()
		if err != nil {
			return fail(500, "Zugang eingerichtet, Host konnte nicht gespeichert werden; Einrichtung wiederholen")
		}
		s.LogAudit(u.ID, "host.connect", h.ID+" "+result.Fingerprint)
		return jsonOut(w, h)
	default:
		return fail(404, "Nicht gefunden")
	}
}
func (s *Service) beginHostConnection(h Host, existing bool) error {
	s.hostMu.Lock()
	defer s.hostMu.Unlock()
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	if s.maintenance || s.connectingHostID != "" {
		return fail(409, "Update oder Hostanbindung läuft; anschließend erneut versuchen")
	}
	if existing {
		if _, err := s.Host(h.ID); err != nil {
			return fail(404, "Host nicht gefunden")
		}
	}
	hosts, err := s.Hosts()
	if err != nil {
		return err
	}
	for _, current := range hosts {
		if current.ID != h.ID && strings.EqualFold(current.Address, h.Address) && current.SSHPort == h.SSHPort {
			return fail(409, "Host ist bereits angelegt; dort Verbindung einrichten")
		}
	}
	jobs, err := s.Jobs()
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if job.HostID == h.ID && activeJob(job) {
			return fail(409, "Aktiven Hostauftrag zuerst beenden")
		}
	}
	s.mu.Lock()
	locked := s.locks[h.ID]
	s.mu.Unlock()
	if locked {
		return fail(409, "Aktiven Hostauftrag zuerst beenden")
	}
	s.connectingHostID = h.ID
	return nil
}
