package main

import (
	"anker/internal/anker"
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupRejectsInvalidExposureBeforeWriting(t *testing.T) {
	for _, listen := range []string{"0.0.0.0:8087", "bad address", "127.0.0.1:bad", "127.0.0.1:0"} {
		if _, err := setupWebEnv(listen, "", ""); err == nil {
			t.Fatalf("unsafe or invalid listen accepted: %s", listen)
		}
	}
	text, err := setupWebEnv("127.0.0.1:8087", "", "")
	if err != nil || text != "ANKER_LISTEN=127.0.0.1:8087\n" {
		t.Fatal(text, err)
	}
}

func TestSetupOnlyAcceptsPublicReleaseKeys(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize))
	if err := validateSetupKey(key); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"", "bad", base64.StdEncoding.EncodeToString(make([]byte, ed25519.PrivateKeySize))} {
		if err := validateSetupKey(invalid); err == nil {
			t.Fatal("invalid signing key accepted")
		}
	}
}
func TestSetupConfigValidationAndAtomicWrite(t *testing.T) {
	for _, bad := range []string{"a/b/c", "../Anker", "jahartmann/Anker\nOTHER=value", ""} {
		if err := validateSetupRepo(bad); err == nil {
			t.Fatal("repository accepted", bad)
		}
	}
	if err := validateSetupRepo("jahartmann/Anker"); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "service.env")
	if err := setupWrite(p, []byte("new-config"), 0600, os.Getuid(), os.Getgid()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(p)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal(info, err)
	}
	if got := setupEnvValue("ANKER_TLS_KEY=\"/etc/anker/tls/server key.pem\"\n", "ANKER_TLS_KEY"); got != "/etc/anker/tls/server key.pem" {
		t.Fatal(got)
	}
}
func TestSetupRepairsInterruptedSSHKeyWithoutReplacingIt(t *testing.T) {
	p := filepath.Join(t.TempDir(), "backup")
	if err := setupSSHKey(p, os.Getuid(), os.Getgid()); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(p + ".pub"); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(p, 0000); err != nil {
		t.Fatal(err)
	}
	if err = setupSSHKey(p, os.Getuid(), os.Getgid()); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(p)
	if err != nil || string(before) != string(after) {
		t.Fatal("existing private key replaced", err)
	}
	if _, err = os.Stat(p + ".pub"); err != nil {
		t.Fatal("public key not recovered", err)
	}
	if info, err := os.Stat(p); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("private key not protected", err)
	}
}

func TestSetupPromptDoesNotTreatEOFAsAnAnswer(t *testing.T) {
	if _, err := setupAnswer(strings.NewReader(""), "admin"); err == nil {
		t.Fatal("EOF accepted as confirmation")
	}
	r := strings.NewReader("\nsecond\n")
	if answer, err := setupAnswer(r, "admin"); err != nil || answer != "admin" {
		t.Fatal(answer, err)
	}
	if answer, err := setupAnswer(r, ""); err != nil || answer != "second" {
		t.Fatal("input consumed beyond prompt", answer, err)
	}
}

func TestInitDoesNotReplaceExistingAccess(t *testing.T) {
	root := t.TempDir()
	store, err := anker.OpenStore(filepath.Join(root, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	auth := anker.NewAuth(store)
	if err = auth.CreateUser("existing", "existing-test-password", "admin", true); err != nil {
		t.Fatal(err)
	}
	store.Close()
	t.Setenv("ANKER_INITIAL_PASSWORD", "")
	if err = run([]string{"--data", root, "init"}); err != nil {
		t.Fatal("repeated setup failed", err)
	}
	store, err = anker.OpenStore(filepath.Join(root, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, _, err = anker.NewAuth(store).Login("existing", "existing-test-password"); err != nil {
		t.Fatal("existing access changed", err)
	}
	var user anker.User
	if err = store.Get("users", "admin", &user); err == nil {
		t.Fatal("unexpected second admin")
	}
	if _, err = os.Stat(filepath.Join(root, "catalog.db")); err != nil {
		t.Fatal(err)
	}
}
