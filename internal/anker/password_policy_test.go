package anker

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEightCharacterPasswordsWorkForCreationAndReset(t *testing.T) {
	s := testService(t)
	a := NewAuth(s.Store)
	if err := a.CreateUser("eight", "12345678", "reader", false); err != nil {
		t.Fatal("eight characters rejected at creation", err)
	}
	token, _, err := a.Login("eight", "12345678")
	if err != nil {
		t.Fatal(err)
	}
	if err = a.SetPassword("eight", "87654321"); err != nil {
		t.Fatal("eight characters rejected at reset", err)
	}
	if _, err = a.Session(token, time.Now()); err == nil {
		t.Fatal("reset kept the old session")
	}
	if _, _, err = a.Login("eight", "87654321"); err != nil {
		t.Fatal(err)
	}
	if err = a.SetPassword("eight", "1234567"); err == nil {
		t.Fatal("seven characters accepted")
	}
	if _, _, err = a.Login("eight", "87654321"); err != nil {
		t.Fatal("rejected reset changed credentials", err)
	}
}

func TestConfiguredMinimumIsEnforcedWithoutLockingOutExistingAccounts(t *testing.T) {
	s := testService(t)
	a := NewAuth(s.Store)
	if err := a.CreateUser("existing", "abcdefghijkl", "reader", false); err != nil {
		t.Fatal(err)
	}
	settings, _ := s.Settings()
	body, _ := json.Marshal(settings)
	var values map[string]any
	json.Unmarshal(body, &values)
	values["password_min_length"] = 16
	if err := s.Store.Put("settings", "main", values); err != nil {
		t.Fatal(err)
	}
	if err := a.CreateUser("short", "abcdefghijkl", "reader", false); err == nil {
		t.Fatal("custom minimum ignored for creation")
	}
	if err := a.SetPassword("existing", "abcdefghijkl"); err == nil {
		t.Fatal("custom minimum ignored for reset")
	}
	if _, _, err := a.Login("existing", "abcdefghijkl"); err != nil {
		t.Fatal("policy change blocked existing login", err)
	}
	if err := a.SetPassword("existing", "abcdefghijklmnop"); err != nil {
		t.Fatal(err)
	}
}

func TestPasswordPolicyCanBeSavedThroughSettingsAPI(t *testing.T) {
	s := testService(t)
	settings, _ := s.Settings()
	body, _ := json.Marshal(settings)
	var values map[string]any
	json.Unmarshal(body, &values)
	values["password_min_length"] = 12
	request := func() *httptest.ResponseRecorder {
		body, _ := json.Marshal(values)
		r := httptest.NewRequest("PUT", "/api/settings", strings.NewReader(string(body)))
		r.Header.Set("X-Anker-Request", "1")
		w := httptest.NewRecorder()
		Handler(s, NewAuth(s.Store), true).ServeHTTP(w, r)
		return w
	}
	w := request()
	if w.Code != 200 {
		t.Fatal("password policy not configurable", w.Code, w.Body.String())
	}
	var got map[string]any
	json.Unmarshal(w.Body.Bytes(), &got)
	if got["password_min_length"] != float64(12) {
		t.Fatal("saved policy missing", got)
	}
	for _, invalid := range []int{7, 129} {
		values["password_min_length"] = invalid
		if request().Code != 400 {
			t.Fatal("invalid minimum accepted", invalid)
		}
	}
}

func TestUnreadablePasswordPolicyCannotChangeCredentials(t *testing.T) {
	s := testService(t)
	a := NewAuth(s.Store)
	if err := a.CreateUser("existing", "long-test-password", "reader", false); err != nil {
		t.Fatal(err)
	}
	s.Store.db.Exec(`UPDATE records SET value='invalid-json' WHERE bucket='settings' AND id='main'`)
	if err := a.SetPassword("existing", "replacement-password"); err == nil {
		t.Fatal("reset ignored corrupt policy")
	}
	if err := a.CreateUser("new", "long-test-password", "reader", false); err == nil {
		t.Fatal("creation ignored corrupt policy")
	}
}
