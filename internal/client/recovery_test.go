package client

import (
	"anker/internal/anker"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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

// Catches lost snapshot binding, implicit/ambiguous decisions and a comparison
// accidentally submitted as a mutation.
func TestCLIRecoveryComparisonAndDecisionParsing(t *testing.T) {
	var requests []string
	var saved anker.PlanRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.RequestURI())
		if r.URL.Path == "/api/plans/inspect" {
			json.NewEncoder(w).Encode(anker.RecoveryInspection{SnapshotID: "captured-target"})
			return
		}
		if r.URL.Path == "/api/plans" {
			json.NewDecoder(r.Body).Decode(&saved)
		}
		json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer server.Close()
	c := &Client{HTTP: server.Client(), Base: server.URL, Out: &bytes.Buffer{}}
	base := []string{"restore", "plan", "--backup", "b", "--target", "t", "--files", "etc/a.conf,etc/b.conf"}
	args := append(append([]string{}, base...), "--decision", "etc/a.conf=backup", "--decision", "etc/b.conf=keep")
	if err := c.Run(t.Context(), args); err != nil {
		t.Fatal(err)
	}
	if saved.SnapshotID != "captured-target" || saved.Decisions["etc/a.conf"] != anker.DecisionBackup || saved.Decisions["etc/b.conf"] != anker.DecisionKeep {
		t.Fatalf("review choices/evidence lost: %+v", saved)
	}
	for _, basis := range []string{"target", "source", "source-target"} {
		if err := c.Run(t.Context(), []string{"restore", "compare", "p", "--path", "etc/a.conf", "--basis", basis}); err != nil {
			t.Fatal(err)
		}
		want := "GET /api/plans/p/comparison?basis=" + basis + "&path=etc%2Fa.conf"
		if requests[len(requests)-1] != want {
			t.Fatal(requests)
		}
	}
	before := len(requests)
	for _, flags := range [][]string{{"--decision", ""}, {"--decision", "etc/a.conf="}, {"--decision", "=backup"}, {"--decision", "etc/a.conf=other"}, {"--decision", "etc/a.conf=backup=keep"}, {"--decision", "etc/a.conf=backup", "--decision", "etc/a.conf=keep"}, {"--decision", "../etc/a=backup"}} {
		if err := c.Run(t.Context(), append(append([]string{}, base...), flags...)); err == nil {
			t.Fatalf("ambiguous decision accepted: %v", flags)
		}
	}
	for _, args := range [][]string{{"restore", "compare", "p"}, {"restore", "compare", "p", "--path", "etc/a", "--basis", "live"}, {"restore", "check", "p", "--task", "review", "--evidence", " "}, {"restore", "check", "p", "--task", "review", "--evidence", "ok", "--actor", "spoof"}} {
		if err := c.Run(t.Context(), args); err == nil {
			t.Fatalf("invalid command accepted: %v", args)
		}
	}
	if len(requests) != before {
		t.Fatal("invalid input contacted service")
	}
}

func TestCLIManualCheckNotRebootProof(t *testing.T) {
	root := t.TempDir()
	if err := anker.EnsureDataMode(root, true); err != nil {
		t.Fatal(err)
	}
	store, err := anker.OpenStore(filepath.Join(root, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s, err := anker.NewService(root, store, anker.DemoCollector{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	s.Demo = true
	auth := anker.NewAuth(store)
	if err := s.SeedDemo(auth); err != nil {
		t.Fatal(err)
	}
	backups, _ := s.ListBackups("")
	hosts, _ := s.Hosts()
	p, err := s.CreatePlan(t.Context(), anker.PlanRequest{BackupID: backups[0].ID, TargetID: hosts[0].ID, Scenario: "files", Files: []string{"etc/sysctl.d/99-anker.conf"}})
	if err != nil {
		t.Fatal(err)
	}
	p.State, p.Result = "checks_pending", &anker.ApplyResult{State: "applied"}
	p.Tasks = []anker.RecoveryTask{{ID: "check-reboot", Kind: "manual-check", State: "pending"}}
	data, _ := json.Marshal(p)
	if err := os.WriteFile(filepath.Join(root, "plans", p.ID, "plan.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.Put("plans", p.ID, p); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(anker.Handler(s, auth, true))
	defer server.Close()
	c := &Client{HTTP: server.Client(), Base: server.URL, Out: &bytes.Buffer{}}
	if err := c.Run(t.Context(), []string{"restore", "check", p.ID, "--task", "check-reboot", "--evidence", "Neustart an der Konsole geprüft"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Plan(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "checks_pending" || got.Result.RebootVerified || got.Tasks[0].State != "user_confirmed" || got.Tasks[0].ConfirmedBy != "local" || got.Tasks[0].ConfirmedAt == "" {
		t.Fatalf("manual check became machine proof or lost trusted actor: %+v", got)
	}
}

func TestCLISelectedNetworkStanzaPayload(t *testing.T) {
	var payload anker.PlanRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&payload)
		json.NewEncoder(w).Encode(map[string]any{"snapshot_id": "bound"})
	}))
	defer server.Close()
	c := &Client{HTTP: server.Client(), Base: server.URL}
	args := []string{"restore", "plan", "--backup", "b", "--target", "t", "--files", "etc/network/interfaces", "--decision", "etc/network/interfaces=prepared", "--network", "vmbr0/inet=198.51.100.9/24|198.51.100.1|192.0.2.53,2001:db8::53"}
	if err := c.Run(t.Context(), args); err != nil {
		t.Fatal(err)
	}
	v := payload.Mapping.Network["vmbr0/inet"]
	if payload.SnapshotID != "bound" || v.AddressCIDR != "198.51.100.9/24" || len(v.DNSServers) != 2 {
		t.Fatalf("typed network choice lost: %+v", payload)
	}
}

func TestRelevantRecoveryMappingRetainsSharedManualReferences(t *testing.T) {
	i := anker.RecoveryInspection{Ports: []anker.RecoveryPort{{Name: "eno1", Paths: []string{"kept", "manual"}}, {Name: "eno2", Paths: []string{"kept"}}}, Storage: []anker.RecoveryStorage{{ID: "shared", Paths: []string{"kept", "manual"}}, {ID: "only-kept", Paths: []string{"kept"}}}}
	m := RelevantRecoveryMapping(anker.Mapping{Interfaces: map[string]string{"eno1": "ens3", "eno2": "ens4"}, Storage: map[string]string{"shared": "manual", "only-kept": "manual"}}, i, map[string]anker.FileDecision{"kept": anker.DecisionKeep, "manual": anker.DecisionManual})
	if len(m.Interfaces) != 1 || m.Interfaces["eno1"] != "ens3" || len(m.Storage) != 1 || m.Storage["shared"] != "manual" {
		t.Fatalf("shared manual requirement lost: %+v", m)
	}
	for _, invalid := range []string{"vmbr0=1|2|3", "vmbr0/inet=2001:db8::1/64||", "vmbr0/inet=192.0.2.1/24|2001:db8::1|", "vmbr0/inet=||bad-dns"} {
		if _, err := ParseRecoveryNetwork([]string{invalid}); err == nil {
			t.Fatalf("invalid typed input accepted: %s", invalid)
		}
	}
}
func TestCLIRecoveryVerificationFixedRequest(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/api/plans/plan1/verify" {
			t.Error("wrong route", r.Method, r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if len(body) != 0 {
			t.Error("client-selected command")
		}
		json.NewEncoder(w).Encode(map[string]any{"verification_attempts": []any{map[string]any{"state": "failed"}}})
	}))
	defer server.Close()
	var out bytes.Buffer
	c := &Client{HTTP: server.Client(), Base: server.URL, Out: &out}
	if err := c.Run(context.Background(), []string{"restore", "verify", "plan1"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "verification_attempts") {
		t.Fatal("history missing")
	}
	for _, args := range [][]string{{"restore", "verify"}, {"restore", "verify", "plan1", "--services", "sshd"}} {
		if err := c.Run(context.Background(), args); err == nil {
			t.Fatal("invalid verification request accepted")
		}
	}
	if calls != 1 {
		t.Fatal("invalid command reached server")
	}
}
