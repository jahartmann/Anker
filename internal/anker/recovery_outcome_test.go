package anker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecoveryOutcomeAutomaticAndDetectsDrift(t *testing.T) {
	s, r, root := localRecoveryHelper(t)
	p := localPlan(t, s, r)
	p, err := s.ApplyPlan(t.Context(), p.ID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.OutcomeChecks) != 1 || p.OutcomeChecks[0].State != "passed" {
		t.Fatalf("automatic outcome %+v", p.OutcomeChecks)
	}
	a := p.OutcomeChecks[0]
	if len(a.Files) != 1 || len(a.Settings) != 1 || a.Settings[0].ExpectedValue != "10" || a.Settings[0].ObservedValue != "10" {
		t.Fatalf("outcome %+v", a)
	}
	file := filepath.Join(root, r.Files[0])
	if err = os.WriteFile(file, []byte("vm.swappiness=77\n"), 0640); err != nil {
		t.Fatal(err)
	}
	p, err = s.VerifyRecoveryOutcome(t.Context(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	a = p.OutcomeChecks[len(p.OutcomeChecks)-1]
	if a.State != "different" || a.Files[0].ContentState != "different" || a.Settings[0].ObservedValue != "77" || a.Files[0].Comparison == nil {
		t.Fatalf("drift %+v", a)
	}
	if len(p.OutcomeChecks) != 2 || p.OutcomeChecks[0].State != "passed" {
		t.Fatal("history overwritten")
	}
	if p.State != "checks_pending" {
		t.Fatal("verification promoted plan state", p.State)
	}
	if err = s.verifyPlanFiles(p); err != nil {
		t.Fatal("report changed immutable plan", err)
	}
	if err = os.WriteFile(file, []byte("vm.swappiness=10\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(file, 0600); err != nil {
		t.Fatal(err)
	}
	p, err = s.VerifyRecoveryOutcome(t.Context(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	a = p.OutcomeChecks[len(p.OutcomeChecks)-1]
	if a.State != "different" || a.Files[0].ContentState != "passed" || a.Files[0].MetadataState != "different" {
		t.Fatalf("mode drift %+v", a)
	}
}
func TestRecoveryOutcomeChecksRemovalsRollbackAndMissingFiles(t *testing.T) {
	s, r, root := removalFixture(t)
	p := localPlan(t, s, r)
	p, err := s.ApplyPlan(t.Context(), p.ID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	a := p.OutcomeChecks[0]
	if a.State != "passed" || a.Files[0].Expected != "missing" || a.Files[0].Observed != "missing" {
		t.Fatalf("delete check %+v", a)
	}
	p, err = s.RollbackPlan(t.Context(), p.ID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	a = p.OutcomeChecks[len(p.OutcomeChecks)-1]
	if a.State != "passed" || a.Direction != "rollback" || a.Files[0].Expected != "read" {
		t.Fatalf("rollback check %+v", a)
	}
	if err = os.Remove(filepath.Join(root, r.RemoveFiles[0])); err != nil {
		t.Fatal(err)
	}
	p, err = s.VerifyRecoveryOutcome(t.Context(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	a = p.OutcomeChecks[len(p.OutcomeChecks)-1]
	if a.State != "different" || a.Files[0].Observed != "missing" {
		t.Fatalf("missing %+v", a)
	}
}
func TestRecoveryOutcomeConflictingUnselectedDeclarationsAndSecrets(t *testing.T) {
	s, r, root := localRecoveryHelper(t)
	p := localPlan(t, s, r)
	p, err := s.ApplyPlan(t.Context(), p.ID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	extra := filepath.Join(root, "etc/sysctl.d/extra.conf")
	os.WriteFile(extra, []byte("vm.swappiness=99\n"), 0640)
	p, err = s.VerifyRecoveryOutcome(t.Context(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	a := p.OutcomeChecks[len(p.OutcomeChecks)-1]
	if a.State != "different" || a.Files[0].State != "passed" || a.Settings[0].ObservedState != "ambiguous" {
		t.Fatalf("conflict %+v", a)
	}
	os.Remove(extra)
	const secret = "private-outcome-credential-123"
	os.WriteFile(filepath.Join(root, r.Files[0]), []byte("password="+secret+"\nvm.swappiness=99\n"), 0640)
	p, err = s.VerifyRecoveryOutcome(t.Context(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	a = p.OutcomeChecks[len(p.OutcomeChecks)-1]
	raw, _ := json.Marshal(a)
	if strings.Contains(string(raw), secret) || a.Files[0].Comparison != nil || a.Settings[0].ObservedValue != "" {
		t.Fatalf("private outcome leaked %s", raw)
	}
}
func TestRecoveryOutcomeRejectsInvalidProofAndPrematureChecks(t *testing.T) {
	s, r, _ := localRecoveryHelper(t)
	p := localPlan(t, s, r)
	if _, err := s.VerifyRecoveryOutcome(t.Context(), p.ID); err == nil {
		t.Fatal("ready plan verified")
	}
	p, err := s.ApplyPlan(t.Context(), p.ID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(s.Root, "plans", p.ID, "prepared-files", r.Files[0]), []byte("changed"), 0600)
	p, err = s.VerifyRecoveryOutcome(t.Context(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	a := p.OutcomeChecks[len(p.OutcomeChecks)-1]
	if a.State != "unknown" || len(a.Files) > 0 || len(a.Settings) > 0 {
		t.Fatalf("invalid proof produced partial success %+v", a)
	}
}

func TestRecoveryOutcomeHTTPAuthorizationAndStrictInput(t *testing.T) {
	s, r, _ := localRecoveryHelper(t)
	p := localPlan(t, s, r)
	p, err := s.ApplyPlan(t.Context(), p.ID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/plans/" + p.ID + "/verify-files"
	for _, tc := range []struct {
		method string
		body   string
		role   string
		code   int
	}{
		{"POST", "{}", "reader", 403}, {"GET", "", "restore", 405}, {"POST", `{"reveal":true}`, "restore", 400}, {"POST", "{}", "restore", 200},
	} {
		w := comparisonRequest(t, s, tc.method, path, json.RawMessage(tc.body), &User{Role: tc.role})
		if w.Code != tc.code {
			t.Fatalf("%s %s %d %s", tc.method, tc.role, w.Code, w.Body.String())
		}
	}
}

type outcomeMutationReader struct {
	SSHCollector
	change func(*RecoveryReadResult)
}

func (c outcomeMutationReader) ReadRecoveryFiles(ctx context.Context, h Host, paths, ids []string) (RecoveryReadResult, error) {
	result, err := c.SSHCollector.ReadRecoveryFiles(ctx, h, paths, ids)
	if err == nil {
		c.change(&result)
	}
	return result, err
}
func TestRecoveryOutcomeRejectsChangingAndPartialReads(t *testing.T) {
	for _, kind := range []string{"target-change", "partial-response", "connection-change"} {
		t.Run(kind, func(t *testing.T) {
			s, r, root := localRecoveryHelper(t)
			p := localPlan(t, s, r)
			p, err := s.ApplyPlan(t.Context(), p.ID, p.ID)
			if err != nil {
				t.Fatal(err)
			}
			s.Collector = outcomeMutationReader{change: func(result *RecoveryReadResult) {
				switch kind {
				case "target-change":
					os.WriteFile(filepath.Join(root, r.Files[0]), []byte("vm.swappiness=80\n"), 0640)
				case "partial-response":
					result.Files = result.Files[:0]
				case "connection-change":
					h, _ := s.Host(r.TargetID)
					h.Address = "192.0.2.200"
					if e := s.SaveHost(h); e != nil {
						t.Fatal(e)
					}
				}
			}}
			p, err = s.VerifyRecoveryOutcome(t.Context(), p.ID)
			if err != nil {
				t.Fatal(err)
			}
			a := p.OutcomeChecks[len(p.OutcomeChecks)-1]
			if a.State != "unknown" || len(a.Files) > 0 || len(a.Settings) > 0 {
				t.Fatalf("partial/changing observation accepted %+v", a)
			}
		})
	}
}
