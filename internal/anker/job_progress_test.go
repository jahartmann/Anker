package anker

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"testing"
)

func TestBackupJobPublishesObservedStagesAndCounts(t *testing.T) {
	s := testService(t)
	gate := make(chan struct{})
	s.Collector = slowCollector{gate: gate}
	defer s.StopJobs(context.Background())
	j, err := s.QueueBackup("host1")
	if err != nil {
		t.Fatal(err)
	}
	inFlight := waitReliabilityJob(t, s, j.ID, func(j Job) bool { return len(j.Stages) > 0 })
	if inFlight.State != "running" || len(inFlight.Stages) != 1 || inFlight.Stages[0].ID != "collect" || inFlight.Stages[0].Total != 0 || inFlight.Stages[0].FinishedAt != "" {
		t.Fatal("unobserved work claimed", inFlight)
	}
	close(gate)
	done := waitReliabilityJob(t, s, j.ID, finishedJob)
	want := []string{"collect", "validate", "dependencies", "manifest", "publish"}
	if done.State != "successful" || len(done.Stages) != len(want) || done.Revision <= inFlight.Revision {
		t.Fatal(done)
	}
	for i, id := range want {
		stage := done.Stages[i]
		if stage.ID != id || stage.State != "successful" || stage.Attempt != 1 || stage.StartedAt == "" || stage.FinishedAt == "" {
			t.Fatal(stage)
		}
	}
	measured := done.Stages[1]
	if measured.Total < 1 || measured.Completed != measured.Total {
		t.Fatal("entry count not measured", measured)
	}
}
func TestJobStagesRetainFailureAcrossRetry(t *testing.T) {
	s := testService(t)
	s.Collector = &connectionCollector{failures: 1, failure: errTemporaryConnection}
	defer s.StopJobs(context.Background())
	j, err := s.QueueBackup("host1")
	if err != nil {
		t.Fatal(err)
	}
	backoff := waitReliabilityJob(t, s, j.ID, func(j Job) bool { return j.RetryAt != "" })
	if len(backoff.Stages) != 1 || backoff.Stages[0].State != "failed" {
		t.Fatal(backoff)
	}
	done := waitReliabilityJob(t, s, j.ID, finishedJob)
	if done.State != "successful" || len(done.Stages) != 6 || done.Stages[0].State != "failed" || done.Stages[0].Attempt != 1 || done.Stages[1].Attempt != 2 {
		t.Fatal(done)
	}
}
func TestJobStageCancellationAndRecoveryNeverClaimCompletion(t *testing.T) {
	s := testService(t)
	s.Collector = slowCollector{gate: make(chan struct{})}
	defer s.StopJobs(context.Background())
	j, err := s.QueueBackup("host1")
	if err != nil {
		t.Fatal(err)
	}
	waitReliabilityJob(t, s, j.ID, func(j Job) bool { return len(j.Stages) > 0 })
	if err := s.CancelJob(j.ID); err != nil {
		t.Fatal(err)
	}
	done := waitReliabilityJob(t, s, j.ID, finishedJob)
	if done.State != "cancelled" || len(done.Stages) != 1 || done.Stages[0].State != "cancelled" {
		t.Fatal(done)
	}
	interrupted := Job{ID: "orphan", Kind: "backup", State: "running", Stages: []JobStage{{ID: "collect", State: "running", Attempt: 1}}}
	if err := s.Store.Put("jobs", interrupted.ID, interrupted); err != nil {
		t.Fatal(err)
	}
	if err := s.RecoverJobs(); err != nil {
		t.Fatal(err)
	}
	recovered, err := s.Job(interrupted.ID)
	if err != nil || recovered.Stages[0].State != "interrupted" || recovered.Stages[0].FinishedAt == "" {
		t.Fatal(recovered, err)
	}
}

func TestReceiveStageRequiresActualPackageBytes(t *testing.T) {
	for _, bytesArrive := range []bool{false, true} {
		t.Run(fmt.Sprint(bytesArrive), func(t *testing.T) {
			var data bytes.Buffer
			if bytesArrive {
				tw := tar.NewWriter(&data)
				if err := tw.WriteHeader(&tar.Header{Name: "collection.json", Typeflag: tar.TypeReg, Mode: 0600, Size: 2}); err != nil {
					t.Fatal(err)
				}
				if _, err := tw.Write([]byte("{}")); err != nil {
					t.Fatal(err)
				}
				if err := tw.Close(); err != nil {
					t.Fatal(err)
				}
			}
			reports := 0
			ctx := context.WithValue(context.Background(), jobProgressKey{}, jobReporter(func(id, title, detail string, n, total int) error {
				reports++
				if id != "receive" || n != 0 || total != 0 {
					t.Fatal(id, n, total)
				}
				return nil
			}))
			err := receiveCollection(ctx, &data, t.TempDir())
			if bytesArrive && (err != nil || reports != 1) {
				t.Fatal(err, reports)
			}
			if !bytesArrive && (err == nil || reports != 0) {
				t.Fatal("transfer claimed without payload", err, reports)
			}
		})
	}
}
