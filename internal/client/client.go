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
		case "plan":
			f := fs("restore plan")
			var p anker.PlanRequest
			files := f.String("files", "", "Kommaliste")
			ports := f.String("ports", "", "eno1=ens3,eno2=ens4")
			f.StringVar(&p.BackupID, "backup", "", "Backup-ID")
			f.StringVar(&p.TargetID, "target", "", "Ziel-ID")
			f.StringVar(&p.Scenario, "scenario", "files", "Szenario")
			f.BoolVar(&p.ConsoleConfirmed, "console", false, "Konsolenzugang bestätigt")
			f.BoolVar(&p.SourceOffline, "source-offline", false, "Quelle isoliert")
			if err := f.Parse(args[2:]); err != nil {
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
			return call("POST", "plans", p)
		}
	case "jobs":
		return call("GET", "status", nil)
	case "job":
		if err := need(3); err != nil {
			return err
		}
		if args[1] == "cancel" {
			return call("POST", "jobs/"+args[2]+"/cancel", map[string]any{})
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

const Help = `Anker — Hostkonfigurationen sichern und wiederherstellen

anker status | doctor | tui
anker host list | probe HOST | remove HOST
anker host add --name NAME --address IP --key /PFAD --known-hosts /PFAD
anker backup run HOST | list | files BACKUP | verify BACKUP | archive BACKUP | pin BACKUP
anker diff ALT NEU
anker export BACKUP ./sicherung.tar
anker restore list
anker restore plan --backup ID --target HOST --scenario files --files etc/test.conf
anker restore plan --backup ID --target HOST --scenario migration --ports eno1=ens3 --console --source-offline
anker restore apply PLAN --confirm PLAN
anker restore export PLAN ./plan.tar
anker job cancel ID
anker settings show | save DATEI.json
anker user list | add NAME ROLE | password NAME (ANKER_USER_PASSWORD setzen)
anker reindex

Globale Optionen vor dem Befehl: --data /srv/anker --socket /srv/anker/anker.sock`
