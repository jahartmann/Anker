package anker

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

type connectionCollector struct {
	fixtureCollector
	calls    atomic.Int32
	failures int32
	failure  error
}

func (c *connectionCollector) Collect(ctx context.Context, h Host, d string) (Collection, error) {
	if c.calls.Add(1) <= c.failures {
		return Collection{}, c.failure
	}
	return c.fixtureCollector.Collect(ctx, h, d)
}
func waitReliabilityJob(t *testing.T, s *Service, id string, condition func(Job) bool) Job {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		j, err := s.Job(id)
		if err != nil {
			t.Fatal(err)
		}
		if condition(j) {
			return j
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("job did not reach expected state")
	return Job{}
}
func TestConnectionRetriesRemainOneJobAndOneCapture(t *testing.T) {
	s := testService(t)
	c := &connectionCollector{failures: 1, failure: fmt.Errorf("transport: %w", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED})}
	s.Collector = c
	defer s.StopJobs(context.Background())
	j, err := s.QueueBackup("host1")
	if err != nil {
		t.Fatal(err)
	}
	waiting := waitReliabilityJob(t, s, j.ID, func(j Job) bool { return j.RetryAt != "" })
	if waiting.Attempts != 1 || waiting.LastError == "" || waiting.MaxAttempts != 4 || !strings.Contains(waiting.Message, "gleicher Auftrag") {
		t.Fatal(waiting)
	}
	if _, err = s.QueueBackup("host1"); err == nil {
		t.Fatal("duplicate admitted during backoff")
	}
	done := waitReliabilityJob(t, s, j.ID, finishedJob)
	jobs, _ := s.Jobs()
	backups, _ := s.ListBackups("host1")
	if done.State != "successful" || done.Attempts != 2 || done.RetryAt != "" || done.Revision <= waiting.Revision || len(jobs) != 1 || len(backups) != 1 {
		t.Fatal(done, len(jobs), len(backups))
	}
}
func TestConnectionRetryLimitAndPermanentFailures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failure error
		retries int
		want    int
	}{
		{"limit", errTemporaryConnection, 2, 3}, {"disabled", errTemporaryConnection, 0, 1}, {"authorization", errors.New("permission denied"), 3, 1}, {"malformed", errors.New("unexpected EOF"), 3, 1}, {"text-is-not-classification", errors.New("connection refused"), 3, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testService(t)
			c := &connectionCollector{failures: 99, failure: tc.failure}
			s.Collector = c
			defer s.StopJobs(context.Background())
			settings, _ := s.Settings()
			settings.Retries = tc.retries
			if err := s.SaveSettings(settings); err != nil {
				t.Fatal(err)
			}
			j, err := s.QueueBackup("host1")
			if err != nil {
				t.Fatal(err)
			}
			done := waitReliabilityJob(t, s, j.ID, finishedJob)
			if done.State != "failed" || done.Attempts != tc.want || int(c.calls.Load()) != tc.want || done.RetryAt != "" {
				t.Fatal(done, c.calls.Load())
			}
		})
	}
}
func TestCancellationDuringConnectionBackoffStopsRetry(t *testing.T) {
	s := testService(t)
	c := &connectionCollector{failures: 99, failure: errTemporaryConnection}
	s.Collector = c
	j, err := s.QueueBackup("host1")
	if err != nil {
		t.Fatal(err)
	}
	waitReliabilityJob(t, s, j.ID, func(j Job) bool { return j.RetryAt != "" })
	if err = s.CancelJob(j.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.StopJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	done, _ := s.Job(j.ID)
	if done.State != "cancelled" || done.RetryAt != "" || c.calls.Load() != 1 {
		t.Fatal(done, c.calls.Load())
	}
}
func TestRetryRequestIsIdempotentEvenAfterChildCompletes(t *testing.T) {
	s := testService(t)
	defer s.StopJobs(context.Background())
	original := Job{ID: ID(), HostID: "host1", Kind: "backup", State: "failed"}
	if err := s.Store.Put("jobs", original.ID, original); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	ids := make(chan string, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			j, err := s.RetryJob(original.ID)
			if err != nil {
				t.Error(err)
				return
			}
			ids <- j.ID
		}()
	}
	wg.Wait()
	close(ids)
	child := ""
	for id := range ids {
		if child != "" && child != id {
			t.Fatal("duplicate child", child, id)
		}
		child = id
	}
	waitReliabilityJob(t, s, child, finishedJob)
	repeated, err := s.RetryJob(original.ID)
	if err != nil || repeated.ID != child {
		t.Fatal(repeated, err)
	}
	jobs, _ := s.Jobs()
	backups, _ := s.ListBackups("host1")
	if len(jobs) != 2 || len(backups) != 1 {
		t.Fatal(len(jobs), len(backups))
	}
}
func TestRestartExplainsQueuedAndUnconfirmedRunningWithoutReplay(t *testing.T) {
	s := testService(t)
	for _, state := range []string{"queued", "running"} {
		j := Job{ID: state, HostID: "host1", Kind: "backup", State: state, Attempts: 1, RetryAt: now()}
		if err := s.Store.Put("jobs", j.ID, j); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RecoverJobs(); err != nil {
		t.Fatal(err)
	}
	queued, _ := s.Job("queued")
	running, _ := s.Job("running")
	if queued.State != "interrupted" || !strings.Contains(queued.Message, "Warteschlange") || running.State != "interrupted" || !strings.Contains(running.Message, "Abschluss nicht bestätigt") || running.RetryAt != "" {
		t.Fatal(queued, running)
	}
	if err := s.RecoverJobs(); err != nil {
		t.Fatal(err)
	}
	again, _ := s.Job("running")
	jobs, _ := s.Jobs()
	if again.FinishedAt != running.FinishedAt || len(jobs) != 2 {
		t.Fatal(again, len(jobs))
	}
}
func TestPublishedResultNeverRetriesAndRestoreNeverRetries(t *testing.T) {
	for _, kind := range []string{"backup", "restore"} {
		t.Run(kind, func(t *testing.T) {
			s := testService(t)
			defer s.StopJobs(context.Background())
			var calls atomic.Int32
			j, err := s.queue("host1", kind, func(context.Context) (string, error) {
				calls.Add(1)
				if kind == "backup" {
					return "published-result", errTemporaryConnection
				}
				return "", errTemporaryConnection
			})
			if err != nil {
				t.Fatal(err)
			}
			waitReliabilityJob(t, s, j.ID, finishedJob)
			if calls.Load() != 1 {
				t.Fatal(calls.Load())
			}
		})
	}
}
func TestSSHExitClassificationKeepsIdentityAndRightsFailuresPermanent(t *testing.T) {
	exit := exec.Command("sh", "-c", "exit 255").Run()
	for _, tc := range []struct {
		text string
		want bool
	}{{"ssh: connect to host: Connection refused", true}, {"Read from remote host: Connection reset by peer", true}, {"Host key verification failed. Connection closed by remote host", false}, {"Permission denied (publickey). Connection closed by remote host", false}, {"remote helper failed", false}} {
		if got := sshConnectionFailure(exit, tc.text); got != tc.want {
			t.Fatalf("%q: %v", tc.text, got)
		}
	}
	if sshConnectionFailure(exec.Command("sh", "-c", "exit 1").Run(), "Connection refused") {
		t.Fatal("remote command failure retried")
	}
}

func TestGracefulShutdownPersistsReasonForNextStartup(t *testing.T) {
	s := testService(t)
	s.Collector = slowCollector{gate: make(chan struct{})}
	j, err := s.QueueBackup("host1")
	if err != nil {
		t.Fatal(err)
	}
	waitReliabilityJob(t, s, j.ID, func(j Job) bool { return j.State == "running" })
	if err = s.StopJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	stopped, _ := s.Job(j.ID)
	if stopped.State != "cancelled" || !strings.Contains(stopped.Message, "Dienst wurde beendet") {
		t.Fatal(stopped)
	}
	if err = s.RecoverJobs(); err != nil {
		t.Fatal(err)
	}
	again, _ := s.Job(j.ID)
	if again.Message != stopped.Message {
		t.Fatal(again)
	}
}
