package client

import (
	"anker/internal/anker"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestCLIBackupReadinessReadsProjection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/backups/backup1/readiness" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"backup_id":"backup1","items":[{"id":"verification","state":"unknown","title":"Integrität","detail":"Ungeprüft"}],"coverage":[],"dependencies":[]}`))
	}))
	defer server.Close()
	var out bytes.Buffer
	c := &Client{HTTP: server.Client(), Base: server.URL, Out: &out}
	if err := c.Run(context.Background(), []string{"backup", "readiness", "backup1"}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte(`"state": "unknown"`)) {
		t.Fatal(out.String())
	}
}

func TestCLICreatesHostAndListsStatus(t *testing.T) {
	root := t.TempDir()
	store, err := anker.OpenStore(filepath.Join(root, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s, err := anker.NewService(root, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(anker.Handler(s, anker.NewAuth(store), true))
	defer server.Close()
	var out bytes.Buffer
	c := &Client{HTTP: server.Client(), Base: server.URL, Out: &out}
	if err = c.Run(context.Background(), []string{"host", "add", "--name", "pve-cli", "--address", "192.0.2.60"}); err != nil {
		t.Fatal(err)
	}
	hosts, _ := s.Hosts()
	if len(hosts) != 1 || hosts[0].Name != "pve-cli" {
		t.Fatal(hosts)
	}
	if err = c.Run(context.Background(), []string{"status"}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("pve-cli")) {
		t.Fatal(out.String())
	}
}
func TestUnknownCLICommandFails(t *testing.T) {
	c := &Client{}
	if c.Run(context.Background(), []string{"unknown"}) == nil {
		t.Fatal("unknown command succeeded")
	}
}

func TestCLITLSStatusUsesLocalAPIAndDemoRefusesChanges(t *testing.T) {
	root := t.TempDir()
	store, err := anker.OpenStore(filepath.Join(root, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s, err := anker.NewService(root, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Demo = true
	server := httptest.NewServer(anker.Handler(s, anker.NewAuth(store), true))
	defer server.Close()
	var out bytes.Buffer
	c := &Client{HTTP: server.Client(), Base: server.URL, Out: &out}
	if err := c.Run(context.Background(), []string{"tls", "status"}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte(`"enabled": false`)) {
		t.Fatal(out.String())
	}
	if err := c.Run(context.Background(), []string{"tls", "renew"}); err == nil {
		t.Fatal("demo certificate changed")
	}
	for _, args := range [][]string{{"tls", "auto", "maybe"}, {"tls", "auto", "on", "--days", "1"}, {"tls", "renew", "arbitrary"}} {
		if err := c.Run(context.Background(), args); err == nil {
			t.Fatal("invalid certificate command accepted", args)
		}
	}
}

func TestCLIReadsAndRemovesJobHistory(t *testing.T) {
	root := t.TempDir()
	store, err := anker.OpenStore(filepath.Join(root, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s, err := anker.NewService(root, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	job := anker.Job{ID: "finished-job", HostID: "removed-host", Kind: "backup", State: "failed"}
	if err := store.Put("jobs", job.ID, job); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(anker.Handler(s, anker.NewAuth(store), true))
	defer server.Close()
	var out bytes.Buffer
	c := &Client{HTTP: server.Client(), Base: server.URL, Out: &out}
	if err := c.Run(context.Background(), []string{"job", "show", job.ID}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte(job.ID)) {
		t.Fatal(out.String())
	}
	if err := c.Run(context.Background(), []string{"job", "remove", job.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Job(job.ID); err == nil {
		t.Fatal("history survived deletion")
	}
}

func TestCLIVerifiesCopyWithLocalPathAndRejectsExtraProof(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != "POST" || r.URL.Path != "/api/local-copy-verification/backup1" {
			t.Errorf("unexpected route %s %s", r.Method, r.URL.Path)
		}
		var in map[string]string
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Error(err)
		}
		if len(in) != 1 || in["copy_root"] != "/local/copy" {
			t.Errorf("unexpected proof input: %v", in)
		}
		json.NewEncoder(w).Encode(map[string]string{"state": "ok"})
	}))
	defer server.Close()
	var out bytes.Buffer
	c := &Client{HTTP: server.Client(), Base: server.URL, Out: &out}
	if err := c.Run(context.Background(), []string{"backup", "verify-copy", "backup1", "/local/copy"}); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"backup", "verify-copy", "backup1"}, {"backup", "verify-copy", "backup1", "relative"}, {"backup", "verify-copy", "backup1", "/local/copy", "2026-01-01"}} {
		if c.Run(context.Background(), args) == nil {
			t.Fatal("invalid local verification accepted", args)
		}
	}
	if requests != 1 {
		t.Fatal("invalid requests sent", requests)
	}
}
