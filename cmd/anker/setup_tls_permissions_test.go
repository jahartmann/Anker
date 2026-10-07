package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestSetupTLSDirectoryReadableWithPrivateUmask(t *testing.T) {
	if os.Getenv("ANKER_TLS_UMASK_CHILD") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestSetupTLSDirectoryReadableWithPrivateUmask$")
		cmd.Env = append(os.Environ(), "ANKER_TLS_UMASK_CHILD=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("private umask setup: %v\n%s", err, out)
		}
		return
	}
	unix.Umask(0077) // The real installer passes this mask to its setup child.
	dir := filepath.Join(t.TempDir(), "tls")
	if err := setupTLSDirectory(dir, os.Getgid()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil || info.Mode().Perm() != 0750 {
		t.Fatalf("service group cannot traverse TLS directory: %v, %v", info, err)
	}
}

func TestSetupRepairsTLSDirectoryWithoutReplacingIdentity(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tls")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(dir, "server.key")
	if err := os.WriteFile(key, []byte("existing private key"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := setupTLSDirectory(dir, os.Getgid()); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(dir)
	if info.Mode().Perm() != 0750 {
		t.Fatal("existing root-only TLS directory was not repaired")
	}
	b, err := os.ReadFile(key)
	if err != nil || string(b) != "existing private key" {
		t.Fatal("repair changed existing private key", err)
	}
}

func TestSetupTLSDirectoryRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "private")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "tls")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := setupTLSDirectory(link, os.Getgid()); err == nil {
		t.Fatal("symlink accepted as managed TLS directory")
	}
	info, _ := os.Stat(target)
	if info.Mode().Perm() != 0700 {
		t.Fatal("permissions on an unrelated directory changed")
	}
}
