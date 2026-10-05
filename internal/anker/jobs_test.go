package anker

import (
	"context"
	"sync"
	"testing"
	"time"
)

type slowCollector struct {
	fixtureCollector
	gate chan struct{}
	once *sync.Once
}

func (c slowCollector) Collect(ctx context.Context, h Host, d string) (Collection, error) {
	select {
	case <-c.gate:
	case <-ctx.Done():
		return Collection{}, ctx.Err()
	}
	return c.fixtureCollector.Collect(ctx, h, d)
}
func TestJobsRejectOverlap(t *testing.T) {
	s := testService(t)
	gate := make(chan struct{})
	s.Collector = slowCollector{gate: gate}
	j, err := s.QueueBackup("host1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.QueueBackup("host1"); err == nil {
		t.Fatal("overlap permitted")
	}
	close(gate)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var v Job
		s.Store.Get("jobs", j.ID, &v)
		if v.State == "successful" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("job did not complete")
}
func TestRecoveryMarksInterruptedJobs(t *testing.T) {
	s := testService(t)
	s.Store.Put("jobs", "job1", Job{ID: "job1", State: "running", Kind: "restore"})
	if err := s.RecoverJobs(); err != nil {
		t.Fatal(err)
	}
	var j Job
	s.Store.Get("jobs", "job1", &j)
	if j.State != "interrupted" {
		t.Fatal(j)
	}
}
func TestSchedulerRunsOncePerLocalDay(t *testing.T) {
	s := testService(t)
	h, _ := s.Host("host1")
	h.Schedule = "02:00"
	s.SaveHost(h)
	sch := NewScheduler(s)
	loc, _ := time.LoadLocation("Europe/Berlin")
	at := time.Date(2026, 10, 25, 3, 30, 0, 0, loc)
	if err := sch.Tick(at); err != nil {
		t.Fatal(err)
	}
	if err := sch.Tick(at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	jobs, _ := s.Jobs()
	if len(jobs) != 1 {
		t.Fatal(len(jobs))
	}
	time.Sleep(200 * time.Millisecond)
}
