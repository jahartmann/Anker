package anker

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPProtectsAnonymousAndReaders(t *testing.T) {
	s := testService(t)
	a := NewAuth(s.Store)
	a.CreateUser("reader", "long-test-password", "reader", false)
	token, _, _ := a.Login("reader", "long-test-password")
	handler := Handler(s, a, false)
	for _, tc := range []struct {
		method, path, body string
		cookie             bool
		want               int
	}{{"GET", "/api/status", "", false, 401}, {"POST", "/api/hosts", `{"name":"pve","address":"192.0.2.4"}`, true, 403}, {"GET", "/api/settings", "", true, 403}} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		r.Header.Set("X-Anker-Request", "1")
		if tc.cookie {
			r.AddCookie(&http.Cookie{Name: "anker_session", Value: token})
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s %s got %d want %d: %s", tc.method, tc.path, w.Code, tc.want, w.Body.String())
		}
	}
}
func TestHTTPRejectsForeignOrigin(t *testing.T) {
	s := testService(t)
	a := NewAuth(s.Store)
	a.CreateUser("admin", "long-test-password", "admin", true)
	token, _, _ := a.Login("admin", "long-test-password")
	r := httptest.NewRequest("POST", "/api/hosts", strings.NewReader(`{}`))
	r.Header.Set("Origin", "https://evil.example")
	r.Header.Set("X-Anker-Request", "1")
	r.AddCookie(&http.Cookie{Name: "anker_session", Value: token})
	w := httptest.NewRecorder()
	Handler(s, a, false).ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}

func TestPlanRedactsContentDetectedSecrets(t *testing.T) {
	p := Plan{Steps: []Step{{Path: "etc/application.conf", Diff: "password=do-not-leak", Secret: true}}}
	masked := redactPlan(p, false)
	if strings.Contains(masked.Steps[0].Diff, "do-not-leak") {
		t.Fatal("secret diff leaked")
	}
	if !strings.Contains(p.Steps[0].Diff, "do-not-leak") {
		t.Fatal("redaction modified original plan")
	}
}

type failingStream struct{ header http.Header }

func (w *failingStream) Header() http.Header       { return w.header }
func (w *failingStream) WriteHeader(int)           {}
func (w *failingStream) Write([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func TestFailedExportAbortsResponse(t *testing.T) {
	s := testService(t)
	b, _ := s.CreateBackup(context.Background(), "host1")
	r := httptest.NewRequest("GET", "/api/backups/"+b.ID+"/download", nil)
	defer func() {
		if recover() != http.ErrAbortHandler {
			t.Fatal("failed stream completed as valid response")
		}
	}()
	Handler(s, NewAuth(s.Store), true).ServeHTTP(&failingStream{header: http.Header{}}, r)
}
func TestReaderCanCompareButCannotRevealSecret(t *testing.T) {
	s := testService(t)
	b, _ := s.CreateBackup(context.Background(), "host1")
	a := NewAuth(s.Store)
	a.CreateUser("reader", "long-test-password", "reader", false)
	token, _, _ := a.Login("reader", "long-test-password")
	for _, tc := range []struct {
		method, path, body string
		want               int
	}{{"POST", "/api/backups/diff", `{"from":"` + b.ID + `","to":"` + b.ID + `","path":""}`, 200}, {"GET", "/api/backups/" + b.ID + "/file?path=etc/network/interfaces&reveal=1", "", 403}} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		r.Header.Set("X-Anker-Request", "1")
		r.AddCookie(&http.Cookie{Name: "anker_session", Value: token})
		w := httptest.NewRecorder()
		Handler(s, a, false).ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatal(tc.path, w.Code, w.Body.String())
		}
	}
}
