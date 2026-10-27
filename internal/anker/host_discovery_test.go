package anker

import (
	"anker/internal/discovery"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func discoveryRequest(s *Service, method, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "/api/hosts/discovery", strings.NewReader(body))
	r.Header.Set("X-Anker-Request", "1")
	w := httptest.NewRecorder()
	Handler(s, NewAuth(s.Store), true).ServeHTTP(w, r)
	return w
}
func TestDiscoveryPersistenceProgressAndNoEnrollment(t *testing.T) {
	s := testService(t)
	s.discovery.Probe = func(ctx context.Context, address string, c discovery.Config) *discovery.Result {
		return &discovery.Result{Address: address}
	}
	body := `{"ranges":["10.20.0.1 255.255.255.255","10.21.0.1/32"],"web_port":8006,"ssh_port":2222}`
	w := discoveryRequest(s, "PUT", body)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var saved discovery.Config
	if err := s.Store.Get("discovery", "networks", &saved); err != nil || saved.Ranges[0] != "10.20.0.1/32" {
		t.Fatal(saved, err)
	}
	w = discoveryRequest(s, "GET", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "10.21.0.1/32") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = discoveryRequest(s, "POST", body)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	deadline := time.Now().Add(time.Second)
	for s.discovery.Snapshot().Status == "running" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	w = discoveryRequest(s, "GET", "")
	var data struct {
		Scan discovery.State `json:"scan"`
	}
	json.Unmarshal(w.Body.Bytes(), &data)
	if data.Scan.Checked != 2 || len(data.Scan.Results) != 2 || data.Scan.Status != "completed" {
		t.Fatal(w.Body.String())
	}
	hosts, _ := s.Hosts()
	if len(hosts) != 1 {
		t.Fatal("discovery enrolled hosts", hosts)
	}
	for _, bad := range []string{`{"ranges":["0.0.0.0/0"]}`, body + ` {}`, `{"ranges":["10.0.0.1"],"password":"secret"}`} {
		if w := discoveryRequest(s, "POST", bad); w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
func TestDiscoveryAdminAndDemoBoundaries(t *testing.T) {
	s := testService(t)
	for _, role := range []string{"reader", "restore"} {
		for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
			w := httptest.NewRecorder()
			err := s.hostDiscovery(w, httptest.NewRequest(method, "/api/hosts/discovery", nil), User{Role: role})
			if err == nil {
				t.Fatal("allowed", role, method)
			}
		}
	}
	s.Demo = true
	if w := discoveryRequest(s, "POST", `{"ranges":["10.0.0.1"]}`); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	r := httptest.NewRequest("POST", "/api/hosts/discovery", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	Handler(s, NewAuth(s.Store), true).ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("origin", w.Code)
	}
}
