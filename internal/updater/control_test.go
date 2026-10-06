package updater

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestPreparePersistsGuardBeforeDrainAndCleansRejection(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "anker-guard-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	guard := filepath.Join(root, "guard")
	socket := filepath.Join(root, "socket")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	reject := true
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/update-control/prepare" {
			if _, err := os.Stat(guard); err != nil {
				t.Error("guard missing before drain")
			}
			if reject {
				w.WriteHeader(409)
				w.Write([]byte(`{"error":"busy"}`))
				return
			}
		}
		w.Write([]byte(`{"version":"0.2.0"}`))
	})}
	defer server.Close()
	go server.Serve(listener)
	c := SystemControl{Socket: socket, guard: guard}
	if _, err := c.Prepare(context.Background()); err == nil {
		t.Fatal("busy drain accepted")
	}
	if _, err := os.Stat(guard); !os.IsNotExist(err) {
		t.Fatal("rejected drain retains guard")
	}
	reject = false
	if _, err := c.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(guard); err != nil {
		t.Fatal("acknowledged drain not persistent")
	}
	if err := c.Release(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseKeepsGuardUntilServiceAcknowledges(t *testing.T) {
	root := t.TempDir()
	guard := filepath.Join(root, "guard")
	if err := os.WriteFile(guard, []byte("update\n"), 0640); err != nil {
		t.Fatal(err)
	}
	c := SystemControl{Socket: filepath.Join(root, "missing.sock"), guard: guard}
	if err := c.Release(context.Background()); err == nil {
		t.Fatal("offline service acknowledged release")
	}
	if _, err := os.Stat(guard); err != nil {
		t.Fatal("retry marker removed before acknowledgement", err)
	}
}
