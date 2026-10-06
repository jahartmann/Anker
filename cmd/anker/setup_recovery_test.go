package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetupInterruptedConfigurationRestoresWholePreviousState(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, "service.env")
	os.WriteFile(env, []byte("ANKER_LISTEN=127.0.0.1:9090\n"), 0640)
	os.WriteFile(filepath.Join(dir, "setup-complete"), []byte("complete\n"), 0600)
	os.Mkdir(filepath.Join(dir, "tls"), 0750)
	cert := filepath.Join(dir, "tls", "server-123.crt")
	journal, err := setupBeginChange(dir, []string{cert}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a killed process after only some of the configuration was written.
	os.WriteFile(env, []byte("partial\n"), 0600)
	os.WriteFile(filepath.Join(dir, "tls-renewal.json"), []byte("partial\n"), 0600)
	os.WriteFile(cert, []byte("unused certificate"), 0640)
	loaded, err := setupLoadChange(dir)
	if err != nil || loaded == nil {
		t.Fatal("durable recovery record missing", err)
	}
	if err = loaded.rollback(dir); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(env)
	info, _ := os.Stat(env)
	if string(b) != "ANKER_LISTEN=127.0.0.1:9090\n" || info.Mode().Perm() != 0640 {
		t.Fatal("old configuration or permissions lost")
	}
	for _, name := range []string{"tls-renewal.json", "setup-pending.json", "tls/server-123.crt"} {
		if _, err = os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatal("partial file survived", name, err)
		}
	}
	if err = journal.rollback(dir); err != nil {
		t.Fatal("recovery is not repeatable", err)
	}
}

func TestSetupJournalNeverRemovesExistingIdentity(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "tls"), 0750)
	cert := filepath.Join(dir, "tls", "server-existing.crt")
	os.WriteFile(cert, []byte("existing"), 0640)
	if _, err := setupBeginChange(dir, []string{cert}, false, false); err == nil {
		t.Fatal("existing identity accepted as disposable")
	}
	if _, err := setupBeginChange(dir, []string{filepath.Join(dir, "keys", "backup")}, false, false); err == nil {
		t.Fatal("SSH key accepted as disposable")
	}
	b, _ := os.ReadFile(cert)
	if string(b) != "existing" {
		t.Fatal("identity changed")
	}
}
