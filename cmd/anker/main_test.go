package main

import (
	"anker/internal/anker"
	"path/filepath"
	"testing"
)

func TestSecondInstanceDoesNotInterruptLiveJobs(t *testing.T) {
	root := t.TempDir()
	release, e := anker.InstanceLock(root)
	if e != nil {
		t.Fatal(e)
	}
	defer release()
	store, e := anker.OpenStore(filepath.Join(root, "catalog.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	store.Put("jobs", "live", anker.Job{ID: "live", State: "running"})
	if e = run([]string{"--data", root, "serve"}); e == nil {
		t.Fatal("second instance admitted")
	}
	var job anker.Job
	if e = store.Get("jobs", "live", &job); e != nil || job.State != "running" {
		t.Fatal("live job was changed", job, e)
	}
}
