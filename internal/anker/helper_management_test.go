package anker

import (
	"anker/internal/hostconnect"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

type helperConnectionFixture struct {
	connectionFixture
	refresh func(hostconnect.EnrollRequest) (hostconnect.RefreshResult, error)
}

func (f *helperConnectionFixture) Refresh(_ context.Context, in hostconnect.EnrollRequest) (hostconnect.RefreshResult, error) {
	f.calls++
	return f.refresh(in)
}
func TestHelperRefreshPreservesHostConfigAndNeverPersistsPassword(t *testing.T) {
	s := testService(t)
	h, _ := s.Host("host1")
	h.Group = "CP"
	h.ExtraPaths = []string{"/etc/hostname"}
	h.Schedule = "04:00"
	h.DriftMinutes = 30
	if e := s.saveHost(h); e != nil {
		t.Fatal(e)
	}
	before, _ := s.Host(h.ID)
	f := &helperConnectionFixture{}
	s.hostConnector = f
	f.refresh = func(in hostconnect.EnrollRequest) (hostconnect.RefreshResult, error) {
		if in.Address != h.Address || in.Port != h.SSHPort || in.Password != "only-this-call" {
			t.Fatal("unexpected refresh request")
		}
		if _, e := s.QueueProbe(h.ID); e == nil {
			t.Fatal("probe raced privileged refresh")
		}
		if e := s.PrepareUpdate(); e == nil {
			t.Fatal("server update raced refresh")
		}
		return hostconnect.RefreshResult{Version: hostconnect.HelperVersion, SHA256: hostconnect.HelperHash()}, nil
	}
	request := map[string]any{"host_id": h.ID, "revision": hostRevision(h), "username": "root", "password": "only-this-call", "fingerprint": "SHA256:pinned", "confirmed": true}
	raw, _ := json.Marshal(request)
	r := httptest.NewRequest("POST", "/api/operations/helpers/refresh", strings.NewReader(string(raw)))
	w := httptest.NewRecorder()
	if e := s.refreshHostHelper(User{ID: "admin", Role: "admin"}, w, r); e != nil || w.Code != 200 {
		t.Fatal(e, w.Code, w.Body.String())
	}
	after, _ := s.Host(h.ID)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("existing host was re-enrolled")
	}
	for _, bucket := range []string{"hosts", "jobs", "host_operations", "audit"} {
		rows, _ := records[json.RawMessage](s.Store, bucket)
		for _, row := range rows {
			if strings.Contains(string(row), "only-this-call") {
				t.Fatal("transient password persisted", bucket)
			}
		}
	}
	if e := s.PrepareUpdate(); e != nil {
		t.Fatal("refresh fence leaked", e)
	}
}
func TestHelperRefreshRejectsRolesStaleSelectionAndHidesRemoteErrors(t *testing.T) {
	for _, mode := range []string{"reader", "stale", "custom-trust", "failure"} {
		t.Run(mode, func(t *testing.T) {
			s := testService(t)
			h, _ := s.Host("host1")
			revision := hostRevision(h)
			if mode == "stale" {
				revision = strings.Repeat("0", 64)
			}
			if mode == "custom-trust" {
				h.KnownHostsPath = "/custom/trust"
				s.saveHost(h)
				revision = hostRevision(h)
			}
			f := &helperConnectionFixture{refresh: func(hostconnect.EnrollRequest) (hostconnect.RefreshResult, error) {
				return hostconnect.RefreshResult{}, errors.New("only-this-call remote secret")
			}}
			s.hostConnector = f
			raw, _ := json.Marshal(map[string]any{"host_id": h.ID, "revision": revision, "username": "root", "password": "only-this-call", "fingerprint": "SHA256:pinned", "confirmed": true})
			r := httptest.NewRequest("POST", "/api/operations/helpers/refresh", strings.NewReader(string(raw)))
			u := User{ID: "user", Role: "admin"}
			if mode == "reader" {
				u.Role = "reader"
			}
			e := s.refreshHostHelper(u, httptest.NewRecorder(), r)
			if e == nil || strings.Contains(e.Error(), "only-this-call") {
				t.Fatal(e)
			}
			if mode != "failure" && f.calls != 0 {
				t.Fatal("privileged call before admission")
			}
		})
	}
}
