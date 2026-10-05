package anker

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type apiError struct {
	code    int
	message string
}

func (e apiError) Error() string      { return e.message }
func fail(code int, msg string) error { return apiError{code, msg} }
func jsonOut(w http.ResponseWriter, v any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(v)
}
func input(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return fail(400, "Ungültige Anfrage: "+err.Error())
	}
	return nil
}

type responseState struct {
	http.ResponseWriter
	started bool
}

func (w *responseState) WriteHeader(status int) {
	w.started = true
	w.ResponseWriter.WriteHeader(status)
}
func (w *responseState) Write(p []byte) (int, error) {
	w.started = true
	return w.ResponseWriter.Write(p)
}
func Handler(s *Service, a *Auth, local bool) http.Handler {
	return http.HandlerFunc(func(original http.ResponseWriter, r *http.Request) {
		w := &responseState{ResponseWriter: original}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			if r.Header.Get("X-Anker-Request") != "1" {
				jsonError(w, fail(403, "Anfrageherkunft ungültig"))
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || u.Host != r.Host || (u.Scheme != "http" && u.Scheme != "https") {
					jsonError(w, fail(403, "Anfrageherkunft ungültig"))
					return
				}
			}
		}
		if r.URL.Path == "/api/login" {
			if r.Method != "POST" {
				jsonError(w, fail(405, "POST erforderlich"))
				return
			}
			remote, _, _ := net.SplitHostPort(r.RemoteAddr)
			if remote == "" {
				remote = r.RemoteAddr
			}
			if !a.PermitLogin(remote) {
				jsonError(w, fail(429, "Zu viele Anmeldeversuche; später erneut versuchen"))
				return
			}
			var in struct {
				Name     string `json:"name"`
				Password string `json:"password"`
			}
			if err := input(r, &in); err != nil {
				jsonError(w, err)
				return
			}
			token, u, err := a.Login(in.Name, in.Password)
			if err != nil {
				jsonError(w, fail(401, "Anmeldung fehlgeschlagen"))
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "anker_session", Value: token, Path: "/", HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: 8 * 3600})
			jsonOut(w, u)
			return
		}
		u := User{ID: "local", Name: "local", Role: "admin", Secrets: true}
		if !local {
			cookie, err := r.Cookie("anker_session")
			if err != nil {
				jsonError(w, fail(401, "Anmeldung erforderlich"))
				return
			}
			u, err = a.Session(cookie.Value, time.Now())
			if err != nil {
				jsonError(w, fail(401, "Anmeldung erforderlich"))
				return
			}
		}
		if r.Method != "GET" && r.Method != "HEAD" && r.URL.Path != "/api/logout" && r.URL.Path != "/api/backups/diff" && !userAllows(u, "restore") {
			jsonError(w, fail(403, "Keine Schreibberechtigung"))
			return
		}
		if err := handleAPI(s, a, u, w, r); err != nil {
			if w.started {
				panic(http.ErrAbortHandler)
			}
			jsonError(w, err)
		}
	})
}
func jsonError(w http.ResponseWriter, err error) {
	w.Header().Del("Content-Disposition")
	w.Header().Del("Content-Length")
	code := 400
	var e apiError
	if errors.As(err, &e) {
		code = e.code
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
func require(u User, action string) error {
	if !userAllows(u, action) {
		return fail(403, deny(action).Error())
	}
	return nil
}
func redactPlan(p Plan, reveal bool) Plan {
	if !reveal {
		p.Steps = append([]Step(nil), p.Steps...)
		for i := range p.Steps {
			if p.Steps[i].Secret || isSecret(p.Steps[i].Path) {
				p.Steps[i].Diff = "[Verdeckter Inhalt]"
			}
		}
	}
	return p
}
func handleAPI(s *Service, a *Auth, u User, w http.ResponseWriter, r *http.Request) error {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	path := strings.Join(parts[1:], "/")
	method := r.Method
	switch path {
	case "doctor":
		if err := require(u, "admin"); err != nil {
			return err
		}
		disk, err := s.DiskUsage()
		if err != nil {
			return err
		}
		return jsonOut(w, map[string]any{"data_root": s.Root, "disk": disk, "demo": s.Demo, "protocol": FormatVersion, "host_helper": "Python 3, root-owned fixed helper; SSH keys and verified known_hosts required", "support": "File restore with guarded preconditions; full scenarios require manual checks and real lab validation"})
	case "me":
		return jsonOut(w, map[string]any{"user": u, "demo": s.Demo})
	case "logout":
		if method != "POST" {
			return fail(405, "POST erforderlich")
		}
		if c, err := r.Cookie("anker_session"); err == nil {
			a.Logout(c.Value)
		}
		http.SetCookie(w, &http.Cookie{Name: "anker_session", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		return jsonOut(w, map[string]bool{"ok": true})
	case "status":
		hosts, err := s.Hosts()
		if err != nil {
			return err
		}
		backups, err := s.ListBackups("")
		if err != nil {
			return err
		}
		jobs, err := s.Jobs()
		if err != nil {
			return err
		}
		plans, err := s.Plans()
		if err != nil {
			return err
		}
		for i := range plans {
			plans[i] = redactPlan(plans[i], false)
			plans[i].Source.Details = nil
			plans[i].Target.Details = nil
		}
		settings, _ := s.Settings()
		var notificationHealth map[string]string
		s.Store.Get("health", "notifications", &notificationHealth)
		var maintenanceHealth map[string]string
		s.Store.Get("health", "maintenance", &maintenanceHealth)
		for i := range hosts {
			if u.Role != "admin" {
				hosts[i].KeyPath = ""
				hosts[i].RestoreKeyPath = ""
				hosts[i].KnownHostsPath = ""
			}
		}
		return jsonOut(w, map[string]any{"hosts": hosts, "backups": backups, "jobs": jobs, "plans": plans, "demo": s.Demo, "timezone": settings.Timezone, "stale_hours": settings.StaleHours, "notification_health": notificationHealth, "maintenance_health": maintenanceHealth})
	case "hosts":
		if method == "GET" {
			hosts, err := s.Hosts()
			if err != nil {
				return err
			}
			for i := range hosts {
				if u.Role != "admin" {
					hosts[i].KeyPath = ""
					hosts[i].RestoreKeyPath = ""
					hosts[i].KnownHostsPath = ""
				}
			}
			return jsonOut(w, hosts)
		}
		if method != "POST" {
			return fail(405, "Methode nicht unterstützt")
		}
		if err := require(u, "admin"); err != nil {
			return err
		}
		var h Host
		if err := input(r, &h); err != nil {
			return err
		}
		if h.ID == "" {
			h.ID = ID()
		}
		if err := s.SaveHost(h); err != nil {
			return err
		}
		s.LogAudit(u.ID, "host.save", h.ID)
		return jsonOut(w, h)
	case "plans":
		if method == "GET" {
			p, err := s.Plans()
			if err != nil {
				return err
			}
			for i := range p {
				p[i] = redactPlan(p[i], false)
			}
			return jsonOut(w, p)
		}
		if method != "POST" {
			return fail(405, "Methode nicht unterstützt")
		}
		var in PlanRequest
		if err := input(r, &in); err != nil {
			return err
		}
		p, err := s.CreatePlan(r.Context(), in)
		if err != nil {
			return err
		}
		s.LogAudit(u.ID, "plan.create", p.ID)
		return jsonOut(w, redactPlan(p, false))
	case "settings":
		if err := require(u, "admin"); err != nil {
			return err
		}
		v, err := s.Settings()
		if err != nil {
			return err
		}
		if method == "GET" {
			v.SMTPPassword = ""
			return jsonOut(w, v)
		}
		if method != "PUT" {
			return fail(405, "Methode nicht unterstützt")
		}
		var in Settings
		if err := input(r, &in); err != nil {
			return err
		}
		if in.SMTPPassword == "" {
			in.SMTPPassword = v.SMTPPassword
		}
		if err = s.SaveSettings(in); err != nil {
			return err
		}
		s.LogAudit(u.ID, "settings.save", "main")
		in.SMTPPassword = ""
		return jsonOut(w, in)
	case "users":
		if err := require(u, "admin"); err != nil {
			return err
		}
		if method == "GET" {
			users, err := records[User](s.Store, "users")
			if err != nil {
				return err
			}
			return jsonOut(w, users)
		}
		if method != "POST" {
			return fail(405, "Methode nicht unterstützt")
		}
		var in struct {
			Name     string `json:"name"`
			Password string `json:"password"`
			Role     string `json:"role"`
			Secrets  bool   `json:"secrets"`
		}
		if err := input(r, &in); err != nil {
			return err
		}
		if err := a.CreateUser(in.Name, in.Password, in.Role, in.Secrets); err != nil {
			return err
		}
		s.LogAudit(u.ID, "user.create", in.Name)
		return jsonOut(w, map[string]bool{"ok": true})
	case "backups/diff":
		if method != "POST" {
			return fail(405, "POST erforderlich")
		}
		var in struct {
			From string `json:"from"`
			To   string `json:"to"`
			Path string `json:"path"`
		}
		if err := input(r, &in); err != nil {
			return err
		}
		v, err := s.DiffBackups(in.From, in.To, in.Path)
		if err != nil {
			return err
		}
		for i := range v {
			if !v[i].Secret {
				left, leftEntry, _ := s.ReadFile(in.From, v[i].Path)
				right, rightEntry, _ := s.ReadFile(in.To, v[i].Path)
				v[i].Secret = leftEntry.Secret || rightEntry.Secret
				if !v[i].Secret {
					v[i].Diff = lineDiff(string(left), string(right))
				}
			}
		}
		return jsonOut(w, v)
	case "audit":
		if err := require(u, "admin"); err != nil {
			return err
		}
		v, err := records[Audit](s.Store, "audit")
		if err != nil {
			return err
		}
		sort.Slice(v, func(i, j int) bool { return v[i].At > v[j].At })
		if len(v) > 500 {
			v = v[:500]
		}
		return jsonOut(w, v)
	case "notifications/test":
		if err := require(u, "admin"); err != nil {
			return err
		}
		if method != "POST" {
			return fail(405, "POST erforderlich")
		}
		v, _ := s.Settings()
		if err := s.sendOperationalNotification(v, "Anker Testnachricht"); err != nil {
			return err
		}
		return jsonOut(w, map[string]bool{"ok": true})
	case "reindex":
		if err := require(u, "admin"); err != nil {
			return err
		}
		if method != "POST" {
			return fail(405, "POST erforderlich")
		}
		n, err := s.Reindex()
		if err != nil {
			return err
		}
		return jsonOut(w, map[string]int{"indexed": n})
	}
	if len(parts) < 3 {
		return fail(404, "Nicht gefunden")
	}
	id := parts[2]
	if !validID(id) {
		return fail(400, "Ungültige ID")
	}
	action := ""
	if len(parts) > 3 {
		action = parts[3]
	}
	switch parts[1] {
	case "hosts":
		h, err := s.Host(id)
		if err != nil {
			return fail(404, "Host nicht gefunden")
		}
		if method == "GET" {
			if u.Role != "admin" {
				h.KeyPath = ""
				h.RestoreKeyPath = ""
				h.KnownHostsPath = ""
			}
			return jsonOut(w, h)
		}
		if action == "backup" && method == "POST" {
			j, err := s.QueueBackup(id)
			if err != nil {
				return err
			}
			s.LogAudit(u.ID, "backup.start", id)
			return jsonOut(w, j)
		}
		if action == "probe" && method == "POST" {
			j, err := s.QueueProbe(id)
			if err != nil {
				return err
			}
			s.LogAudit(u.ID, "host.probe", id)
			return jsonOut(w, j)
		}
		if action == "" && method == "DELETE" {
			if err := require(u, "admin"); err != nil {
				return err
			}
			jobs, _ := s.Jobs()
			for _, j := range jobs {
				if j.HostID == id && (j.State == "running" || j.State == "queued") {
					return errors.New("Host hat aktive Aufträge")
				}
			}
			if err = s.Store.Delete("hosts", id); err != nil {
				return err
			}
			s.LogAudit(u.ID, "host.remove", id)
			return jsonOut(w, map[string]bool{"ok": true})
		}
	case "backups":
		b, err := s.Backup(id)
		if err != nil {
			return fail(404, "Sicherung nicht gefunden")
		}
		switch action {
		case "inventory":
			if method != "GET" {
				break
			}
			m, err := s.Manifest(id)
			if err != nil {
				return err
			}
			return jsonOut(w, m.Inventory)
		case "files":
			if method != "GET" {
				break
			}
			m, err := s.Manifest(id)
			if err != nil {
				return err
			}
			return jsonOut(w, m.Entries)
		case "file":
			if method != "GET" {
				break
			}
			data, e, err := s.ReadFile(id, r.URL.Query().Get("path"))
			if err != nil {
				return err
			}
			reveal := r.URL.Query().Get("reveal") == "1"
			if reveal {
				if err = require(u, "secrets"); err != nil {
					return err
				}
				s.LogAudit(u.ID, "file.reveal", id+":"+e.Path)
			}
			text := string(data)
			if e.Secret && !reveal {
				text = "[Verdeckter Inhalt · ausdrückliche Freigabe erforderlich]"
			}
			return jsonOut(w, map[string]any{"entry": e, "content": text, "masked": e.Secret && !reveal})
		case "verify":
			if method != "POST" {
				break
			}
			if err = s.VerifyBackup(id); err != nil {
				return err
			}
			s.LogAudit(u.ID, "backup.verify", id)
			return jsonOut(w, map[string]bool{"ok": true})
		case "pin":
			if method != "POST" {
				break
			}
			var in struct {
				Pinned bool `json:"pinned"`
			}
			if err = input(r, &in); err != nil {
				return err
			}
			lock := s.backupLock(id)
			lock.Lock()
			defer lock.Unlock()
			b, err = s.Backup(id)
			if err != nil {
				return err
			}
			b.Pinned = in.Pinned
			if err = s.Store.Put("backups", id, b); err != nil {
				return err
			}
			return jsonOut(w, b)
		case "archive":
			if method != "POST" {
				break
			}
			if err = s.ArchiveBackup(id); err != nil {
				return err
			}
			s.LogAudit(u.ID, "backup.archive", id)
			return jsonOut(w, map[string]bool{"ok": true})
		case "download":
			if method != "GET" {
				break
			}
			if err = require(u, "secrets"); err != nil {
				return err
			}
			if err = s.EnsureReadable(id); err != nil {
				return err
			}
			if err = s.VerifyBackup(id); err != nil {
				return err
			}
			if r.URL.Query().Get("check") == "1" {
				return jsonOut(w, map[string]bool{"ok": true})
			}
			s.LogAudit(u.ID, "backup.export", id)
			w.Header().Set("Content-Type", "application/x-tar")
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="anker-%s.tar"`, id))
			return s.ExportBackup(id, w)
		case "file-download":
			if method != "GET" {
				break
			}
			release, err := s.readableLease(id)
			if err != nil {
				return err
			}
			defer release()
			data, entry, err := s.readFileUnlocked(id, r.URL.Query().Get("path"), 64<<20)
			if err != nil {
				return err
			}
			if entry.Secret {
				if err = require(u, "secrets"); err != nil {
					return err
				}
			}
			if r.URL.Query().Get("check") == "1" {
				return jsonOut(w, map[string]bool{"ok": true})
			}
			if err = s.LogAudit(u.ID, "file.export", id+":"+entry.Path); err != nil {
				return err
			}
			name := filepath.Base(entry.Path)
			if entry.Type == "symlink" {
				name += ".symlink.txt"
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
			w.Header().Set("Content-Length", fmt.Sprint(len(data)))
			w.Header().Set("X-Anker-SHA256", entry.SHA256)
			_, err = w.Write(data)
			return err
		}
	case "plans":
		p, err := s.Plan(id)
		if err != nil {
			return fail(404, "Plan nicht gefunden")
		}
		if action == "" && method == "GET" {
			return jsonOut(w, redactPlan(p, false))
		}
		if action == "apply" && method == "POST" {
			var in struct {
				Confirmation string `json:"confirmation"`
			}
			if err = input(r, &in); err != nil {
				return err
			}
			j, err := s.QueueApply(id, in.Confirmation)
			if err != nil {
				return err
			}
			s.LogAudit(u.ID, "plan.apply", id)
			return jsonOut(w, j)
		}
		if action == "download" && method == "GET" {
			if err = require(u, "secrets"); err != nil {
				return err
			}
			if r.URL.Query().Get("check") == "1" {
				lock := s.planLock(id)
				lock.RLock()
				defer lock.RUnlock()
				if err = s.VerifyBackup(p.BackupID); err != nil {
					return err
				}
				if err = s.verifyPlanFiles(p); err != nil {
					return err
				}
				return jsonOut(w, map[string]bool{"ok": true})
			}
			s.LogAudit(u.ID, "plan.export", id)
			w.Header().Set("Content-Type", "application/x-tar")
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="anker-plan-%s.tar"`, id))
			return s.ExportPlan(id, w)
		}
	case "jobs":
		if action == "cancel" && method == "POST" {
			if err := s.CancelJob(id); err != nil {
				return err
			}
			s.LogAudit(u.ID, "job.cancel", id)
			return jsonOut(w, map[string]bool{"ok": true})
		}
	case "users":
		if err := require(u, "admin"); err != nil {
			return err
		}
		if action == "password" && method == "POST" {
			var in struct {
				Password string `json:"password"`
			}
			if err := input(r, &in); err != nil {
				return err
			}
			if err := a.SetPassword(id, in.Password); err != nil {
				return err
			}
			s.LogAudit(u.ID, "user.password", id)
			return jsonOut(w, map[string]bool{"ok": true})
		}
	}
	return fail(404, "Nicht gefunden")
}
