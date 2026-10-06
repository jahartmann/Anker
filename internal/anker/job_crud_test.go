package anker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestJobHistoryCRUDDoesNotDeleteBackupsOrResetTheSchedule(t *testing.T) {
	s := testService(t)
	defer s.StopJobs(context.Background())
	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	if err := NewScheduler(s).Tick(at); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	var j Job
	for time.Now().Before(deadline) {
		jobs, _ := s.Jobs()
		if len(jobs) > 0 {
			j = jobs[0]
			if j.State == "successful" {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if j.State != "successful" {
		t.Fatal(j)
	}
	if err := s.StopJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	handler := Handler(s, NewAuth(s.Store), true)
	request := func(method, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(`{}`))
		r.Header.Set("X-Anker-Request", "1")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	read := request("GET", "/api/jobs/"+j.ID)
	if read.Code != 200 {
		t.Fatal("job details unavailable", read.Code, read.Body.String())
	}
	var got Job
	json.Unmarshal(read.Body.Bytes(), &got)
	if got.Trigger != "scheduled" || got.ScheduledDay != "2026-10-06" || got.StartedAt == "" {
		t.Fatal("scheduled job metadata not exposed", got)
	}
	remove := request("DELETE", "/api/jobs/"+j.ID)
	if remove.Code != 200 {
		t.Fatal("finished history entry cannot be deleted", remove.Code, remove.Body.String())
	}
	if err := s.VerifyBackup(j.ResultID); err != nil {
		t.Fatal("deleting job removed its backup", err)
	}
	if err := NewScheduler(s).Tick(at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	jobs, _ := s.Jobs()
	if len(jobs) != 0 {
		t.Fatal("deleting history reset today's scheduled run", jobs)
	}
	if request("GET", "/api/jobs/"+j.ID).Code != 404 {
		t.Fatal("deleted history entry still exists")
	}
}

func TestActiveJobCannotBeDeletedOrRetriedAndCancelIsValidated(t *testing.T) {
	s := testService(t)
	gate := make(chan struct{})
	s.Collector = slowCollector{gate: gate}
	defer s.StopJobs(context.Background())
	j, err := s.QueueBackup("host1")
	if err != nil {
		t.Fatal(err)
	}
	handler := Handler(s, NewAuth(s.Store), true)
	for _, action := range []struct{ method, path string }{{"DELETE", "/api/jobs/" + j.ID}, {"POST", "/api/jobs/" + j.ID + "/retry"}} {
		r := httptest.NewRequest(action.method, action.path, strings.NewReader(`{}`))
		r.Header.Set("X-Anker-Request", "1")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 409 {
			t.Fatal("active mutation was not rejected", action, w.Code, w.Body.String())
		}
	}
	if err = s.CancelJob(j.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.StopJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = s.CancelJob(j.ID); err == nil {
		t.Fatal("completed job accepted a misleading cancellation")
	}
}

func TestRetryDoesNotAutomaticallyRepeatARestore(t *testing.T) {
	s := testService(t)
	j := Job{ID: ID(), HostID: "host1", Kind: "restore", State: "failed"}
	s.Store.Put("jobs", j.ID, j)
	r := httptest.NewRequest("POST", "/api/jobs/"+j.ID+"/retry", strings.NewReader(`{}`))
	r.Header.Set("X-Anker-Request", "1")
	w := httptest.NewRecorder()
	Handler(s, NewAuth(s.Store), true).ServeHTTP(w, r)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "Plan") {
		t.Fatal("restore retry was not directed through current plan checks", w.Code, w.Body.String())
	}
}

func TestHostDeletionFailsClosedWithUnreadableJobList(t *testing.T) {
	s := testService(t)
	s.Store.db.Exec(`INSERT INTO records(bucket,id,value) VALUES('jobs','broken','invalid-json')`)
	r := httptest.NewRequest("DELETE", "/api/hosts/host1", strings.NewReader(`{}`))
	r.Header.Set("X-Anker-Request", "1")
	w := httptest.NewRecorder()
	Handler(s, NewAuth(s.Store), true).ServeHTTP(w, r)
	if w.Code == 200 {
		t.Fatal("host deleted without checking active jobs")
	}
	if _, err := s.Host("host1"); err != nil {
		t.Fatal("host removed", err)
	}
}

func TestScheduleSettingsAndHostCRUDReachTheSchedulerAcrossRestart(t *testing.T) {
	s := testService(t)
	defer s.StopJobs(context.Background())
	handler := Handler(s, NewAuth(s.Store), true)
	send := func(method, path string, value any) *httptest.ResponseRecorder {
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(method, path, strings.NewReader(string(body)))
		r.Header.Set("X-Anker-Request", "1")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(method, path, w.Code, w.Body.String())
		}
		return w
	}
	settings, _ := s.Settings()
	settings.Schedule = "00:00"
	settings.Timezone = "UTC"
	send("PUT", "/api/settings", settings)
	h, _ := s.Host("host1")
	h.Enabled = false
	send("POST", "/api/hosts", h)
	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	tick := func(svc *Service, at time.Time, count int) {
		t.Helper()
		if err := NewScheduler(svc).Tick(at); err != nil {
			t.Fatal(err)
		}
		jobs, err := svc.Jobs()
		if err != nil || len(jobs) != count {
			t.Fatal("unexpected scheduled jobs", jobs, err)
		}
	}
	tick(s, at, 0)
	h.Enabled = true
	h.Schedule = "23:59"
	send("POST", "/api/hosts", h)
	tick(s, at, 0)
	h.Schedule = "" // Use the shared schedule saved through the same API as the UI.
	send("POST", "/api/hosts", h)
	tick(s, at, 1)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		jobs, _ := s.Jobs()
		if jobs[0].State == "successful" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	jobs, _ := s.Jobs()
	if jobs[0].State != "successful" {
		t.Fatal(jobs)
	}
	if err := s.StopJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewService(s.Root, s.Store, s.Collector)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.StopJobs(context.Background())
	tick(restarted, at.Add(time.Hour), 1)
	tick(restarted, at.Add(24*time.Hour), 2)
}

func TestQueueAndHostRemovalCannotCreateAnOrphanJob(t *testing.T) {
	for i := 0; i < 15; i++ {
		s := testService(t)
		s.Collector = slowCollector{gate: make(chan struct{})}
		start := make(chan struct{})
		queued := make(chan error, 1)
		removed := make(chan error, 1)
		go func() { <-start; _, err := s.QueueBackup("host1"); queued <- err }()
		go func() { <-start; removed <- s.DeleteHost("host1") }()
		close(start)
		q, d := <-queued, <-removed
		if q == nil && d == nil {
			t.Fatal("host removed while a new job was admitted")
		}
		if q != nil && d != nil {
			t.Fatal("neither operation succeeded", q, d)
		}
		if err := s.StopJobs(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestJobCRUDHonorsReaderRestoreAndAdminPermissions(t *testing.T) {
	s := testService(t)
	defer s.StopJobs(context.Background())
	a := NewAuth(s.Store)
	for _, role := range []string{"reader", "restore", "admin"} {
		if err := a.CreateUser(role, "long-test-password", role, false); err != nil {
			t.Fatal(err)
		}
		token, _, err := a.Login(role, "long-test-password")
		if err != nil {
			t.Fatal(err)
		}
		j := Job{ID: ID(), HostID: "host1", Kind: "probe", State: "failed"}
		if err := s.Store.Put("jobs", j.ID, j); err != nil {
			t.Fatal(err)
		}
		send := func(method, path string) int {
			r := httptest.NewRequest(method, path, strings.NewReader(`{}`))
			r.Header.Set("X-Anker-Request", "1")
			r.AddCookie(&http.Cookie{Name: "anker_session", Value: token})
			w := httptest.NewRecorder()
			Handler(s, a, false).ServeHTTP(w, r)
			return w.Code
		}
		if got := send("GET", "/api/jobs/"+j.ID); got != 200 {
			t.Fatal(role, "read", got)
		}
		want := 403
		if role == "admin" {
			want = 200
		}
		if got := send("DELETE", "/api/jobs/"+j.ID); got != want {
			t.Fatal(role, "remove", got, want)
		}
		if role == "admin" {
			s.Store.Put("jobs", j.ID, j)
		}
		want = 200
		if role == "reader" {
			want = 403
		}
		if got := send("POST", "/api/jobs/"+j.ID+"/retry"); got != want {
			t.Fatal(role, "retry", got, want)
		}
		s.StopJobs(context.Background())
	}
}

func TestHostScheduleChangeWaitsForInProgressAdmission(t *testing.T) {
	s := testService(t)
	h, _ := s.Host("host1")
	h.Enabled = false
	// Hold the admission boundary while the operator saves a schedule change.
	s.jobMu.Lock()
	finished := make(chan error, 1)
	go func() { finished <- s.SaveHost(h) }()
	select {
	case err := <-finished:
		s.jobMu.Unlock()
		t.Fatal("host schedule committed during queue admission", err)
	case <-time.After(100 * time.Millisecond):
	}
	s.jobMu.Unlock()
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	saved, _ := s.Host("host1")
	if saved.Enabled {
		t.Fatal("pause did not persist")
	}
}
