package main

import (
	"anker/internal/anker"
	"os"
	"path/filepath"
	"testing"
)

func TestSetupRecognizesExistingAccessWithoutChangingCatalog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.db")
	initialized, err := setupInitialized(path)
	if err != nil || initialized {
		t.Fatal(initialized, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("probe created catalog", err)
	}
	store, err := anker.OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := anker.NewAuth(store).CreateUser("existing", "existing-setup-password", "admin", true); err != nil {
		t.Fatal(err)
	}
	store.Close()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err = setupInitialized(path)
	if err != nil || !initialized {
		t.Fatal(initialized, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("probe changed catalog", err)
	}
}
