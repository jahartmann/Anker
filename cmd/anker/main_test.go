package main

import (
	"anker/internal/anker"
	"os"
	"path/filepath"
	"strings"
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

func TestDemoCannotSeedProduction(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ANKER_INITIAL_PASSWORD", "production-test-password")
	if err := run([]string{"--data", root, "init"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(root, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	err = run([]string{"--data", root, "--listen", "invalid", "demo"})
	if err == nil || !strings.Contains(err.Error(), "Produktions") {
		t.Fatalf("production directory accepted for demo: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(root, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("production catalog changed")
	}
	if _, err := os.Stat(filepath.Join(root, "demo-hosts")); !os.IsNotExist(err) {
		t.Fatal("demo files appeared in production", err)
	}
}

func TestProductionRejectsLegacyDemoBeforeOpeningCatalog(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "demo-hosts"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANKER_INITIAL_PASSWORD", "production-test-password")
	for _, command := range []string{"init", "serve"} {
		err := run([]string{"--data", root, "--listen", "invalid", command})
		if err == nil || !strings.Contains(err.Error(), "Demo") {
			t.Fatalf("legacy demo accepted: %v", err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "catalog.db")); !os.IsNotExist(err) {
		t.Fatal("production opened legacy demo catalog", err)
	}
}

func TestDemoRejectsSharedSocketAndPublicListener(t *testing.T) {
	for _, tc := range []struct {
		args   []string
		reason string
	}{
		{[]string{"--socket", filepath.Join(t.TempDir(), "production.sock")}, "Demo-Socket"},
		{[]string{"--listen", "0.0.0.0:8087", "--allow-http"}, "Loopback"},
		{[]string{"--data", "/srv/anker"}, "Produktionsverzeichnis"},
		{[]string{"--data", "/srv"}, "Produktionsverzeichnis"},
		{[]string{"--data", "/"}, "Produktionsverzeichnis"},
	} {
		root := filepath.Join(t.TempDir(), "demo")
		command := append([]string{"--data", root}, tc.args...)
		command = append(command, "demo")
		if err := run(command); err == nil || !strings.Contains(err.Error(), tc.reason) {
			t.Fatal("unsafe demo accepted", err)
		}
		if _, err := os.Stat(filepath.Join(root, "catalog.db")); !os.IsNotExist(err) {
			t.Fatal("unsafe demo created data before rejection", err)
		}
	}
}

func TestImplicitDemoIgnoresProductionEnvironment(t *testing.T) {
	root := t.TempDir()
	production := filepath.Join(root, "production")
	t.Setenv("ANKER_INITIAL_PASSWORD", "production-test-password")
	if err := run([]string{"--data", production, "init"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(production, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANKER_DATA", production)
	t.Chdir(root)
	if err := run([]string{"--listen", "invalid", "demo"}); err == nil {
		t.Fatal("invalid listener accepted")
	}
	after, err := os.ReadFile(filepath.Join(production, "catalog.db"))
	if err != nil || string(before) != string(after) {
		t.Fatal("environment-selected production changed", err)
	}
	marker, err := os.ReadFile(filepath.Join(root, "var/demo/.anker-mode"))
	if err != nil || string(marker) != "demo\n" {
		t.Fatal("implicit demo did not use its own directory", err)
	}
}

func TestDemoRejectsProductionSymlink(t *testing.T) {
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink("/srv/anker", alias); err != nil {
		t.Fatal(err)
	}
	err := run([]string{"--data", alias, "demo"})
	if err == nil {
		t.Fatal("production symlink accepted")
	}
}
