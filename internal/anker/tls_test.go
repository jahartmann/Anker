package anker

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTLSAdministrationRequiresAdminAndDemoCannotReachHelper(t *testing.T) {
	s := testService(t)
	s.Demo = true
	a := NewAuth(s.Store)
	for _, role := range []string{"reader", "restore", "admin"} {
		if err := a.CreateUser(role, "tls-test-password", role, false); err != nil {
			t.Fatal(err)
		}
		token, _, err := a.Login(role, "tls-test-password")
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range []struct{ method, path, body string }{
			{"GET", "/api/tls", ""}, {"POST", "/api/tls", `{"automatic":true,"renew_before_days":30}`}, {"POST", "/api/tls/renew", "{}"}, {"GET", "/api/tls/certificate", ""},
		} {
			req := httptest.NewRequest(item.method, item.path, strings.NewReader(item.body))
			req.AddCookie(&http.Cookie{Name: "anker_session", Value: token})
			req.Header.Set("X-Anker-Request", "1")
			res := httptest.NewRecorder()
			Handler(s, a, false).ServeHTTP(res, req)
			want := 403
			if role == "admin" {
				want = 409
				if item.path == "/api/tls" && item.method == "GET" {
					want = 200
				}
			}
			if res.Code != want {
				t.Fatal(role, item.path, res.Code, res.Body.String())
			}
		}
	}
	res := httptest.NewRecorder()
	Handler(s, a, false).ServeHTTP(res, httptest.NewRequest("GET", "/api/tls", nil))
	if res.Code != 401 {
		t.Fatal("anonymous certificate settings", res.Code)
	}
}
