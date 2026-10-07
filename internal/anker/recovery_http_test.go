package anker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoveryInspectionAndJournalRoutesRequireRestoreRole(t *testing.T) {
	c := &recoveryResultCollector{}
	s, p := recoveryReadyPlan(t, c)
	a := NewAuth(s.Store)
	for _, role := range []string{"reader", "restore"} {
		if err := a.CreateUser(role, "long-test-password", role, false); err != nil {
			t.Fatal(err)
		}
		token, _, err := a.Login(role, "long-test-password")
		if err != nil {
			t.Fatal(err)
		}
		p.State = "interrupted"
		s.savePlan(p)
		c.status = ApplyResult{State: "applied", OperationID: p.ID, Applied: []string{"etc/app.conf"}}
		for _, tc := range []struct{ path, body string }{
			{"/api/plans/inspect", `{"backup_id":"` + p.BackupID + `","target_id":"host1","scenario":"files","files":["etc/app.conf"]}`},
			{"/api/plans/" + p.ID + "/reconcile", `{}`},
			{"/api/plans/" + p.ID + "/rollback", `{"confirm":"wrong"}`},
		} {
			r := httptest.NewRequest("POST", tc.path, strings.NewReader(tc.body))
			r.Header.Set("X-Anker-Request", "1")
			r.AddCookie(&http.Cookie{Name: "anker_session", Value: token})
			w := httptest.NewRecorder()
			Handler(s, a, false).ServeHTTP(w, r)
			want := 200
			if role == "reader" {
				want = 403
			} else if strings.HasSuffix(tc.path, "/rollback") {
				want = 400
			}
			if w.Code != want {
				t.Fatalf("%s %s: %d %s", role, tc.path, w.Code, w.Body.String())
			}
		}
	}
	plans, err := records[Plan](s.Store, "plans")
	if err != nil || len(plans) != 1 {
		t.Fatalf("inspection created plan: %d %v", len(plans), err)
	}
	if c.rollbacks != 0 {
		t.Fatal("unconfirmed HTTP rollback reached host")
	}
}

func TestRestartMarksRollbackInterruptedAndDoesNotRetry(t *testing.T) {
	c := &recoveryResultCollector{}
	s, p := recoveryReadyPlan(t, c)
	p.State = "rolling_back"
	s.savePlan(p)
	if err := s.Store.Put("jobs", "rollback-job", Job{ID: "rollback-job", HostID: p.TargetID, Kind: "rollback", State: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecoverJobs(); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Plan(p.ID)
	if err != nil || loaded.State != "interrupted" {
		t.Fatalf("restart plan: %+v %v", loaded, err)
	}
	var j Job
	if err = s.Store.Get("jobs", "rollback-job", &j); err != nil || j.State != "interrupted" {
		t.Fatalf("restart job: %+v %v", j, err)
	}
	if c.rollbacks != 0 {
		t.Fatal("restart retried rollback")
	}
	c.status = ApplyResult{OperationID: p.ID, State: "rolled_back"}
	resolved, err := s.ReconcilePlan(context.Background(), p.ID)
	if err != nil || resolved.State != "rolled_back" {
		t.Fatalf("reconcile after restart: %+v %v", resolved, err)
	}
	raw, _ := json.Marshal(resolved)
	if !strings.Contains(string(raw), "rolled_back") {
		t.Fatal("status missing")
	}
}
