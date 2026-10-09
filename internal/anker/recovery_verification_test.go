package anker

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type verificationAPI interface {
	VerifyRecovery(context.Context, string) (Plan, error)
}

func TestRecoveryVerificationOptionalAndRetainsUnsupported(t *testing.T) {
	s, _, r := snapshotSetup(t)
	p, err := s.CreatePlan(t.Context(), requestDecision(t, r, r.Files[0], "keep"))
	if err != nil {
		t.Fatal(err)
	}
	api, ok := any(s).(verificationAPI)
	if !ok {
		t.Fatal("fresh plan-bound machine verification missing")
	}
	for n := 1; n <= 2; n++ {
		got, err := api.VerifyRecovery(t.Context(), p.ID)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(got)
		var wire struct {
			Attempts []struct {
				State string `json:"state"`
			} `json:"verification_attempts"`
		}
		json.Unmarshal(raw, &wire)
		if len(wire.Attempts) != n || wire.Attempts[n-1].State != "unsupported" {
			t.Fatalf("history missing: %s", raw)
		}
		if got.State != p.State {
			t.Fatal("verification changed operation state")
		}
	}
}

type verifyCollector struct {
	snapshotCollector
	resultVerify RecoveryVerificationResult
	run          func()
	err          error
	calls        int
}

func (c *verifyCollector) VerifyRecovery(_ context.Context, _ Host, services, ids []string) (RecoveryVerificationResult, error) {
	c.calls++
	if c.run != nil {
		c.run()
	}
	return c.resultVerify, c.err
}
func verificationSetup(t *testing.T) (*Service, Plan, *verifyCollector) {
	s, _, r := snapshotSetup(t)
	c := &verifyCollector{snapshotCollector: s.Collector.(snapshotCollector)}
	c.inv.Details["machine_identity_sha256"] = json.RawMessage(`"` + strings.Repeat("a", 64) + `"`)
	c.inv.Details["boot_id"] = json.RawMessage(`"12345678-1234-1234-1234-123456789abc"`)
	c.inv.Details["command_results"] = json.RawMessage(`{"file_inventory":{"state":"ok"},"machine_identity":{"state":"ok"},"boot_id":{"state":"ok"}}`)
	c.inv.Details["capabilities"] = json.RawMessage(`{"recovery_verify_protocol":1}`)
	c.resultVerify = RecoveryVerificationResult{Protocol: 1, ObservedAt: now(), Identity: VerificationQuery{"ok", strings.Repeat("a", 64)}, Boot: VerificationQuery{"ok", "12345678-1234-1234-1234-123456789def"}, Services: map[string]VerificationQuery{"pve-cluster": {"ok", "active"}, "pvedaemon": {"ok", "active"}, "pveproxy": {"ok", "active"}}, StorageVolumes: map[string]RecoveryVolumeResult{}}
	s.Collector = c
	p, err := s.CreatePlan(t.Context(), requestDecision(t, r, r.Files[0], "keep"))
	if err != nil {
		t.Fatal(err)
	}
	return s, p, c
}
func TestRecoveryVerificationBaselineAndFailures(t *testing.T) {
	for _, kind := range []string{"changed-boot", "unchanged", "missing-current-id", "wrong-id", "failed-service", "ssh", "corrupt", "changed-connection", "during-connection", "private-plan", "private-snapshot", "legacy", "missing-baseline", "false-query-state", "missing-boot", "missing-capability"} {
		t.Run(kind, func(t *testing.T) {
			s, p, c := verificationSetup(t)
			switch kind {
			case "unchanged":
				c.resultVerify.Boot.Value = "12345678-1234-1234-1234-123456789abc"
			case "missing-current-id":
				c.resultVerify.Identity = VerificationQuery{"unknown", ""}
			case "wrong-id":
				c.resultVerify.Identity.Value = strings.Repeat("b", 64)
			case "failed-service":
				c.resultVerify.Services["pveproxy"] = VerificationQuery{"failed", ""}
			case "ssh":
				c.err = errors.New("interrupted")
			case "corrupt":
				delete(c.resultVerify.Services, "pveproxy")
			case "changed-connection", "during-connection":
				change := func() {
					h, _ := s.Host(p.TargetID)
					h.Address = "192.0.2.250"
					if err := s.SaveHost(h); err != nil {
						t.Fatal(err)
					}
				}
				if kind == "during-connection" {
					c.run = change
				} else {
					change()
				}
			case "private-plan":
				os.WriteFile(filepath.Join(s.Root, "plans", p.ID, "mapping.json"), []byte(`{}`), 0600)
			case "private-snapshot":
				os.WriteFile(filepath.Join(s.Root, "plans", p.ID, "target-snapshot.json"), []byte(`{}`), 0600)
			case "legacy":
				p.SchemaVersion = 0
				s.savePlan(p)
			case "missing-baseline", "false-query-state", "missing-boot", "missing-capability":
				snap, err := s.loadPlanSnapshot(p)
				if err != nil {
					t.Fatal(err)
				}
				if kind == "missing-baseline" {
					delete(snap.Target.Details, "machine_identity_sha256")
				}
				if kind == "missing-boot" {
					delete(snap.Target.Details, "boot_id")
				}
				if kind == "false-query-state" {
					snap.Target.Details["command_results"] = json.RawMessage(`{"file_inventory":{"state":"ok"}}`)
					snap.Target.QueryStates = map[string]InventoryQueryState{"boot_id": {"ok"}, "machine_identity": {"ok"}}
				}
				if kind == "missing-capability" {
					delete(snap.Target.Details, "capabilities")
				}
				snap.Target.Fingerprint = Fingerprint(snap.Target)
				p.Target = snap.Target
				p.TargetSnapshotSHA, err = copyPlanSnapshot(filepath.Join(s.Root, "plans", p.ID), snap)
				if err != nil {
					t.Fatal(err)
				}
				s.savePlan(p)
			}
			got, err := s.VerifyRecovery(t.Context(), p.ID)
			if err != nil {
				t.Fatal(err)
			}
			a := got.VerificationAttempts[0]
			if kind == "changed-boot" || kind == "failed-service" {
				if a.RebootState != "passed" {
					t.Fatalf("valid changed boot not proven: %+v", a)
				}
			} else if a.RebootState == "passed" {
				t.Fatalf("invalid reboot proof: %+v", a)
			}
			if kind == "failed-service" && a.ServicesState != "failed" {
				t.Fatal(a)
			}
			if (kind == "changed-connection" || kind == "private-snapshot" || kind == "legacy") && c.calls != 0 {
				t.Fatal("unsafe read reached host")
			}
			if got.State != p.State || !reflect.DeepEqual(got.Tasks, p.Tasks) || !reflect.DeepEqual(got.Result, p.Result) {
				t.Fatal("machine result changed human/operation state")
			}
			saved, _ := s.Plan(p.ID)
			if len(saved.VerificationAttempts) != 1 {
				t.Fatal("failed attempt not persisted")
			}
		})
	}
}
func TestRecoveryVerificationStrictWire(t *testing.T) {
	_, _, c := verificationSetup(t)
	raw, _ := json.Marshal(c.resultVerify)
	for _, mutate := range []func(string) string{
		func(s string) string { return s + "{}" }, func(s string) string { return strings.Replace(s, `"protocol":1`, `"protocol":1,"protocol":1`, 1) },
		func(s string) string { return strings.Replace(s, `"protocol":1`, `"protocol":1,"extra":true`, 1) },
		func(s string) string {
			return strings.Replace(s, `"services":{`, `"services":{"sshd":{"state":"ok","value":"active"},`, 1)
		},
		func(s string) string { return strings.Replace(s, `"identity":{`, `"identity":{"extra":1,`, 1) },
		func(s string) string { return strings.Replace(s, `"state":"ok"`, `"state":"unknown"`, 1) },
		func(s string) string { return s[:len(s)-1] },
	} {
		if _, err := decodeVerification(strings.NewReader(mutate(string(raw))), verificationServices, nil); err == nil {
			t.Fatal("corrupt wire accepted")
		}
	}
	if _, err := decodeVerification(strings.NewReader(string(raw)), verificationServices, nil); err != nil {
		t.Fatal(err)
	}
}
func TestRecoveryVerificationTrustMaterialAndInterruption(t *testing.T) {
	for _, field := range []string{"known", "key"} {
		t.Run(field, func(t *testing.T) {
			s, _, r := snapshotSetup(t)
			h, _ := s.Host(r.TargetID)
			h.KeyPath = filepath.Join(t.TempDir(), "key")
			h.KnownHostsPath = filepath.Join(t.TempDir(), "known")
			os.WriteFile(h.KeyPath, []byte("key-before"), 0600)
			os.WriteFile(h.KnownHostsPath, []byte("known-before"), 0600)
			s.SaveHost(h)
			p, err := s.CreatePlan(t.Context(), requestDecision(t, r, r.Files[0], "keep"))
			if err != nil {
				t.Fatal(err)
			}
			path := h.KnownHostsPath
			if field == "key" {
				path = h.KeyPath
			}
			os.WriteFile(path, []byte("changed"), 0600)
			got, err := s.VerifyRecovery(t.Context(), p.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.VerificationAttempts[0].State != "failed" {
				t.Fatal("changed trust material not rejected", got.VerificationAttempts)
			}
		})
	}
	s, p, c := verificationSetup(t)
	c.run = func() {
		saved, err := s.Plan(p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(saved.VerificationAttempts) != 1 || saved.VerificationAttempts[0].State != "interrupted" {
			t.Fatal("attempt not durable before host read")
		}
	}
	if _, err := s.VerifyRecovery(t.Context(), p.ID); err != nil {
		t.Fatal(err)
	}
}
func TestRecoveryVerificationHTTPAuthorityAndFixedRequest(t *testing.T) {
	s, p, c := verificationSetup(t)
	for _, tc := range []struct {
		role, method, body string
		want               int
	}{{"reader", "POST", "{}", 403}, {"restore", "GET", "{}", 405}, {"restore", "POST", `{"services":["sshd"]}`, 400}, {"restore", "POST", `{} {}`, 400}, {"restore", "POST", "null", 400}, {"restore", "POST", "{}", 200}} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(tc.method, "/api/plans/"+p.ID+"/verify", strings.NewReader(tc.body))
		handled, err := s.recoveryHTTP(User{ID: "verifier", Role: tc.role}, w, r)
		if !handled {
			t.Fatal("missing verify route")
		}
		if tc.want == 200 {
			if err != nil {
				t.Fatal(err)
			}
			var got Plan
			json.Unmarshal(w.Body.Bytes(), &got)
			if got.VerificationAttempts[0].Actor != "verifier" {
				t.Fatal("missing actor")
			}
		} else {
			var e apiError
			if !errors.As(err, &e) || e.code != tc.want {
				t.Fatalf("%+v: %v", tc, err)
			}
		}
	}
	if c.calls != 1 {
		t.Fatalf("invalid request reached host: %d", c.calls)
	}
}
func TestRecoveryVerificationPrivateExportAndHistoryLimit(t *testing.T) {
	s, p, c := verificationSetup(t)
	before, err := os.ReadFile(filepath.Join(s.Root, "plans", p.ID, "plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.VerifyRecovery(t.Context(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	c.err = errors.New("interrupted")
	got, err = s.VerifyRecovery(t.Context(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(s.Root, "plans", p.ID, "plan.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("verification mutated private operation plan")
	}
	if err = s.verifyPlanFiles(got); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = s.ExportPlan(p.ID, &out); err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyPlanPackage(bytes.NewReader(out.Bytes())); err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(bytes.NewReader(out.Bytes()))
	found := map[string][]byte{}
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		data, _ := io.ReadAll(tr)
		found[h.Name] = data
	}
	if !bytes.Contains(found["WIEDERHERSTELLUNG.md"], []byte(got.VerificationAttempts[1].ID)) || !bytes.Contains(found["machine-verification.json"], []byte(got.VerificationAttempts[0].ID)) {
		t.Fatal("fresh machine history missing from export")
	}
	history := got.VerificationAttempts
	for len(history) < 100 {
		a := history[0]
		a.ID = ID()
		history = append(history, a)
	}
	s.Store.Put("recovery-verifications", p.ID, history)
	calls := c.calls
	if _, err = s.VerifyRecovery(t.Context(), p.ID); err == nil {
		t.Fatal("history limit absent")
	}
	if c.calls != calls {
		t.Fatal("limit performed host read")
	}
	saved, _ := s.Plan(p.ID)
	if len(saved.VerificationAttempts) != 100 {
		t.Fatal("old attempts pruned")
	}
}
func TestRecoveryVerificationUsesReadSSH(t *testing.T) {
	_, _, c := verificationSetup(t)
	raw, _ := json.Marshal(c.resultVerify)
	dir := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$ANKER_VERIFY_ARGS\"\ncat > \"$ANKER_VERIFY_REQUEST\"\nprintf '%s' \"$ANKER_VERIFY_REPLY\"\n"
	os.WriteFile(filepath.Join(dir, "ssh"), []byte(script), 0700)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("ANKER_VERIFY_ARGS", filepath.Join(dir, "args"))
	t.Setenv("ANKER_VERIFY_REQUEST", filepath.Join(dir, "request"))
	t.Setenv("ANKER_VERIFY_REPLY", string(raw))
	for _, name := range []string{"read", "restore", "known"} {
		os.WriteFile(filepath.Join(dir, name), []byte("fixture"), 0600)
	}
	h := Host{Address: "192.0.2.3", SSHPort: 22, SSHUser: "read-user", KeyPath: filepath.Join(dir, "read"), KnownHostsPath: filepath.Join(dir, "known"), RestoreSSHUser: "restore-user", RestoreKeyPath: filepath.Join(dir, "restore")}
	if _, err := (SSHCollector{}).VerifyRecovery(t.Context(), h, verificationServices, []string{}); err != nil {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(filepath.Join(dir, "args"))
	req, _ := os.ReadFile(filepath.Join(dir, "request"))
	for _, required := range []string{"StrictHostKeyChecking=yes", "read-user@192.0.2.3", "--read-only", filepath.Join(dir, "read")} {
		if !bytes.Contains(args, []byte(required)) {
			t.Fatal("read authority missing", string(args))
		}
	}
	if bytes.Contains(args, []byte("restore-user")) || !bytes.Contains(req, []byte(`"operation":"verify-recovery"`)) {
		t.Fatal("wrong operation/authority")
	}
}
func TestRecoveryVerificationDisasterPreservesRevokedHistory(t *testing.T) {
	s, p, _ := verificationSetup(t)
	if err := EnsureDataMode(s.Root, false); err != nil {
		t.Fatal(err)
	}
	got, err := s.VerifyRecovery(t.Context(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = ExportDisaster(s.Root, "", []byte("test-passphrase"), &out); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "imported")
	if _, err = ImportDisaster(&out, dest, []byte("test-passphrase")); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(filepath.Join(dest, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var attempts []RecoveryVerificationAttempt
	if err = store.Get("recovery-verifications", p.ID, &attempts); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(attempts, got.VerificationAttempts) {
		t.Fatal("disaster lost historical machine evidence")
	}
	imported := *s
	imported.Store = store
	imported.Root = dest
	if err = imported.disasterPlanAllowed(p.ID); err == nil {
		t.Fatal("import revived old plan authority")
	}
}
func TestRecoveryVerificationStorageStatesAndBound(t *testing.T) {
	for _, kind := range []string{"present", "missing", "failed", "unknown-baseline"} {
		t.Run(kind, func(t *testing.T) {
			s, p, c := verificationSetup(t)
			snap, err := s.loadPlanSnapshot(p)
			if err != nil {
				t.Fatal(err)
			}
			snap.StorageVolumes = map[string]RecoveryVolumeResult{"local": {State: "ok", Volumes: []string{"local:vm-100-disk-0"}}}
			if kind == "unknown-baseline" {
				snap.StorageVolumes["local"] = RecoveryVolumeResult{State: "unknown"}
			}
			p.TargetSnapshotSHA, err = copyPlanSnapshot(filepath.Join(s.Root, "plans", p.ID), snap)
			if err != nil {
				t.Fatal(err)
			}
			s.savePlan(p)
			c.resultVerify.StorageVolumes = map[string]RecoveryVolumeResult{"local": {State: "ok", Volumes: []string{"local:vm-100-disk-0"}}}
			if kind == "missing" {
				c.resultVerify.StorageVolumes["local"] = RecoveryVolumeResult{State: "ok", Volumes: []string{}}
			}
			if kind == "failed" {
				c.resultVerify.StorageVolumes["local"] = RecoveryVolumeResult{State: "failed"}
			}
			got, err := s.VerifyRecovery(t.Context(), p.ID)
			if err != nil {
				t.Fatal(err)
			}
			a := got.VerificationAttempts[0]
			want := "passed"
			if kind != "present" {
				want = "failed"
			}
			if kind == "unknown-baseline" {
				want = "unknown"
			}
			if a.VolumesState != want {
				t.Fatal(a)
			}
			if kind == "failed" || kind == "missing" {
				if a.State != "failed" {
					t.Fatal("failed volume not presented as failed attempt", a)
				}
			}
		})
	}
	_, _, c := verificationSetup(t)
	raw, _ := json.Marshal(c.resultVerify)
	if _, err := decodeVerification(strings.NewReader(string(raw)+strings.Repeat(" ", 256<<10)), verificationServices, nil); err == nil {
		t.Fatal("response too large for bounded 100-attempt exports accepted")
	}
}
func TestRecoveryVerificationSourceDriftAndPrivatePlanRetained(t *testing.T) {
	for _, during := range []bool{false, true} {
		t.Run(fmt.Sprint(during), func(t *testing.T) {
			s, p, c := verificationSetup(t)
			b, _ := s.Backup(p.BackupID)
			corrupt := func() {
				if err := os.WriteFile(filepath.Join(s.backupDir(b), "manifest.json"), []byte(`{}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if during {
				c.run = corrupt
			} else {
				corrupt()
			}
			got, err := s.VerifyRecovery(t.Context(), p.ID)
			if err != nil {
				t.Fatal(err)
			}
			a := got.VerificationAttempts[0]
			if a.State != "failed" || a.RebootState != "unknown" || a.Result != nil {
				t.Fatal("source corruption accepted", a)
			}
			if !during && c.calls != 0 {
				t.Fatal("source corruption reached host")
			}
		})
	}
	s, p, _ := verificationSetup(t)
	path := filepath.Join(s.Root, "plans", p.ID, "mapping.json")
	raw := []byte(`{"storage":{"changed":"bad"}}`)
	os.WriteFile(path, raw, 0600)
	got, err := s.VerifyRecovery(t.Context(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if got.VerificationAttempts[0].State != "failed" || !bytes.Equal(raw, after) {
		t.Fatal("corrupt private plan repaired or accepted")
	}
}
func TestRecoveryVerificationRejectsPreviousAttemptResponse(t *testing.T) {
	s, p, c := verificationSetup(t)
	c.resultVerify.ObservedAt = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
	got, err := s.VerifyRecovery(t.Context(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.VerificationAttempts[0].State != "failed" || got.VerificationAttempts[0].RebootState == "passed" {
		t.Fatal("old response accepted")
	}
}

// The transport adapter exercises wire validation and persisted service behavior
// together, without replacing either decoder or verification decision logic.
type verificationWireCollector struct {
	*verifyCollector
	wire string
}

func (c verificationWireCollector) VerifyRecovery(_ context.Context, _ Host, services, ids []string) (RecoveryVerificationResult, error) {
	return decodeVerification(strings.NewReader(c.wire), services, ids)
}
func TestRecoveryVerificationRejectsNegativeServiceCaseAlias(t *testing.T) {
	s, p, c := verificationSetup(t)
	raw, _ := json.Marshal(c.resultVerify)
	wire := strings.Replace(string(raw), `"pveproxy":{"state":"ok","value":"active"}`, `"pveproxy":{"state":"failed","value":"","State":"ok","Value":"active"}`, 1)
	if wire == string(raw) {
		t.Fatal("fixture did not insert contradictory service result")
	}
	s.Collector = verificationWireCollector{c, wire}
	got, err := s.VerifyRecovery(t.Context(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	a := got.VerificationAttempts[0]
	if a.State != "failed" || a.ServicesState != "unknown" || a.IdentityState != "unknown" || a.RebootState != "unknown" || a.Result != nil {
		t.Fatalf("contradictory wire created positive evidence: %+v", a)
	}
	saved, err := s.Plan(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(saved.VerificationAttempts, got.VerificationAttempts) {
		t.Fatal("corrupt-response attempt not retained")
	}
	if saved.State != p.State || !reflect.DeepEqual(saved.Tasks, p.Tasks) || !reflect.DeepEqual(saved.Result, p.Result) {
		t.Fatal("corrupt response changed operation or human evidence")
	}
}
func TestRecoveryVerificationExactWireFieldNames(t *testing.T) {
	_, _, c := verificationSetup(t)
	c.resultVerify.StorageVolumes = map[string]RecoveryVolumeResult{"local": {State: "ok", Volumes: []string{"local:vm-100-disk-0"}}}
	raw, _ := json.Marshal(c.resultVerify)
	// Every field at every struct level must be exact, even if both spellings
	// would currently decode to the same value. Map entry names retain their
	// separately validated service/storage membership contract.
	scopes := []struct {
		name string
		path []string
		keys []string
	}{
		{"envelope", nil, []string{"protocol", "observed_at", "identity", "boot", "services", "storage_volumes"}},
		{"identity", []string{"identity"}, []string{"state", "value"}},
		{"boot", []string{"boot"}, []string{"state", "value"}},
		{"service", []string{"services", "pveproxy"}, []string{"state", "value"}},
		{"storage", []string{"storage_volumes", "local"}, []string{"state", "volumes", "error"}},
	}
	for _, scope := range scopes {
		for _, key := range scope.keys {
			t.Run(scope.name+"/"+key, func(t *testing.T) {
				var tree map[string]any
				json.Unmarshal(raw, &tree)
				node := tree
				for _, segment := range scope.path {
					node = node[segment].(map[string]any)
				}
				value := node[key]
				if key == "error" {
					value = ""
					node[key] = value
				}
				node[strings.ToUpper(key[:1])+key[1:]] = value
				bad, _ := json.Marshal(tree)
				if _, err := decodeVerification(bytes.NewReader(bad), verificationServices, []string{"local"}); err == nil {
					t.Fatalf("case alias accepted at %s.%s: %s", scope.name, key, bad)
				}
			})
		}
	}
	if _, err := decodeVerification(bytes.NewReader(raw), verificationServices, []string{"local"}); err != nil {
		t.Fatal("canonical result rejected", err)
	}
}
func TestRecoveryVerificationWireRequiredFieldsAndNulls(t *testing.T) {
	_, _, c := verificationSetup(t)
	c.resultVerify.StorageVolumes = map[string]RecoveryVolumeResult{"local": {State: "ok", Volumes: []string{}}}
	raw, _ := json.Marshal(c.resultVerify)
	for _, path := range [][]string{{"protocol"}, {"observed_at"}, {"identity"}, {"boot"}, {"services"}, {"storage_volumes"}, {"identity", "state"}, {"identity", "value"}, {"boot", "state"}, {"boot", "value"}, {"services", "pveproxy"}, {"services", "pveproxy", "state"}, {"services", "pveproxy", "value"}, {"storage_volumes", "local"}, {"storage_volumes", "local", "state"}, {"storage_volumes", "local", "volumes"}} {
		for _, omit := range []bool{false, true} {
			t.Run(strings.Join(path, "/")+fmt.Sprint(omit), func(t *testing.T) {
				var tree map[string]any
				json.Unmarshal(raw, &tree)
				node := tree
				for _, segment := range path[:len(path)-1] {
					node = node[segment].(map[string]any)
				}
				if omit {
					delete(node, path[len(path)-1])
				} else {
					node[path[len(path)-1]] = nil
				}
				bad, _ := json.Marshal(tree)
				if _, err := decodeVerification(bytes.NewReader(bad), verificationServices, []string{"local"}); err == nil {
					t.Fatal("missing/null required field accepted", string(bad))
				}
			})
		}
	}
	// Error is optional, but an explicitly present null is not a string value.
	bad := strings.Replace(string(raw), `"storage_volumes":{"local":{`, `"storage_volumes":{"local":{"error":null,`, 1)
	if _, err := decodeVerification(strings.NewReader(bad), verificationServices, []string{"local"}); err == nil {
		t.Fatal("explicit null optional error accepted")
	}
	// The host helper omits volumes on a failed query. The shared Go model's
	// nil slice marshals to null, which is deliberately not accepted on this wire.
	failed := strings.Replace(string(raw), `"local":{"state":"ok","volumes":[]}`, `"local":{"state":"failed","error":"query failed"}`, 1)
	if failed == string(raw) {
		t.Fatal("fixture did not insert failed volume query")
	}
	raw = []byte(failed)
	if _, err := decodeVerification(bytes.NewReader(raw), verificationServices, []string{"local"}); err != nil {
		t.Fatal("canonical failed volume result rejected", err)
	}
}
