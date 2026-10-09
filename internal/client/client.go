package client

import (
	"anker/internal/anker"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Client struct {
	HTTP *http.Client
	Base string
	Out  io.Writer
}

// ParseRecoveryDecisions validates explicit adapter input without deciding
// whether a file may be applied; the service owns that policy.
func ParseRecoveryDecisions(values []string) (map[string]anker.FileDecision, error) {
	out := map[string]anker.FileDecision{}
	for _, value := range values {
		pair := strings.Split(value, "=")
		if len(pair) != 2 {
			return nil, errors.New("Dateientscheidung als PATH=backup|prepared|keep|manual angeben")
		}
		path := strings.TrimSpace(pair[0])
		if path == "" {
			return nil, errors.New("Dateientscheidung benötigt einen Pfad")
		}
		if err := anker.ValidPath(path); err != nil {
			return nil, err
		}
		if _, exists := out[path]; exists {
			return nil, fmt.Errorf("Dateientscheidung für %s mehrfach angegeben", path)
		}
		choice := anker.FileDecision(strings.TrimSpace(pair[1]))
		switch choice {
		case anker.DecisionBackup, anker.DecisionPrepared, anker.DecisionKeep, anker.DecisionManual:
		default:
			return nil, errors.New("Dateientscheidung muss backup, prepared, keep oder manual sein")
		}
		out[path] = choice
	}
	return out, nil
}

func New(socket string) *Client {
	return &Client{HTTP: &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}, Timeout: 11 * time.Minute}, Base: "http://anker.local", Out: os.Stdout}
}
func (c *Client) Call(ctx context.Context, method, path string, in, out any) error {
	var reader io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	r, err := http.NewRequestWithContext(ctx, method, c.Base+"/api/"+path, reader)
	if err != nil {
		return err
	}
	r.Header.Set("X-Anker-Request", "1")
	r.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(r)
	if err != nil {
		return fmt.Errorf("Anker-Dienst nicht erreichbar: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var v struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&v)
		return fmt.Errorf("%s (HTTP %d)", v.Error, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(out)
}
func (c *Client) Download(ctx context.Context, path, output string) error {
	r, err := http.NewRequestWithContext(ctx, "GET", c.Base+"/api/"+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("Export fehlgeschlagen (HTTP %d)", resp.StatusCode)
	}
	if err = os.MkdirAll(filepath.Dir(output), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(output), ".anker-export-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	f.Chmod(0600)
	_, err = io.Copy(f, resp.Body)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(f.Name(), output)
	}
	return err
}
func (c *Client) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New(Help)
	}
	out := c.Out
	if out == nil {
		out = io.Discard
	}
	var result any
	call := func(method, path string, in any) error {
		if err := c.Call(ctx, method, path, in, &result); err != nil {
			return err
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}
	fs := func(name string) *flag.FlagSet {
		f := flag.NewFlagSet(name, flag.ContinueOnError)
		f.SetOutput(io.Discard)
		return f
	}
	need := func(n int) error {
		if len(args) < n {
			return errors.New("Argument fehlt; anker help zeigt Befehle")
		}
		return nil
	}
	switch args[0] {
	case "help":
		fmt.Fprintln(out, Help)
		return nil
	case "status":
		return call("GET", "status", nil)
	case "doctor":
		return call("GET", "doctor", nil)
	case "storage":
		if len(args) == 2 && args[1] == "status" {
			return call("GET", "storage", nil)
		}
		if len(args) == 2 && args[1] == "state" {
			return call("GET", "storage/state", nil)
		}
		if len(args) == 3 && args[1] == "plan" {
			return call("POST", "storage/plan", map[string]string{"volume_id": args[2]})
		}
		if len(args) >= 3 && args[1] == "grow" {
			flags := fs("storage grow")
			plan := flags.String("plan", "", "Geprüfte Plan-ID")
			confirmation := flags.String("confirm", "", "Mountpoint bestätigen")
			if err := flags.Parse(args[3:]); err != nil {
				return err
			}
			if flags.NArg() != 0 || *plan == "" || *confirmation == "" {
				return errors.New("Zuerst anker storage plan VOLUME verwenden; --plan ID und --confirm MOUNT sind erforderlich")
			}
			return call("POST", "storage/grow", map[string]string{"volume_id": args[2], "plan_id": *plan, "confirmation": *confirmation})
		}
		return errors.New("anker storage status | state | plan VOLUME | grow VOLUME --plan ID --confirm MOUNT")
	case "tls":
		if len(args) == 2 && args[1] == "status" {
			return call("GET", "tls", nil)
		}
		if len(args) == 2 && args[1] == "renew" {
			return call("POST", "tls/renew", struct{}{})
		}
		if len(args) == 3 && args[1] == "certificate" {
			return c.Download(ctx, "tls/certificate", args[2])
		}
		if len(args) >= 3 && args[1] == "auto" && (args[2] == "on" || args[2] == "off") {
			flags := fs("tls auto")
			days := flags.Int("days", 30, "Erneuerungsvorlauf in Tagen")
			if err := flags.Parse(args[3:]); err != nil {
				return err
			}
			if flags.NArg() != 0 || *days < 7 || *days > 90 {
				return errors.New("Vorlauf zwischen 7 und 90 Tagen angeben")
			}
			return call("POST", "tls", map[string]any{"automatic": args[2] == "on", "renew_before_days": *days})
		}
		return errors.New("anker tls status | renew | auto on/off [--days 30] | certificate DATEI.crt")
	case "host":
		if err := need(2); err != nil {
			return err
		}
		switch args[1] {
		case "list":
			return call("GET", "hosts", nil)
		case "probe":
			if err := need(3); err != nil {
				return err
			}
			return call("POST", "hosts/"+args[2]+"/probe", map[string]any{})
		case "remove":
			if err := need(3); err != nil {
				return err
			}
			return call("DELETE", "hosts/"+args[2], nil)
		case "add":
			f := fs("host add")
			var h anker.Host
			f.StringVar(&h.Name, "name", "", "Hostname")
			f.StringVar(&h.Address, "address", "", "Adresse")
			f.StringVar(&h.ID, "id", "", "ID")
			f.StringVar(&h.Group, "group", "", "Gruppe")
			f.StringVar(&h.ClusterID, "cluster", "", "Cluster")
			f.StringVar(&h.SSHUser, "user", "anker", "SSH-Benutzer")
			f.IntVar(&h.SSHPort, "port", 22, "SSH-Port")
			f.StringVar(&h.KeyPath, "key", "", "privater Keypfad")
			f.StringVar(&h.RestoreKeyPath, "restore-key", "", "separater Wiederherstellungsschlüssel")
			f.StringVar(&h.RestoreSSHUser, "restore-user", "anker-restore", "separater Wiederherstellungsbenutzer")
			f.StringVar(&h.KnownHostsPath, "known-hosts", "", "Known-Hosts-Pfad")
			f.StringVar(&h.Schedule, "schedule", "", "HH:MM")
			f.BoolVar(&h.Enabled, "enabled", true, "Zeitplan aktiv")
			if err := f.Parse(args[2:]); err != nil {
				return err
			}
			return call("POST", "hosts", h)
		}
	case "backup":
		if err := need(2); err != nil {
			return err
		}
		if args[1] == "list" {
			if err := c.Call(ctx, "GET", "status", nil, &result); err != nil {
				return err
			}
			v := result.(map[string]any)["backups"]
			return json.NewEncoder(out).Encode(v)
		}
		if err := need(3); err != nil {
			return err
		}
		switch args[1] {
		case "reference":
			if len(args) != 3 {
				return errors.New("backup reference HOST")
			}
			return call("GET", "hosts/"+url.PathEscape(args[2])+"/reference", nil)
		case "compare-reference":
			if len(args) != 4 {
				return errors.New("backup compare-reference HOST BACKUP")
			}
			return call("GET", "hosts/"+url.PathEscape(args[2])+"/reference/compare?to="+url.QueryEscape(args[3]), nil)
		case "reference-clear-drill":
			if len(args) != 3 {
				return errors.New("backup reference-clear-drill HOST")
			}
			return call("DELETE", "hosts/"+url.PathEscape(args[2])+"/reference/drill", nil)
		case "reference-set":
			if len(args) < 4 {
				return errors.New("backup reference-set HOST BACKUP --name NAME [--reason GRUND] [--tested-at RFC3339 --outcome passed|failed --evidence TEXT]")
			}
			f := flag.NewFlagSet("backup reference-set", flag.ContinueOnError)
			name, reason, testedAt, outcome, evidence := "", "", "", "", ""
			f.StringVar(&name, "name", "", "Referenzname")
			f.StringVar(&reason, "reason", "", "Grund")
			f.StringVar(&testedAt, "tested-at", "", "Zeit der externen Probe (RFC3339)")
			f.StringVar(&outcome, "outcome", "", "passed oder failed laut Bericht")
			f.StringVar(&evidence, "evidence", "", "Menschlicher Beleg")
			if err := f.Parse(args[4:]); err != nil {
				return err
			}
			if name == "" || f.NArg() != 0 {
				return errors.New("Referenzname erforderlich")
			}
			payload := map[string]any{"backup_id": args[3], "name": name, "reason": reason}
			if testedAt != "" || outcome != "" || evidence != "" {
				if testedAt == "" || outcome == "" || evidence == "" {
					return errors.New("Offline-Probe benötigt Datum, Ergebnis und Beleg")
				}
				payload["drill"] = map[string]string{"tested_at": testedAt, "outcome": outcome, "evidence": evidence}
			}
			return call("PUT", "hosts/"+url.PathEscape(args[2])+"/reference", payload)
		case "verify-copy":
			if len(args) != 4 || !filepath.IsAbs(args[3]) {
				return fmt.Errorf("Kopie prüfen: anker backup verify-copy BACKUP /absoluter/kopiepfad")
			}
			return call("POST", "local-copy-verification/"+url.PathEscape(args[2]), map[string]string{"copy_root": args[3]})
		case "readiness":
			return call("GET", "backups/"+args[2]+"/readiness", nil)
		case "run":
			return call("POST", "hosts/"+args[2]+"/backup", map[string]any{})
		case "verify":
			return call("POST", "backups/"+args[2]+"/verify", map[string]any{})
		case "archive":
			return call("POST", "backups/"+args[2]+"/archive", map[string]any{})
		case "files":
			return call("GET", "backups/"+args[2]+"/files", nil)
		case "pin":
			return call("POST", "backups/"+args[2]+"/pin", map[string]bool{"pinned": true})
		}
	case "diff":
		if err := need(3); err != nil {
			return err
		}
		return call("POST", "backups/diff", map[string]string{"from": args[1], "to": args[2]})
	case "export":
		if err := need(3); err != nil {
			return err
		}
		if err := c.Download(ctx, "backups/"+args[1]+"/download", args[2]); err != nil {
			return err
		}
		fmt.Fprintln(out, "Export gespeichert:", args[2])
		return nil
	case "restore":
		if err := need(2); err != nil {
			return err
		}
		switch args[1] {
		case "list":
			return call("GET", "plans", nil)
		case "export":
			if err := need(4); err != nil {
				return err
			}
			return c.Download(ctx, "plans/"+args[2]+"/download", args[3])
		case "apply":
			f := fs("restore apply")
			confirmation := f.String("confirm", "", "Plan-ID bestätigen")
			if len(args) < 3 {
				return errors.New("Plan-ID fehlt")
			}
			if err := f.Parse(args[3:]); err != nil {
				return err
			}
			return call("POST", "plans/"+args[2]+"/apply", map[string]string{"confirmation": *confirmation})
		case "verify":
			if len(args) != 3 {
				return errors.New("anker restore verify PLAN")
			}
			return call("POST", "plans/"+url.PathEscape(args[2])+"/verify", map[string]any{})
		case "status", "reconcile":
			if len(args) != 3 {
				return errors.New("anker restore status PLAN")
			}
			return call("POST", "plans/"+args[2]+"/reconcile", map[string]any{})
		case "compare", "check":
			if len(args) < 3 || args[2] == "" {
				return errors.New("Plan-ID fehlt")
			}
			f := fs("restore " + args[1])
			if args[1] == "compare" {
				path := f.String("path", "", "Ausgewählter relativer Dateipfad")
				basis := f.String("basis", "target", "target|source|source-target")
				if err := f.Parse(args[3:]); err != nil {
					return err
				}
				if f.NArg() != 0 || *path == "" {
					return errors.New("restore compare PLAN --path PATH --basis target|source|source-target")
				}
				if err := anker.ValidPath(*path); err != nil {
					return err
				}
				if *basis != "target" && *basis != "source" && *basis != "source-target" {
					return errors.New("Vergleichsbasis muss target, source oder source-target sein")
				}
				query := url.Values{"path": {*path}, "basis": {*basis}}
				return call("GET", "plans/"+url.PathEscape(args[2])+"/comparison?"+query.Encode(), nil)
			}
			task := f.String("task", "", "Aufgaben-ID aus dem gespeicherten Plan")
			evidence := f.String("evidence", "", "Konkretes Prüfergebnis; Benutzerbestätigung, kein maschineller Nachweis")
			if err := f.Parse(args[3:]); err != nil {
				return err
			}
			if f.NArg() != 0 || strings.TrimSpace(*task) == "" || strings.TrimSpace(*evidence) == "" || len(*evidence) > 4096 {
				return errors.New("restore check PLAN --task ID --evidence TEXT (1–4096 Bytes) erforderlich")
			}
			return call("POST", "plans/"+url.PathEscape(args[2])+"/checks/"+url.PathEscape(*task), map[string]string{"evidence": *evidence})
		case "rollback":
			if len(args) < 3 {
				return errors.New("Plan-ID fehlt")
			}
			f := fs("restore rollback")
			confirmation := f.String("confirm", "", "Plan-ID ausdrücklich zur Rücksetzung bestätigen")
			if err := f.Parse(args[3:]); err != nil {
				return err
			}
			if f.NArg() != 0 || *confirmation != args[2] {
				return errors.New("anker restore rollback PLAN --confirm PLAN erforderlich")
			}
			return call("POST", "plans/"+args[2]+"/rollback", map[string]string{"confirmation": *confirmation})
		case "recipe-list":
			if len(args) != 2 {
				return errors.New("anker restore recipe-list")
			}
			return call("GET", "recovery-recipes", nil)
		case "recipe-save":
			if len(args) < 3 {
				return errors.New("Plan-ID fehlt")
			}
			f := fs("restore recipe-save")
			label := f.String("label", "", "Bezeichnung der Zuordnungsvorlage")
			reviewed := f.Bool("reviewed", false, "Gespeicherte Planzuordnungen ausdrücklich geprüft")
			if err := f.Parse(args[3:]); err != nil {
				return err
			}
			if f.NArg() != 0 || strings.TrimSpace(*label) == "" || !*reviewed {
				return errors.New("recipe-save PLAN --label TEXT --reviewed erforderlich")
			}
			return call("POST", "recovery-recipes", map[string]any{"plan_id": args[2], "label": *label, "reviewed": true})
		case "plan", "inspect", "recipe-preview":
			f := fs("restore " + args[1])
			recipe := f.String("recipe", "", "Zuordnungsvorlage frisch prüfen")
			var p anker.PlanRequest
			var decisions, network []string
			f.Func("network", "STANZA/FAMILIE=ADRESSE/CIDR|GATEWAY|DNS,DNS; wiederholbar", func(value string) error { network = append(network, value); return nil })
			f.Func("decision", "PATH=backup|prepared|keep|manual; je Pfad einmal wiederholbar, ohne Auswahl manuell", func(value string) error { decisions = append(decisions, value); return nil })
			files := f.String("files", "", "Kommaliste")
			ports := f.String("ports", "", "eno1=ens3,eno2=ens4")
			storage := f.String("storage", "", "Storage-ID=Ziel-ID oder Storage-ID=manual; manuelle Entscheidung")
			f.StringVar(&p.Mapping.Hostname, "hostname", "", "Gewünschte Hostidentität; manuelle Entscheidung")
			f.StringVar(&p.Mapping.Address, "address", "", "Gewünschte Adresse; manuelle Entscheidung")
			f.StringVar(&p.BackupID, "backup", "", "Backup-ID")
			f.StringVar(&p.TargetID, "target", "", "Ziel-ID")
			f.StringVar(&p.Scenario, "scenario", "files", "files|host|cluster-node|cluster-disaster; Legacy-Szenarien bleiben lesbar")
			f.BoolVar(&p.ConsoleConfirmed, "console", false, "Konsolenzugang bestätigt")
			f.BoolVar(&p.SourceOffline, "source-offline", false, "Quelle isoliert")
			if err := f.Parse(args[2:]); err != nil {
				return err
			}
			if f.NArg() != 0 {
				return errors.New("Unerwartete Planargumente")
			}
			var err error
			p.Mapping.Network, err = ParseRecoveryNetwork(network)
			if err != nil {
				return err
			}
			p.Decisions, err = ParseRecoveryDecisions(decisions)
			if err != nil {
				return err
			}
			if *files != "" {
				p.Files = strings.Split(*files, ",")
			}
			p.Mapping.Interfaces = map[string]string{}
			for _, pair := range strings.Split(*ports, ",") {
				if pair == "" {
					continue
				}
				v := strings.SplitN(pair, "=", 2)
				if len(v) != 2 {
					return errors.New("Portzuordnung muss alt=neu sein")
				}
				p.Mapping.Interfaces[v[0]] = v[1]
			}
			p.Mapping.Storage = map[string]string{}
			for _, pair := range strings.Split(*storage, ",") {
				if pair == "" {
					continue
				}
				v := strings.SplitN(pair, "=", 2)
				if len(v) != 2 || strings.TrimSpace(v[0]) == "" || strings.TrimSpace(v[1]) == "" {
					return errors.New("Storage-Entscheidung muss ID=Ziel oder ID=manual sein")
				}
				p.Mapping.Storage[strings.TrimSpace(v[0])] = strings.TrimSpace(v[1])
			}
			if args[1] == "inspect" {
				return call("POST", "plans/inspect", p)
			}
			if args[1] == "recipe-preview" && *recipe == "" {
				return errors.New("--recipe ID erforderlich")
			}
			var inspection anker.RecoveryInspection
			if err := c.Call(ctx, "POST", "plans/inspect", p, &inspection); err != nil {
				return err
			}
			p.SnapshotID = inspection.SnapshotID
			if args[1] == "recipe-preview" {
				return call("POST", "recovery-recipes/preview", map[string]any{"recipe_id": *recipe, "request": p})
			}
			return call("POST", "plans", p)
		}
	case "jobs":
		return call("GET", "status", nil)
	case "job":
		if err := need(3); err != nil {
			return err
		}
		switch args[1] {
		case "show":
			return call("GET", "jobs/"+args[2], nil)
		case "remove":
			return call("DELETE", "jobs/"+args[2], nil)
		case "cancel", "retry":
			return call("POST", "jobs/"+args[2]+"/"+args[1], map[string]any{})
		}
	case "settings":
		if len(args) == 1 || args[1] == "show" {
			return call("GET", "settings", nil)
		}
		if args[1] == "save" {
			if err := need(3); err != nil {
				return err
			}
			b, err := os.ReadFile(args[2])
			if err != nil {
				return err
			}
			var v anker.Settings
			if err = json.Unmarshal(b, &v); err != nil {
				return err
			}
			return call("PUT", "settings", v)
		}
	case "user":
		if err := need(2); err != nil {
			return err
		}
		if args[1] == "list" {
			return call("GET", "users", nil)
		}
		if args[1] == "add" {
			if err := need(4); err != nil {
				return err
			}
			return call("POST", "users", map[string]any{"name": args[2], "role": args[3], "password": os.Getenv("ANKER_USER_PASSWORD"), "secrets": false})
		}
		if args[1] == "remove" || args[1] == "revoke" || args[1] == "sessions" {
			if err := need(3); err != nil {
				return err
			}
			path := "users/" + args[2]
			method := "DELETE"
			if args[1] != "remove" {
				path += "/sessions"
			}
			if args[1] == "sessions" {
				method = "GET"
			}
			return call(method, path, nil)
		}
		if args[1] == "disable" || args[1] == "enable" || args[1] == "role" {
			if err := need(3); err != nil {
				return err
			}
			var users []anker.User
			if err := c.Call(ctx, "GET", "users", nil, &users); err != nil {
				return err
			}
			for _, u := range users {
				if u.ID == args[2] {
					if args[1] == "role" {
						if err := need(4); err != nil {
							return err
						}
						u.Role = args[3]
					} else {
						u.Disabled = args[1] == "disable"
					}
					return call("PUT", "users/"+u.ID, map[string]any{"role": u.Role, "secrets": u.Secrets, "disabled": u.Disabled})
				}
			}
			return errors.New("Benutzer nicht gefunden")
		}
		if args[1] == "password" {
			if err := need(3); err != nil {
				return err
			}
			return call("POST", "users/"+args[2]+"/password", map[string]string{"password": os.Getenv("ANKER_USER_PASSWORD")})
		}
	case "reindex":
		return call("POST", "reindex", map[string]any{})
	}
	return errors.New("Unbekannter Befehl.\n" + Help)
}

const Help = `Anker – Konfigurationen über SSH sichern und wiederherstellen

Start und Hilfe
  sudo anker              Geführte Verwaltung im Terminal (auch: anker tui)
  anker -help             Diese Kurzanleitung; auch -h, --help oder help
  man anker               Handbuch; /WORT sucht, n sucht weiter, q beendet
  anker version           Installierte Version anzeigen
  Ohne Terminal zeigt anker die Hilfe. Verwaltungsbefehle benötigen den Dienst.
  Offline: anker plan-package verify --input PLAN.tar (ohne Dienst; interne Prüfsummen)
  Offline: anker --data DATEN disaster export --output PAKET --passphrase-file DATEI
  Offline: anker disaster inspect --input PAKET --passphrase-file DATEI
  Offline: anker disaster import --input PAKET --destination NEU --passphrase-file DATEI
  Disaster-Export: Dienst/Updater vorher stoppen; optionale --config-root /etc/anker.

Installation und Einrichtung (Linux mit systemd)
  sudo ./scripts/install-server.sh       Aus Checkout bauen oder Paket installieren
  sudo anker setup                      Zugang, SSH-Schlüssel und HTTPS einrichten
  Nach git pull denselben Installer erneut ausführen. Er prüft den neuen Start
  und fällt bei Fehlern zurück. --no-setup lässt den Einrichtungsdialog aus.
  Web: eingerichtete HTTPS-Adresse öffnen; lokal zunächst 127.0.0.1:8087.

Terminal bedienen
  Tab oder 1–5 wechseln Hosts, Sicherungen, Wiederherstellung, Aufträge,
  Einstellungen. Pfeile wählen, Enter öffnet, Esc geht zurück, q beendet.
  Aktionen und Formulare zeigen ihre Tasten an; Änderungen vorher prüfen.
  Hosts: a bindet automatisch an, m legt manuell an, v verbindet erneut.
  h prüft SSH-Fingerprints. System → Speicher: g plant Erweiterungen.

Hosts und Sicherungen
  anker status | doctor
  anker host list | probe HOST | remove HOST
  anker host trust ADRESSE --fingerprint SHA256:... [--port 22]
  anker host add --name NAME --address IP [--key /PFAD --known-hosts /PFAD]
  anker backup verify-copy BACKUP /absoluter/kopiepfad (lokal; entpackte Ablage)
  anker backup run HOST | list | files BACKUP | readiness BACKUP | verify BACKUP | archive BACKUP | pin BACKUP
  anker backup reference HOST | reference-set HOST BACKUP --name NAME [--reason GRUND]
  anker backup reference-set HOST BACKUP --name NAME --tested-at RFC3339 --outcome passed|failed --evidence TEXT
  anker backup reference-clear-drill HOST | compare-reference HOST BACKUP
  anker diff ALT NEU
  anker export BACKUP ./sicherung.tar
  Im Web/Terminal Adresse, SSH-Benutzer und einmaliges Passwort eingeben.
  Fingerprint unabhängig prüfen; Helfer und beide Schlüssel werden eingerichtet.

Wiederherstellung und Aufträge
  anker restore list
  anker restore recipe-list
  anker restore recipe-save PLAN --label TEXT --reviewed
  anker restore recipe-preview --recipe ID --backup ID --target HOST --files etc/hostname
  anker restore inspect --backup ID --target HOST --scenario files --files etc/test.conf
  anker restore plan --backup ID --target HOST --scenario files --files etc/test.conf --decision etc/test.conf=backup
  anker restore plan --backup ID --target HOST --scenario host --ports eno1=ens3 --console --source-offline
  anker restore compare PLAN --path PATH --basis target|source|source-target
  anker restore check PLAN --task ID --evidence TEXT
  anker restore apply PLAN --confirm PLAN
  anker restore status PLAN                 Hostprotokoll abgleichen, keine Übernahme wiederholen
  anker restore rollback PLAN --confirm PLAN Kontrollierte Rücksetzung nach Hostprüfung
  anker restore export PLAN ./plan.tar
  anker jobs
  anker job show ID | cancel ID | retry ID | remove ID
  Situationen: files, host, cluster-node, cluster-disaster; alte Szenarien bleiben lesbar.
  Ziel, Dateien und Plan prüfen; Hostwechsel braucht Konsole und isolierte Quelle.
  anker restore verify PLAN: frische lesende Maschinenprüfung und getrennte Versuchshistorie.
  Gesamtrecovery, neue Hardware und Cluster: auf echten Hosts manuell geführter Plan.
  Anleitung und Originaldateien mit restore export herunterladen.
  Plan bindet die frische Zielaufnahme; compare liest gespeicherte Belege ohne Schreibauftrag.
  --decision PATH=backup|prepared|keep|manual je Pfad einmal; ohne Auswahl bleibt die Datei manuell.
  check dokumentiert einen Benutzerbeleg mit Dienstidentität; kein maschineller Neustartnachweis.
  --storage ID=manual, --hostname NAME und --address IP dokumentieren manuelle Entscheidungen.
  Netzwerkdateien sind vorbereitet; Aktivierung bleibt manuell über Konsole.
  Wiederherstellungen werden nicht automatisch wiederholt.

Einstellungen, Benutzer, Zertifikat und Speicher
  anker settings show | save DATEI.json
  anker user list | add NAME ROLE | password NAME (ANKER_USER_PASSWORD setzen)
  anker user disable NAME | enable NAME | role NAME ROLE | sessions NAME | revoke NAME | remove NAME
  anker tls status | renew | auto on/off [--days 30] | certificate DATEI.crt
  anker storage status | state | plan VOLUME | grow VOLUME --plan ID --confirm MOUNT
  anker reindex
  Zeitplan und Aufbewahrung im Interface einstellen. Speichererweiterung erst
  nach neuem Plan und Mountbestätigung; Laufwerke werden nicht formatiert.

Updates
  Web: Einstellungen → System → Updates; Quelle einrichten, prüfen, bestätigen.
  sudo anker update status | check | install
  Releases brauchen eine gültige Signatur und passende Prüfsummen. Eigene Quellen
  brauchen einen unabhängig geprüften Schlüssel. Aktive Aufträge sperren Updates.

Globale Optionen stehen vor dem Befehl:
  --data /srv/anker  --socket /srv/anker/anker.sock
  --listen 127.0.0.1:8087  --tls-cert DATEI  --tls-key DATEI
  --admin NAME (init)  --allow-http (HTTP außerhalb Loopback ausdrücklich erlauben)
  anker init | serve | demo   Manuelle Initialisierung, Dienst oder lokale Demo
  Betrieb und Rückfall: docs/OPERATIONS.md, docs/RECOVERY.md, docs/UPDATES.md
  Lokale Verwaltung über den geschützten Unix-Socket als root/Dienstbenutzer.`
