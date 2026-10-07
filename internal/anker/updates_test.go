package anker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUpdateDrainGuardsJobsAndQueue(t *testing.T) {
	s := testService(t)
	s.Store.Put("jobs", "busy", Job{ID: "busy", HostID: "host1", State: "running"})
	if err := s.PrepareUpdate(); err == nil {
		t.Fatal("running restore allowed update")
	}
	s.Store.Delete("jobs", "busy")
	if err := s.PrepareUpdate(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.QueueBackup("host1"); err == nil {
		t.Fatal("new job while drained")
	}
	s.ReleaseUpdate()
	if s.Updating() {
		t.Fatal("drain not released")
	}
	_ = context.Background()
}
func TestUpdateControlOnlyAvailableThroughLocalSocket(t *testing.T) {
	s := testService(t)
	a := NewAuth(s.Store)
	for _, local := range []bool{false, true} {
		req := httptest.NewRequest("GET", "/api/update-control", nil)
		rr := httptest.NewRecorder()
		Handler(s, a, local).ServeHTTP(rr, req)
		want := 404
		if local {
			want = 200
		}
		if rr.Code != want {
			t.Fatal(local, rr.Code)
		}
	}
}

func TestUpdateSourceAdministrationRoleOriginAndDemo(t *testing.T) {
	s := testService(t)
	s.Demo = true
	a := NewAuth(s.Store)
	for _, role := range []string{"reader", "restore", "admin"} {
		if err := a.CreateUser(role, "update-test-password", role, false); err != nil {
			t.Fatal(err)
		}
		token, _, err := a.Login(role, "update-test-password")
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range []struct{ method, path, body string }{{"GET", "/api/updates/configuration", ""}, {"POST", "/api/updates/configure", `{"confirmed":true}`}} {
			req := httptest.NewRequest(item.method, item.path, strings.NewReader(item.body))
			req.AddCookie(&http.Cookie{Name: "anker_session", Value: token})
			req.Header.Set("X-Anker-Request", "1")
			res := httptest.NewRecorder()
			Handler(s, a, false).ServeHTTP(res, req)
			want := 403
			if role == "admin" {
				want = 409
				if item.method == "GET" {
					want = 200
				}
			}
			if res.Code != want {
				t.Fatal(role, item.path, res.Code, res.Body.String())
			}
			if role == "admin" && item.method == "GET" && (!strings.Contains(res.Body.String(), `"demo":true`) || strings.Contains(res.Body.String(), "token_file")) {
				t.Fatal(res.Body.String())
			}
		}
	}
	for _, origin := range []string{"", "https://attacker.example"} {
		req := httptest.NewRequest("POST", "/api/updates/configure", strings.NewReader(`{"confirmed":true}`))
		if origin != "" {
			req.Header.Set("X-Anker-Request", "1")
			req.Header.Set("Origin", origin)
		}
		res := httptest.NewRecorder()
		Handler(s, a, true).ServeHTTP(res, req)
		if res.Code != 403 {
			t.Fatal(origin, res.Code)
		}
	}
	res := httptest.NewRecorder()
	Handler(s, a, false).ServeHTTP(res, httptest.NewRequest("GET", "/api/updates/configuration", nil))
	if res.Code != 401 {
		t.Fatal(res.Code)
	}
}

func TestUpdateSourceRejectsTrailingDataBeforeHelper(t *testing.T) {
	s := testService(t)
	a := NewAuth(s.Store)
	req := httptest.NewRequest("POST", "/api/updates/configure", strings.NewReader(`{"confirmed":true} {"repository":"other/source"}`))
	req.Header.Set("X-Anker-Request", "1")
	res := httptest.NewRecorder()
	Handler(s, a, true).ServeHTTP(res, req)
	if res.Code != 400 {
		t.Fatal(res.Code, res.Body.String())
	}
}
