package main

import (
	"anker/internal/anker"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDisasterOfflineDispatchAndSecretFile(t *testing.T) {
	dir := t.TempDir()
	pass := filepath.Join(dir, "pass")
	os.WriteFile(pass, []byte("secret\n"), 0600)
	err := runWithTerminal([]string{"--data", filepath.Join(dir, "absent"), "disaster", "export", "--output", filepath.Join(dir, "out"), "--passphrase-file", pass}, false, nil)
	if err == nil || strings.Contains(err.Error(), "socket") || strings.Contains(err.Error(), "Dienst nicht") {
		t.Fatalf("offline command not dispatched: %v", err)
	}
	got, err := disasterPassphrase(pass)
	if err != nil || string(got) != "secret" {
		t.Fatalf("secret: %v", err)
	}
	os.Chmod(pass, 0644)
	if _, err = disasterPassphrase(pass); err == nil {
		t.Fatal("world-readable passphrase")
	}
	os.Chmod(pass, 0600)
	link := filepath.Join(dir, "link")
	os.Symlink(pass, link)
	if _, err = disasterPassphrase(link); err == nil {
		t.Fatal("symlink secret")
	}
	if _, err = os.Stat(filepath.Join(dir, "out")); !os.IsNotExist(err) {
		t.Fatal("failed export left output")
	}
}
func TestDisasterRejectsCommandLinePassphrase(t *testing.T) {
	if err := runWithTerminal([]string{"disaster", "inspect", "--input", "absent", "--passphrase", "secret"}, false, nil); err == nil {
		t.Fatal("CLI secret accepted")
	}
}

func TestDisasterOfflineCLIRoundTripNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	os.Mkdir(source, 0700)
	store, err := anker.OpenStore(filepath.Join(source, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Put("credentials", "admin", map[string]string{"hash": "secret-hash"}); err != nil {
		t.Fatal(err)
	}
	store.Close()
	if err = anker.EnsureDataMode(source, false); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(dir, "pass")
	os.WriteFile(secret, []byte("long-random-test-passphrase\n"), 0600)
	bundle := filepath.Join(dir, "bundle")
	export := []string{"--data", source, "disaster", "export", "--output", bundle, "--passphrase-file", secret}
	if err = runWithTerminal(export, false, nil); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(bundle)
	if err = runWithTerminal(export, false, nil); err == nil {
		t.Fatal("existing package overwritten")
	}
	after, _ := os.ReadFile(bundle)
	if !bytes.Equal(before, after) {
		t.Fatal("package changed")
	}
	if err = runWithTerminal([]string{"disaster", "inspect", "--input", bundle, "--passphrase-file", secret}, false, nil); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "new")
	if err = runWithTerminal([]string{"disaster", "import", "--input", bundle, "--destination", dest, "--passphrase-file", secret}, false, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(dest, ".disaster-isolated")); err != nil {
		t.Fatal(err)
	}
	if err = runWithTerminal([]string{"--data", dest, "init"}, false, nil); err == nil || !strings.Contains(err.Error(), "Isolierter") {
		t.Fatal("import activated", err)
	}
}

func TestDisasterCLIMissingSourceAndSeparateMode(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	os.Mkdir(source, 0700)
	store, err := anker.OpenStore(filepath.Join(source, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Put("backups", "missing", anker.Backup{ID: "missing", HostID: "host", ManifestSHA: anker.Hash([]byte("known manifest"))}); err != nil {
		t.Fatal(err)
	}
	store.Close()
	if err = anker.EnsureDataMode(source, false); err != nil {
		t.Fatal(err)
	}
	pass := filepath.Join(dir, "pass")
	os.WriteFile(pass, []byte("test-pass"), 0600)
	for _, mode := range []string{"include", "separate"} {
		bundle := filepath.Join(dir, "bundle-"+mode)
		if err = runDisaster(source, []string{"export", "--output", bundle, "--backup-data", mode, "--passphrase-file", pass}); err != nil {
			t.Fatal(err)
		}
		f, err := os.Open(bundle)
		if err != nil {
			t.Fatal(err)
		}
		info, err := anker.InspectDisaster(f, []byte("test-pass"))
		f.Close()
		if err != nil || info.MissingSources != 1 {
			t.Fatal("CLI source inventory", info, err)
		}
	}
}
