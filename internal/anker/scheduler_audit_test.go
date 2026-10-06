package anker

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSchedulerRefusesUnreadableDayMarkerWithoutQueuing(t *testing.T) {
	s := testService(t)
	defer s.StopJobs(context.Background())
	if _, err := s.Store.db.Exec(`INSERT INTO records(bucket,id,value) VALUES('schedule','host1','invalid-json')`); err != nil {
		t.Fatal(err)
	}
	err := NewScheduler(s).Tick(time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC))
	jobs, _ := s.Jobs()
	if err == nil || len(jobs) != 0 {
		t.Fatal("unreadable day marker allowed a duplicate run", jobs, err)
	}
}

func TestScheduledJobAndDayMarkerAreCommittedTogether(t *testing.T) {
	s := testService(t)
	defer s.StopJobs(context.Background())
	if _, err := s.Store.db.Exec(`CREATE TRIGGER fail_schedule BEFORE INSERT ON records WHEN NEW.bucket='schedule' BEGIN SELECT RAISE(ABORT,'fixture marker write failure'); END`); err != nil {
		t.Fatal(err)
	}
	err := NewScheduler(s).Tick(time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC))
	jobs, _ := s.Jobs()
	if err == nil || len(jobs) != 0 {
		t.Fatal("job published without durable daily marker", jobs, err)
	}
}

func TestWorkerDoesNotRunWithUnreadableSettings(t *testing.T) {
	s := testService(t)
	if _, err := s.Store.db.Exec(`UPDATE records SET value='invalid-json' WHERE bucket='settings' AND id='main'`); err != nil {
		t.Fatal(err)
	}
	j := Job{ID: ID(), HostID: "host1", Kind: "probe", State: "queued"}
	s.Store.Put("jobs", j.ID, j)
	var calls atomic.Int32
	s.executeJob(context.Background(), j, func(context.Context) (string, error) { calls.Add(1); return "", nil })
	var got Job
	s.Store.Get("jobs", j.ID, &got)
	if calls.Load() != 0 || got.State != "failed" || got.Error == "" {
		t.Fatal("worker silently used fallback settings", calls.Load(), got)
	}
}

func TestQueuedWorkersHonorReducedParallelLimit(t *testing.T) {
	s := testService(t)
	settings, _ := s.Settings()
	settings.Parallel = 2
	settings.Retries = 0
	s.SaveSettings(settings)
	h, _ := s.Host("host1")
	for _, id := range []string{"host2", "host3"} {
		h.ID = id
		h.Name = "pve-" + id
		if err := s.SaveHost(h); err != nil {
			t.Fatal(err)
		}
	}
	gates := map[string]chan struct{}{"host1": make(chan struct{}), "host2": make(chan struct{}), "host3": make(chan struct{})}
	entered := make(chan string, 3)
	defer s.StopJobs(context.Background())
	run := func(id string) func(context.Context) (string, error) {
		return func(ctx context.Context) (string, error) {
			entered <- id
			select {
			case <-gates[id]:
				return "", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
	}
	for _, id := range []string{"host1", "host2"} {
		if _, err := s.queue(id, "probe", run(id)); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("workers did not start")
		}
	}
	if _, err := s.queue("host3", "probe", run("host3")); err != nil {
		t.Fatal(err)
	}
	// Let the existing worker enter its queue-wait loop before changing the limit.
	time.Sleep(250 * time.Millisecond)
	settings.Parallel = 1
	if err := s.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	close(gates["host1"])
	select {
	case id := <-entered:
		t.Fatal("queued worker ignored the updated limit", id)
	case <-time.After(350 * time.Millisecond):
	}
	close(gates["host2"])
	select {
	case id := <-entered:
		if id != "host3" {
			t.Fatal(id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("queued worker did not resume")
	}
	close(gates["host3"])
}

func TestSchedulerReportsMarkerFailureInStatus(t *testing.T) {
	s := testService(t)
	defer s.StopJobs(context.Background())
	s.Store.db.Exec(`INSERT INTO records(bucket,id,value) VALUES('schedule','host1','invalid-json')`)
	NewScheduler(s).Tick(time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC))
	var health map[string]string
	if err := s.Store.Get("health", "scheduler", &health); err != nil || !strings.Contains(health["error"], "host1") {
		t.Fatal("scheduler failure is invisible", health, err)
	}
}

func TestWorkerRejectsInvalidRetryCount(t *testing.T) {
	s := testService(t)
	settings, _ := s.Settings()
	settings.Retries = -1
	if err := s.Store.Put("settings", "main", settings); err != nil {
		t.Fatal(err)
	}
	j := Job{ID: ID(), HostID: "host1", Kind: "backup", State: "queued"}
	s.Store.Put("jobs", j.ID, j)
	var calls atomic.Int32
	s.executeJob(context.Background(), j, func(context.Context) (string, error) { calls.Add(1); return "", nil })
	got, _ := s.Job(j.ID)
	if calls.Load() != 0 || got.State != "failed" {
		t.Fatal("invalid retries silently reported success", got, calls.Load())
	}
}

func TestJobInsertFailureDoesNotConsumeTheScheduledDay(t *testing.T) {
	s := testService(t)
	defer s.StopJobs(context.Background())
	s.Store.db.Exec(`CREATE TRIGGER fail_job BEFORE INSERT ON records WHEN NEW.bucket='jobs' BEGIN SELECT RAISE(ABORT,'fixture job write failure'); END`)
	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	if err := NewScheduler(s).Tick(at); err == nil {
		t.Fatal("failed enqueue reported success")
	}
	var day string
	if err := s.Store.Get("schedule", "host1", &day); err == nil {
		t.Fatal("failed enqueue consumed daily marker", day)
	}
	s.Store.db.Exec(`DROP TRIGGER fail_job`)
	if err := NewScheduler(s).Tick(at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	jobs, err := s.Jobs()
	if err != nil || len(jobs) != 1 {
		t.Fatal("next tick did not retry enqueue", jobs, err)
	}
}

func TestSchedulerReportsMaintenanceMarkerWriteFailure(t *testing.T) {
	s := testService(t)
	defer s.StopJobs(context.Background())
	s.Store.db.Exec(`CREATE TRIGGER fail_maintenance BEFORE INSERT ON records WHEN NEW.bucket='maintenance' BEGIN SELECT RAISE(ABORT,'fixture maintenance marker failure'); END`)
	if err := NewScheduler(s).Tick(time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("maintenance marker failure swallowed")
	}
	var health map[string]string
	if err := s.Store.Get("health", "maintenance", &health); err != nil || health["error"] == "" {
		t.Fatal("maintenance failure invisible", health, err)
	}
}
