package updater

import (
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPendingSetupBlocksSystemChangesUntilRecovery(t *testing.T) {
	manager, cert, _ := tlsFixture(t, 10, false)
	before, _ := os.ReadFile(cert)
	pending := filepath.Join(manager.Dir, "setup-pending.json")
	if err := os.WriteFile(pending, []byte("pending"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Renew(); !errors.Is(err, ErrTLSBusy) {
		t.Fatal("certificate changed during incomplete setup", err)
	}
	server := &Server{state: State{Configured: true}, tlsManager: manager}
	request := httptest.NewRequest("POST", "/install", strings.NewReader(`{"version":"v9.0.0"}`))
	request.Header.Set("X-Anker-Request", "1")
	response := httptest.NewRecorder()
	server.handler(response, request)
	if response.Code != 409 || !strings.Contains(response.Body.String(), "Einrichtung unvollständig") {
		t.Fatal(response.Code, response.Body.String())
	}
	after, _ := os.ReadFile(cert)
	if string(before) != string(after) {
		t.Fatal("certificate changed")
	}
	os.Remove(pending)
	if _, err := manager.Renew(); err != nil {
		t.Fatal("renewal remained blocked after recovery", err)
	}
}
