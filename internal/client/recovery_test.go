package client

import (
	"anker/internal/anker"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIRecoveryInspectPlanStatusAndRollbackRoutes(t *testing.T) {
	paths := []string{}
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		json.NewDecoder(r.Body).Decode(&payload)
		json.NewEncoder(w).Encode(map[string]any{"ports": []any{}, "storage": []any{}, "blockers": []any{}})
	}))
	defer server.Close()
	c := &Client{HTTP: server.Client(), Base: server.URL}
	for _, args := range [][]string{{"restore", "inspect", "--backup", "b", "--target", "t", "--files", "etc/a.conf"}, {"restore", "plan", "--backup", "b", "--target", "t", "--files", "etc/a.conf", "--storage", "local=manual", "--hostname", "new", "--address", "192.0.2.4"}, {"restore", "status", "p"}, {"restore", "rollback", "p", "--confirm", "p"}} {
		if e := c.Run(context.Background(), args); e != nil {
			t.Fatal(e)
		}
	}
	want := []string{"POST /api/plans/inspect", "POST /api/plans/inspect", "POST /api/plans", "POST /api/plans/p/reconcile", "POST /api/plans/p/rollback"}
	if strings.Join(paths, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unsafe/missing route: %v", paths)
	}
	if payload["confirmation"] != "p" {
		t.Fatal("rollback omitted typed confirmation")
	}
}
func TestCLIRecoveryRollbackMissingConfirmationDoesNotContactHost(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; json.NewEncoder(w).Encode(map[string]any{}) }))
	defer server.Close()
	c := &Client{HTTP: server.Client(), Base: server.URL}
	for _, args := range [][]string{{"restore", "rollback", "p"}, {"restore", "rollback", "p", "--confirm", "wrong"}, {"restore", "status", "p", "extra"}} {
		if e := c.Run(context.Background(), args); e == nil {
			t.Fatalf("unsafe command accepted: %v", args)
		}
	}
	if calls != 0 {
		t.Fatal("invalid command reached host")
	}
}
func TestCLIInspectionUsesRealServiceWithoutCreatingPlan(t *testing.T) {
	root := t.TempDir()
	if e := anker.EnsureDataMode(root, true); e != nil {
		t.Fatal(e)
	}
	store, e := anker.OpenStore(filepath.Join(root, "catalog.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	s, e := anker.NewService(root, store, anker.DemoCollector{Root: root})
	if e != nil {
		t.Fatal(e)
	}
	s.Demo = true
	auth := anker.NewAuth(store)
	if e = s.SeedDemo(auth); e != nil {
		t.Fatal(e)
	}
	backups, _ := s.ListBackups("")
	hosts, _ := s.Hosts()
	server := httptest.NewServer(anker.Handler(s, auth, true))
	defer server.Close()
	var out bytes.Buffer
	c := &Client{HTTP: server.Client(), Base: server.URL, Out: &out}
	if e = c.Run(context.Background(), []string{"restore", "inspect", "--backup", backups[0].ID, "--target", hosts[0].ID, "--files", "etc/sysctl.d/99-anker.conf"}); e != nil {
		t.Fatal(e)
	}
	var i anker.RecoveryInspection
	if e = json.Unmarshal(out.Bytes(), &i); e != nil {
		t.Fatal(e)
	}
	if len(i.Ports) != 0 || len(i.Storage) != 0 {
		t.Fatalf("ordinary CLI inspection has unrelated requirements: %+v", i)
	}
	plans, _ := s.Plans()
	if len(plans) != 0 {
		t.Fatal("read-only CLI inspection created plans")
	}
}
