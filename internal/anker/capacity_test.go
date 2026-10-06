package anker

import (
	"anker/internal/storage"
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSchedulerQueuesBackupsWhileStorageSamplingIsBusy(t *testing.T) {
	s := testService(t)
	s.storageMu.Lock()
	done := make(chan error, 1)
	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	go func() { done <- NewScheduler(s).Tick(at) }()
	var err error
	blocked := false
	select {
	case err = <-done:
	case <-time.After(time.Second):
		blocked = true
	}
	s.storageMu.Unlock()
	if blocked {
		<-done
		t.Fatal("blocked storage sampler delayed the backup schedule")
	}
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := s.Jobs()
	if err != nil || len(jobs) != 1 {
		t.Fatal("backup was not queued", jobs, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err = s.StopJobs(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestStorageMeasurementsAreHourlyPersistentAndResetOnExpansion(t *testing.T) {
	s := testService(t)
	at := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	report := storage.Report{Volumes: []storage.Volume{{ID: "data", Total: 1000, Used: 500, Available: 450}}}
	for i := 0; i < 96; i++ {
		if err := s.recordStorage(report, at.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.recordStorage(report, at.Add(95*time.Hour+30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	var h storageHistory
	if err := s.Store.Get("storage", "history", &h); err != nil {
		t.Fatal(err)
	}
	if len(h.Samples["data"]) != 96 {
		t.Fatalf("history duplicates or missing: %d", len(h.Samples["data"]))
	}
	report.Volumes[0].Total = 2000
	if err := s.recordStorage(report, at.Add(96*time.Hour)); err != nil {
		t.Fatal(err)
	}
	s.Store.Get("storage", "history", &h)
	if len(h.Samples["data"]) != 1 || h.Samples["data"][0].Total != 2000 {
		t.Fatal("growth retained old capacity history", h)
	}
	if err := s.recordStorage(report, at.Add(190*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	s.Store.Get("storage", "history", &h)
	if len(h.Samples["data"]) != 1 {
		t.Fatal("expired history retained", h)
	}
}

func TestStorageAPIRequiresAdminAndDemoNeverReadsOrChangesDevices(t *testing.T) {
	s := testService(t)
	s.Demo = true
	for _, role := range []string{"reader", "restore", "admin"} {
		r := httptest.NewRequest("GET", "/api/storage", nil)
		w := httptest.NewRecorder()
		err := s.capacity(w, r, User{Role: role})
		if role != "admin" {
			if err == nil {
				t.Fatal("non-admin can inspect machine", role)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		var got storage.Report
		if err = json.Unmarshal(w.Body.Bytes(), &got); err != nil || !got.Demo || len(got.Volumes) != 0 || len(got.Devices) != 0 {
			t.Fatal("demo machine leaked", w.Body.String(), err)
		}
	}
	for _, path := range []string{"/api/storage/plan", "/api/storage/grow"} {
		if err := s.capacity(httptest.NewRecorder(), httptest.NewRequest("POST", path, bytes.NewBufferString(`{}`)), User{Role: "admin"}); err == nil {
			t.Fatal("demo allowed device action", path)
		}
	}
}

func TestTemporaryMountFailurePreservesEarlierMeasurements(t *testing.T) {
	s := testService(t)
	at := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	r := storage.Report{Volumes: []storage.Volume{{ID: "backup", Total: 1000, Used: 500, Available: 450}}}
	if err := s.recordStorage(r, at); err != nil {
		t.Fatal(err)
	}
	failed := storage.Report{Volumes: []storage.Volume{{ID: "backup", Error: "Mount nicht erreichbar"}}}
	if err := s.recordStorage(failed, at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.recordStorage(r, at.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	h, err := s.storageHistory()
	if err != nil || len(h.Samples["backup"]) != 2 {
		t.Fatal("temporary mount failure erased valid history", h, err)
	}
}
