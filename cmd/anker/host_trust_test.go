package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const trustFixtureKey = "AAAAC3NzaC1lZDI1NTE5AAAAIAABAgMEBQYHCAkKCwwNDg8QERITFBUWFxgZGhscHR4f"

func TestHostTrustRequiresMatchingIndependentFingerprint(t *testing.T) {
	const fingerprint = "SHA256:ZkAslGjFiUHdGf/WUL8rQvkib4PTvQatUV0OUQSncCA"
	scan := "# banner\n192.0.2.10 ssh-ed25519 " + trustFixtureKey + "\n"
	line, err := verifiedHostKey("pve.internal", 2222, fingerprint, scan)
	if err != nil || line != "[pve.internal]:2222 ssh-ed25519 "+trustFixtureKey+"\n" {
		t.Fatal(line, err)
	}
	if _, err := verifiedHostKey("pve.internal", 22, "SHA256:other", scan); err == nil {
		t.Fatal("unverified key accepted")
	}
	if _, err := verifiedHostKey("pve.internal", 22, fingerprint, "192.0.2.10 ssh-ed25519 YWJj\n"); err == nil {
		t.Fatal("malformed key accepted")
	}
}

func TestHostTrustReplacesOnlyThisHostAndIsRepeatable(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "known_hosts")
	if err := os.WriteFile(path, []byte("other.internal ssh-ed25519 "+trustFixtureKey+"\npve.internal ssh-ed25519 "+trustFixtureKey+"\n"), 0640); err != nil {
		t.Fatal(err)
	}
	line := "pve.internal ssh-ed25519 " + trustFixtureKey + "\n"
	for range 2 {
		if err := saveTrustedHost(path, "pve.internal", line, os.Getuid(), os.Getgid()); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.Count(string(data), "pve.internal") != 1 || !strings.Contains(string(data), "other.internal") {
		t.Fatal(string(data), err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0640 {
		t.Fatal(info, err)
	}
	shared := []byte("pve.internal,other.internal ssh-ed25519 " + trustFixtureKey + "\n")
	if err := os.WriteFile(path, shared, 0640); err != nil {
		t.Fatal(err)
	}
	if err := saveTrustedHost(path, "pve.internal", line, os.Getuid(), os.Getgid()); err == nil {
		t.Fatal("shared identity silently replaced")
	}
	data, err = os.ReadFile(path)
	if err != nil || string(data) != string(shared) {
		t.Fatal("shared identity changed", err)
	}
	target := filepath.Join(root, "untouched")
	if err := os.WriteFile(target, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if err := saveTrustedHost(path, "pve.internal", line, os.Getuid(), os.Getgid()); err == nil {
		t.Fatal("known_hosts symlink accepted")
	}
	data, err = os.ReadFile(target)
	if err != nil || string(data) != "keep" {
		t.Fatal("linked file changed", err)
	}
}
