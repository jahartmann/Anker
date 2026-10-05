package anker

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type extendedCollector struct {
	fixtureCollector
	large bool
}

func (c extendedCollector) Collect(ctx context.Context, h Host, d string) (Collection, error) {
	v, e := c.fixtureCollector.Collect(ctx, h, d)
	data := []byte("first\n")
	if c.large {
		data = bytes.Repeat([]byte("x"), 9<<20)
	}
	for _, p := range []string{"etc/aaa.conf", "etc/zzz.conf"} {
		os.WriteFile(filepath.Join(d, "files", p), data, 0600)
		v.Entries = append(v.Entries, Entry{Path: p, Type: "file", Mode: 0600})
	}
	return v, e
}
func TestSelectedFileNeverIncludesLaterEntries(t *testing.T) {
	s := testService(t)
	s.Collector = extendedCollector{}
	b, e := s.CreateBackup(context.Background(), "host1")
	if e != nil {
		t.Fatal(e)
	}
	s.Collector = targetCollector{inv: Inventory{PVEVersion: "8.4", Details: map[string]json.RawMessage{"file_hashes": json.RawMessage(`{}`)}}}
	s.SaveHost(Host{ID: "target", Name: "target", Address: "192.0.2.2"})
	p, e := s.CreatePlan(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{"etc/aaa.conf"}})
	if e != nil || len(p.Steps) != 1 || p.Steps[0].Path != "etc/aaa.conf" {
		t.Fatalf("unselected steps: %+v %v", p.Steps, e)
	}
}
func TestRecoveryReadsExceedPreviewLimit(t *testing.T) {
	s := testService(t)
	s.Collector = extendedCollector{large: true}
	b, e := s.CreateBackup(context.Background(), "host1")
	if e != nil {
		t.Fatal(e)
	}
	s.Collector = targetCollector{inv: Inventory{PVEVersion: "8.4", Details: map[string]json.RawMessage{"file_hashes": json.RawMessage(`{}`)}}}
	s.SaveHost(Host{ID: "target", Name: "target", Address: "192.0.2.2"})
	if _, _, e = s.ReadFile(b.ID, "etc/aaa.conf"); e == nil {
		t.Fatal("preview bound missing")
	}
	p, e := s.CreatePlan(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{"etc/aaa.conf"}})
	if e != nil || len(p.Steps) != 1 {
		t.Fatal(e, len(p.Steps))
	}
}
func TestReindexDoesNotPublishCorruptCandidate(t *testing.T) {
	s := testService(t)
	b, _ := s.CreateBackup(context.Background(), "host1")
	s.Store.Delete("backups", b.ID)
	os.WriteFile(filepath.Join(s.backupDir(b), "files/etc/network/interfaces"), []byte("bad"), 0600)
	if _, e := s.Reindex(); e == nil {
		t.Fatal("corruption accepted")
	}
	if _, e := s.Backup(b.ID); e == nil {
		t.Fatal("corrupt candidate published")
	}
}

type pausedWriter struct {
	first   chan struct{}
	resume  chan struct{}
	started bool
	buf     bytes.Buffer
}

func (w *pausedWriter) Write(p []byte) (int, error) {
	if !w.started {
		w.started = true
		close(w.first)
		<-w.resume
	}
	return w.buf.Write(p)
}
func TestArchiveWaitsForActiveExport(t *testing.T) {
	s := testService(t)
	b, _ := s.CreateBackup(context.Background(), "host1")
	w := &pausedWriter{first: make(chan struct{}), resume: make(chan struct{})}
	export := make(chan error, 1)
	go func() { export <- s.ExportBackup(b.ID, w) }()
	<-w.first
	archived := make(chan error, 1)
	go func() { archived <- s.ArchiveBackup(b.ID) }()
	select {
	case e := <-archived:
		close(w.resume)
		t.Fatal("archive did not wait", e)
	case <-time.After(50 * time.Millisecond):
	}
	close(w.resume)
	if e := <-export; e != nil {
		t.Fatal(e)
	}
	if e := <-archived; e != nil {
		t.Fatal(e)
	}
	if w.buf.Len() == 0 {
		t.Fatal(io.ErrUnexpectedEOF)
	}
}
func TestExclusiveDataRootLock(t *testing.T) {
	root := t.TempDir()
	release, e := InstanceLock(root)
	if e != nil {
		t.Fatal(e)
	}
	if other, e := InstanceLock(root); e == nil {
		other()
		t.Fatal("second daemon admitted")
	}
	release()
	release, e = InstanceLock(root)
	if e != nil {
		t.Fatal(e)
	}
	release()
}
