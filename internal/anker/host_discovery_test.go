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
		for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
			w := httptest.NewRecorder()
			err := s.hostDiscovery(w, httptest.NewRequest(method, "/api/hosts/discovery", nil), User{Role: role})
			if err == nil {
				t.Fatal("allowed", role, method)
			}
		}
	}
	s.Demo = true
	if w := discoveryRequest(s, "PATCH", `{}`); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
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

func TestDiscoveryRestartResumeKeepsOriginalScopeAndUpdatePauses(t *testing.T) {
	s := testService(t)
	s.discovery.TCPProbe = func(ctx context.Context, _ string, _ int, _ time.Duration) (bool, bool) {
		<-ctx.Done()
		return false, true
	}
	w := discoveryRequest(s, "POST", `{"ranges":["10.20.0.0/24"],"pbs_port":9007,"ssh_port":2222}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if err := s.PrepareUpdate(); err != nil {
		t.Fatal(err)
	}
	if w := discoveryRequest(s, "PATCH", `{}`); w.Code != 503 {
		t.Fatal("scan allowed during update", w.Code)
	}
	var cp discovery.Checkpoint
	if err := s.Store.Get("discovery", "checkpoint", &cp); err != nil || !cp.State.Resumable || cp.State.Status == "running" {
		t.Fatalf("checkpoint %+v %v", cp, err)
	}
	s.ReleaseUpdate()
	if w := discoveryRequest(s, "PUT", `{"ranges":["10.99.0.1"],"pbs_port":8007}`); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	restarted, err := NewService(s.Root, s.Store, fixtureCollector{})
	if err != nil {
		t.Fatal(err)
	}
	if restarted.discovery.Snapshot().Status != "paused" {
		t.Fatal("restart did not pause")
	}
	restarted.discovery.Unpaced = true
	restarted.discovery.TCPProbe = func(_ context.Context, a string, _ int, _ time.Duration) (bool, bool) {
		if !strings.HasPrefix(a, "10.20.0.") {
			t.Errorf("scope changed: %s", a)
		}
		return false, false
	}
	if w := discoveryRequest(restarted, "PATCH", `{}`); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := restarted.discovery.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	v := restarted.discovery.Snapshot()
	if v.Status != "completed" || v.Checked != 254 || v.Config.PBSPort != 9007 || v.Config.SSHPort != 2222 {
		t.Fatal(v)
	}
}

type hintFixture struct{ fixtureCollector }

func (hintFixture) DiscoveryHints(context.Context, Host) (discoveryHints, error) {
	return discoveryHints{Version: 1, State: "ok", Hints: []discovery.Hint{{Address: "10.20.0.22", Source: "cluster"}, {Address: "10.99.0.22", Source: "pbs-config"}}}, nil
}
func TestDiscoveryKnownHostAndConfigurationHints(t *testing.T) {
	s := testService(t)
	s.Collector = hintFixture{}
	hints, warnings := s.discoverySeeds(context.Background(), discovery.Config{})
	if len(hints) != 3 || len(warnings) != 0 || hints[0].Source != "known" {
		t.Fatal(hints, warnings)
	}
	var cached discoveryHints
	if err := s.Store.Get("discovery-hints", "host1", &cached); err != nil || len(cached.Hints) != 2 {
		t.Fatal(cached, err)
	}
	disabled := false
	hints, warnings = s.discoverySeeds(context.Background(), discovery.Config{UseClusterHints: &disabled})
	if len(hints) != 1 || len(warnings) != 0 || hints[0].Address != "192.0.2.1" {
		t.Fatal(hints, warnings)
	}
}
