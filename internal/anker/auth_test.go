package anker

import (
	"testing"
	"time"
)

func TestLoginAndSessionExpiry(t *testing.T) {
	s := testService(t)
	a := NewAuth(s.Store)
	if err := a.CreateUser("admin", "long-test-password", "admin", true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.Login("admin", "wrong"); err == nil {
		t.Fatal("wrong password accepted")
	}
	token, u, err := a.Login("admin", "long-test-password")
	if err != nil || u.Role != "admin" {
		t.Fatal(err)
	}
	if _, err = a.Session(token, time.Now().Add(9*time.Hour)); err == nil {
		t.Fatal("expired session accepted")
	}
	a = NewAuth(s.Store)
	if _, _, err = a.Login("admin", "long-test-password"); err != nil {
		t.Fatal("credential not persisted", err)
	}
}

func TestSecretPermissionIsSeparateFromRole(t *testing.T) {
	for _, role := range []string{"admin", "restore", "reader"} {
		if userAllows(User{Role: role}, "secrets") {
			t.Fatal("secret permission bypass for", role)
		}
		if !userAllows(User{Role: role, Secrets: true}, "secrets") {
			t.Fatal("explicit secret permission ignored", role)
		}
	}
}

func TestPasswordResetRevokesSessions(t *testing.T) {
	s := testService(t)
	a := NewAuth(s.Store)
	a.CreateUser("reader", "long-test-password", "reader", false)
	token, _, _ := a.Login("reader", "long-test-password")
	if e := a.SetPassword("reader", "replacement-password"); e != nil {
		t.Fatal(e)
	}
	if _, e := a.Session(token, time.Now()); e == nil {
		t.Fatal("old session remains authorized")
	}
	if _, _, e := a.Login("reader", "long-test-password"); e == nil {
		t.Fatal("old password remains valid")
	}
	if _, _, e := a.Login("reader", "replacement-password"); e != nil {
		t.Fatal(e)
	}
}
