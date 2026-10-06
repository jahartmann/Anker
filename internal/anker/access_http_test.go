package anker

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionCookieAndProtectedAdministration(t *testing.T) {
	s := testService(t)
	a := NewAuth(s.Store)
	a.CreateUser("reader", "long-test-password", "reader", false)
	h := Handler(s, a, false)
	req := httptest.NewRequest("POST", "/api/login", bytes.NewBufferString(`{"name":"reader","password":"long-test-password"}`))
	req.Header.Set("X-Anker-Request", "1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatal(rr.Code, rr.Body.String())
	}
	cookies := rr.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge != 30*24*3600 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("cookie policy", cookies)
	}
	for _, path := range []string{"/api/users", "/api/settings", "/api/updates"} {
		req = httptest.NewRequest("GET", path, nil)
		rr = httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != 401 {
			t.Fatal("anonymous access", path, rr.Code)
		}
		req.AddCookie(cookies[0])
		rr = httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != 403 {
			t.Fatal("reader admin access", path, rr.Code)
		}
	}
}
