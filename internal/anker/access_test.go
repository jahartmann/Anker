package anker

import (
	"strings"
	"testing"
	"time"
)

func TestPersistentSessionsAndAccountLifecycle(t *testing.T) {
	s := testService(t)
	a := NewAuth(s.Store)
	if err := a.CreateUser("owner", "long-test-password", "admin", true); err != nil {
		t.Fatal(err)
	}
	if err := a.CreateUser("reader", "long-test-password", "reader", false); err != nil {
		t.Fatal(err)
	}
	token, _, err := a.Login("reader", "long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	restarted := NewAuth(s.Store)
	if _, err = restarted.Session(token, time.Now().Add(29*24*time.Hour)); err != nil {
		t.Fatal("restart or long session lost", err)
	}
	var stored string
	if err = s.Store.db.QueryRow("SELECT CAST(value AS TEXT) FROM records WHERE bucket='sessions'").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, token) {
		t.Fatal("raw session token persisted")
	}
	if err = restarted.UpdateUser("owner", "reader", "restore", true, false); err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.Session(token, time.Now()); err == nil {
		t.Fatal("permission change did not revoke session")
	}
	token, _, err = restarted.Login("reader", "long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if err = restarted.UpdateUser("owner", "reader", "restore", true, true); err != nil {
		t.Fatal(err)
	}
	if _, err = a.Session(token, time.Now()); err == nil {
		t.Fatal("disabled account retained access")
	}
	if _, _, err = a.Login("reader", "long-test-password"); err == nil {
		t.Fatal("disabled account logged in")
	}
	if err = a.DeleteUser("owner", "reader"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = a.Login("reader", "long-test-password"); err == nil {
		t.Fatal("deleted account logged in")
	}
}
func TestLastAdminAndSelfLockoutGuards(t *testing.T) {
	s := testService(t)
	a := NewAuth(s.Store)
	a.CreateUser("owner", "long-test-password", "admin", true)
	if err := a.UpdateUser("local", "owner", "reader", false, false); err == nil {
		t.Fatal("last admin demoted")
	}
	if err := a.DeleteUser("local", "owner"); err == nil {
		t.Fatal("last admin deleted")
	}
	a.CreateUser("second", "long-test-password", "admin", true)
	if err := a.UpdateUser("owner", "owner", "admin", true, true); err == nil {
		t.Fatal("self disabled")
	}
	if err := a.DeleteUser("owner", "owner"); err == nil {
		t.Fatal("self deleted")
	}
	if err := a.UpdateUser("second", "owner", "reader", false, false); err != nil {
		t.Fatal(err)
	}
	if err := a.UpdateUser("local", "second", "reader", false, false); err == nil {
		t.Fatal("final remaining admin demoted")
	}
}
func TestLogoutAndExplicitRevocationPersist(t *testing.T) {
	s := testService(t)
	a := NewAuth(s.Store)
	a.CreateUser("reader", "long-test-password", "reader", false)
	token, _, _ := a.Login("reader", "long-test-password")
	if err := a.Logout(token); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAuth(s.Store).Session(token, time.Now()); err == nil {
		t.Fatal("logout lost after restart")
	}
	token, _, _ = a.Login("reader", "long-test-password")
	if err := a.RevokeSessions("reader"); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAuth(s.Store).Session(token, time.Now()); err == nil {
		t.Fatal("revocation lost after restart")
	}
}

func TestSessionSurvivesCatalogCloseAndReopen(t *testing.T) {
	s := testService(t)
	a := NewAuth(s.Store)
	if err := a.CreateUser("reader", "long-test-password", "reader", false); err != nil {
		t.Fatal(err)
	}
	token, _, err := a.Login("reader", "long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(s.Root + "/catalog.db")
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	b := NewAuth(reopened)
	if _, err = b.Session(token, time.Now()); err != nil {
		t.Fatal("session lost across storage restart", err)
	}
	if err = b.RevokeSessions("reader"); err != nil {
		t.Fatal(err)
	}
	if err = reopened.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := OpenStore(s.Root + "/catalog.db")
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if _, err = NewAuth(again).Session(token, time.Now()); err == nil {
		t.Fatal("revocation lost across storage restart")
	}
}
func TestConcurrentAdminChangesRetainAnActiveAdmin(t *testing.T) {
	s := testService(t)
	a := NewAuth(s.Store)
	for _, id := range []string{"one", "two"} {
		if err := a.CreateUser(id, "long-test-password", "admin", true); err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, id := range []string{"one", "two"} {
		go func(id string) { <-start; results <- a.UpdateUser("local", id, "reader", false, false) }(id)
	}
	close(start)
	failures := 0
	for range 2 {
		if <-results != nil {
			failures++
		}
	}
	if failures != 1 {
		t.Fatal("concurrent changes allowed last admin loss", failures)
	}
	users, err := records[User](s.Store, "users")
	if err != nil {
		t.Fatal(err)
	}
	admins := 0
	for _, u := range users {
		if u.Role == "admin" && !u.Disabled {
			admins++
		}
	}
	if admins != 1 {
		t.Fatal("active admins", admins)
	}
}
