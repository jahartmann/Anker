package anker

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The catalog connection provides a deterministic boundary after source/copy
// separation and before inventory. No product hooks or source mutations are used.
func TestIndependentCopyAncestorSwapRejectsSourceAndInvalidatesReceipt(t *testing.T) {
	s := testService(t)
	b, err := s.CreateBackup(context.Background(), "host1")
	if err != nil {
		t.Fatal(err)
	}
	validRoot := copyFixture(t, s, b)
	receipt, err := s.VerifyIndependentCopy(b.ID, validRoot)
	if err != nil || receipt.State != "ok" {
		t.Fatal("initial actual copy not verified", receipt, err)
	}
	sourceManifest, err := os.ReadFile(filepath.Join(s.backupDir(b), "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	sourceFile, err := os.ReadFile(filepath.Join(s.backupDir(b), "files/etc/network/interfaces"))
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(t.TempDir(), "copy-parent")
	root := filepath.Join(parent, b.ID)
	if err = os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	conn, err := s.Store.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.checkIndependentCopy(b, root) }()
	// Always release the blocked worker before the service/test directories close.
	released := false
	defer func() {
		conn.Close()
		if !released {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("blocked verifier did not stop")
			}
		}
	}()
	reached := false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		buf := make([]byte, 1<<20)
		stacks := string(buf[:runtime.Stack(buf, true)])
		for _, stack := range strings.Split(stacks, "\n\n") {
			if strings.Contains(stack, "checkIndependentCopy") && strings.Contains(stack, "database/sql.(*DB).conn") && strings.Contains(stack, "(*Service).Manifest") {
				reached = true
				break
			}
		}
		if reached {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !reached {
		t.Fatal("verifier did not reach controlled catalog boundary")
	}
	if err = os.Rename(parent, parent+"-original"); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(filepath.Dir(s.backupDir(b)), parent); err != nil {
		t.Fatal(err)
	}
	if err = conn.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		released = true
	case <-time.After(5 * time.Second):
		t.Fatal("verifier did not finish")
	}
	if err == nil {
		t.Error("source bytes accepted as independent copy after ancestor replacement")
	}
	// Leave the replacement in place and exercise the public receipt-writing path:
	// rejecting this same selected pathname must replace the earlier valid proof.
	receipt, err = s.VerifyIndependentCopy(b.ID, root)
	if err == nil || receipt.State != "failed" || receipt.VerifiedAt != "" {
		t.Fatal("rejected recheck retained success", receipt, err)
	}
	after, err := s.Backup(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.CopyVerification == nil || after.CopyVerification.State != "failed" || after.Status != b.Status || after.ManifestSHA != b.ManifestSHA {
		t.Fatal("receipt/source state changed incorrectly", after)
	}
	readiness, err := s.BackupReadiness(b.ID)
	if err != nil || readinessState(readiness.Items, "independent_copy") != "failed" {
		t.Fatal("old success remains in readiness", readiness, err)
	}
	manifestAfter, _ := os.ReadFile(filepath.Join(s.backupDir(b), "manifest.json"))
	fileAfter, _ := os.ReadFile(filepath.Join(s.backupDir(b), "files/etc/network/interfaces"))
	if string(manifestAfter) != string(sourceManifest) || string(fileAfter) != string(sourceFile) {
		t.Fatal("source tree changed")
	}
}
