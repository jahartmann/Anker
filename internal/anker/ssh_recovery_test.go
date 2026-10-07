package anker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMalformedRestoreResponseCannotLeavePartialAppliedEvidence(t *testing.T) {
	for _, raw := range []string{`{"state":"applied","operation_id":42}`, `{"state":"applied","operation_id":"plan1","applied":["etc/app.conf"]`, `{"state":"applied","operation_id":"plan1"} trailing`} {
		result, err := decodeRecoveryResult(strings.NewReader(raw))
		if err == nil || result.State != "" || result.OperationID != "" || len(result.Applied) > 0 {
			t.Fatalf("partial response trusted: %+v %v", result, err)
		}
	}
}

func TestRollbackCannotReportSuccessFromUnchangedAppliedJournal(t *testing.T) {
	c := &recoveryResultCollector{}
	s, p := recoveryReadyPlan(t, c)
	p.State = "checks_pending"
	s.savePlan(p)
	c.status = ApplyResult{State: "applied", OperationID: p.ID, Applied: []string{"etc/app.conf"}}
	c.rollbackResult = c.status
	p, err := s.RollbackPlan(t.Context(), p.ID, p.ID)
	if err == nil {
		t.Fatalf("rollback did not roll back but returned success: %+v", p)
	}
}

func TestSSHRecoveryTransportPreservesFailedJournalAndUsesRestoreIdentity(t *testing.T) {
	dir := t.TempDir()
	script := []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$ANKER_TEST_ARGS\"\ncat > \"$ANKER_TEST_REQUEST\"\nprintf '%s\\n' \"$ANKER_TEST_REPLY\"\nexit \"$ANKER_TEST_EXIT\"\n")
	if err := os.WriteFile(filepath.Join(dir, "ssh"), script, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, name := range []string{"backup-key", "restore-key", "known"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	requestPath, argsPath := filepath.Join(dir, "request"), filepath.Join(dir, "args")
	t.Setenv("ANKER_TEST_REQUEST", requestPath)
	t.Setenv("ANKER_TEST_ARGS", argsPath)
	t.Setenv("ANKER_TEST_REPLY", `{"state":"rolled_back","operation_id":"plan-test","error":"disk full","applied":["etc/app.conf"],"rollback_path":"/host/plan-test"}`)
	t.Setenv("ANKER_TEST_EXIT", "1")
	h := Host{Address: "192.0.2.2", SSHPort: 22, SSHUser: "anker", KeyPath: filepath.Join(dir, "backup-key"), RestoreSSHUser: "anker-restore", RestoreKeyPath: filepath.Join(dir, "restore-key"), KnownHostsPath: filepath.Join(dir, "known")}
	c := SSHCollector{}
	result, err := c.Rollback(t.Context(), h, Plan{ID: "plan-test"}, "plan-test")
	if err == nil || !strings.Contains(err.Error(), "disk full") || result.State != "rolled_back" || result.OperationID != "plan-test" {
		t.Fatalf("failed journal lost: %+v %v", result, err)
	}
	args, _ := os.ReadFile(argsPath)
	if !strings.Contains(string(args), "anker-restore@192.0.2.2") || strings.Contains(string(args), "--read-only") {
		t.Fatalf("wrong authority: %s", args)
	}
	request, _ := os.ReadFile(requestPath)
	var payload map[string]any
	if json.Unmarshal(request, &payload) != nil || payload["operation"] != "rollback" || payload["confirm"] != "plan-test" {
		t.Fatalf("wrong envelope: %s", request)
	}
	t.Setenv("ANKER_TEST_EXIT", "0")
	t.Setenv("ANKER_TEST_REPLY", `{"state":"interrupted","operation_id":"plan-test"}`)
	if _, err = c.RestoreStatus(t.Context(), h, Plan{ID: "plan-test"}); err != nil {
		t.Fatal(err)
	}
	args, _ = os.ReadFile(argsPath)
	if strings.Contains(string(args), "--read-only") {
		t.Fatal("journal query used backup authority")
	}
	t.Setenv("ANKER_TEST_REPLY", `{"state":"applied","operation_id":42}`)
	result, err = c.RestoreStatus(t.Context(), h, Plan{ID: "plan-test"})
	if err == nil || result.State != "" {
		t.Fatalf("malformed host evidence retained: %+v %v", result, err)
	}
}
