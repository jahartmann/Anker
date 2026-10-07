package anker

import (
	"anker/internal/hostconnect"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type connectionFixture struct {
	calls  int
	enroll func(hostconnect.EnrollRequest) (hostconnect.Enrollment, error)
}

func (f *connectionFixture) Inspect(_ context.Context, in hostconnect.InspectRequest) (hostconnect.Identity, error) {
	f.calls++
	return hostconnect.Identity{Address: in.Address, Port: in.Port, Fingerprint: "SHA256:test", KeyType: "ssh-ed25519"}, nil
}
func (f *connectionFixture) Enroll(_ context.Context, in hostconnect.EnrollRequest) (hostconnect.Enrollment, error) {
	f.calls++
	return f.enroll(in)
}
func connectionRequest(s *Service, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/api/hosts/connection/"+path, strings.NewReader(body))
	r.Header.Set("X-Anker-Request", "1")
	w := httptest.NewRecorder()
	Handler(s, NewAuth(s.Store), true).ServeHTTP(w, r)
	return w
}

const connectBody = `{"host":{"address":"pve.example","enabled":true},"username":"root","password":"temporary-password","fingerprint":"SHA256:test","confirmed":true}`

func TestHostEnrollmentSavesOnlyVerifiedRestrictedAccess(t *testing.T) {
	s := testService(t)
	f := &connectionFixture{}
	s.hostConnector = f
	f.enroll = func(in hostconnect.EnrollRequest) (hostconnect.Enrollment, error) {
		hosts, _ := s.Hosts()
		if len(hosts) != 1 {
			t.Fatal("host saved before verification", hosts)
		}
		if err := s.PrepareUpdate(); err == nil {
			t.Fatal("update raced with enrollment")
		}
		return hostconnect.Enrollment{Address: in.Address, Port: 22, Fingerprint: in.Fingerprint, BackupUser: "anker", RestoreUser: "anker-restore", BackupKeyPath: "/etc/anker/keys/backup", RestoreKeyPath: "/etc/anker/keys/restore", KnownHostsPath: "/etc/anker/known_hosts", Inventory: json.RawMessage(`{"hostname":"pve-one","pve_version":"pve-manager/9.0"}`)}, nil
	}
	w := connectionRequest(s, "enroll", connectBody)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var h Host
	json.Unmarshal(w.Body.Bytes(), &h)
	if h.Name != "pve-one" || h.Inventory == nil || h.SSHUser != "anker" || h.RestoreSSHUser != "anker-restore" || h.LastProbe == "" || h.SSHPort != 22 {
		t.Fatal(h)
	}
	stored, _ := s.Hosts()
	if len(stored) != 2 {
		t.Fatal(stored)
	}
	for _, kind := range []string{"hosts", "audit", "jobs"} {
		data, _ := records[json.RawMessage](s.Store, kind)
		for _, row := range data {
			if strings.Contains(string(row), "temporary-password") {
				t.Fatal("password persisted", kind)
			}
		}
	}
	if err := s.PrepareUpdate(); err != nil {
		t.Fatal("enrollment remained locked", err)
	}
}
func TestHostEnrollmentRejectsFailureAndInvalidInputsBeforePersisting(t *testing.T) {
	for _, body := range []string{strings.Replace(connectBody, `"confirmed":true`, `"confirmed":false`, 1), strings.Replace(connectBody, `"pve.example"`, `"-bad"`, 1), strings.Replace(connectBody, `"enabled":true`, `"enabled":true,"schedule":"25:00"`, 1), connectBody + ` {}`, strings.Replace(connectBody, `"username":"root"`, `"username":"root","private_key":"/tmp/key"`, 1)} {
		s := testService(t)
		f := &connectionFixture{}
		s.hostConnector = f
		w := connectionRequest(s, "enroll", body)
		if w.Code != 400 || f.calls != 0 {
			t.Fatal(w.Code, w.Body.String(), f.calls)
		}
	}
	s := testService(t)
	f := &connectionFixture{enroll: func(hostconnect.EnrollRequest) (hostconnect.Enrollment, error) {
		return hostconnect.Enrollment{}, errors.New("temporary-password leaked")
	}}
	s.hostConnector = f
	w := connectionRequest(s, "enroll", connectBody)
	if w.Code < 400 || strings.Contains(w.Body.String(), "temporary-password") {
		t.Fatal(w.Code, w.Body.String())
	}
	hosts, _ := s.Hosts()
	if len(hosts) != 1 {
		t.Fatal(hosts)
	}
	f.enroll = func(in hostconnect.EnrollRequest) (hostconnect.Enrollment, error) {
		return hostconnect.Enrollment{Inventory: json.RawMessage(`{"hostname":"pve","pve_version":""}`)}, nil
	}
	w = connectionRequest(s, "enroll", connectBody)
	if w.Code < 400 {
		t.Fatal("incomplete probe accepted")
	}
}
func TestHostEnrollmentInspectionAndDemoBoundaries(t *testing.T) {
	s := testService(t)
	f := &connectionFixture{}
	s.hostConnector = f
	w := connectionRequest(s, "inspect", `{"address":"pve.example","port":22}`)
	if w.Code != 200 || f.calls != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = connectionRequest(s, "inspect", `{"address":"pve.example","password":"temporary-password"}`)
	if w.Code != 400 || f.calls != 1 || strings.Contains(w.Body.String(), "temporary-password") {
		t.Fatal(w.Code, w.Body.String())
	}
	s.Demo = true
	w = connectionRequest(s, "enroll", connectBody)
	if w.Code != 409 || f.calls != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestHostConnectionAdministrationAndOrigin(t *testing.T) {
	s := testService(t)
	f := &connectionFixture{}
	s.hostConnector = f
	a := NewAuth(s.Store)
	for _, role := range []string{"reader", "restore", "admin"} {
		if err := a.CreateUser(role, "connection-test-password", role, false); err != nil {
			t.Fatal(err)
		}
		token, _, err := a.Login(role, "connection-test-password")
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest("POST", "/api/hosts/connection/inspect", strings.NewReader(`{"address":"pve.example","port":22}`))
		r.Header.Set("X-Anker-Request", "1")
		r.AddCookie(&http.Cookie{Name: "anker_session", Value: token})
		w := httptest.NewRecorder()
		Handler(s, a, false).ServeHTTP(w, r)
		want := 403
		if role == "admin" {
			want = 200
		}
		if w.Code != want {
			t.Fatal(role, w.Code, w.Body.String())
		}
	}
	for _, origin := range []string{"", "https://foreign.example"} {
		r := httptest.NewRequest("POST", "/api/hosts/connection/enroll", strings.NewReader(connectBody))
		if origin != "" {
			r.Header.Set("Origin", origin)
			r.Header.Set("X-Anker-Request", "1")
		}
		w := httptest.NewRecorder()
		Handler(s, a, true).ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal(w.Code)
		}
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/hosts/connection/inspect", strings.NewReader(`{"address":"pve.example"}`))
	r.Header.Set("X-Anker-Request", "1")
	Handler(s, a, false).ServeHTTP(w, r)
	if w.Code != 401 || f.calls != 1 {
		t.Fatal(w.Code, f.calls)
	}
}

func TestHostEnrollmentReconnectCoordinationAndDuplicateGuard(t *testing.T) {
	s := testService(t)
	h, _ := s.Host("host1")
	h.Schedule = "03:00"
	h.Group = "original"
	if err := s.SaveHost(h); err != nil {
		t.Fatal(err)
	}
	f := &connectionFixture{}
	s.hostConnector = f
	f.enroll = func(in hostconnect.EnrollRequest) (hostconnect.Enrollment, error) {
		if err := s.SaveHost(h); err == nil {
			t.Fatal("host edit raced")
		}
		if err := s.DeleteHost(h.ID); err == nil {
			t.Fatal("host deletion raced")
		}
		if _, err := s.QueueBackup(h.ID); err == nil {
			t.Fatal("backup raced")
		}
		if err := s.PrepareUpdate(); err == nil {
			t.Fatal("update raced")
		}
		return hostconnect.Enrollment{Address: in.Address, Port: in.Port, Fingerprint: in.Fingerprint, BackupUser: "anker", RestoreUser: "anker-restore", BackupKeyPath: "/etc/anker/keys/backup", RestoreKeyPath: "/etc/anker/keys/restore", KnownHostsPath: "/etc/anker/known_hosts", Inventory: json.RawMessage(`{"hostname":"remote-pve","pve_version":"9.0"}`)}, nil
	}
	body, _ := json.Marshal(map[string]any{"host": h, "username": "root", "password": "temporary-password", "fingerprint": "SHA256:test", "confirmed": true})
	s.Store.Put("jobs", "running", Job{ID: "running", HostID: h.ID, State: "running"})
	w := connectionRequest(s, "enroll", string(body))
	if w.Code != 409 || f.calls != 0 {
		t.Fatal(w.Code, f.calls)
	}
	s.Store.Delete("jobs", "running")
	w = connectionRequest(s, "enroll", string(body))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	updated, _ := s.Host(h.ID)
	hosts, _ := s.Hosts()
	if len(hosts) != 1 || updated.Schedule != "03:00" || updated.Group != "original" || updated.Name != h.Name || updated.Inventory.Hostname != "remote-pve" {
		t.Fatal(updated)
	}
	h.ID = ""
	body, _ = json.Marshal(map[string]any{"host": h, "username": "root", "password": "temporary-password", "fingerprint": "SHA256:test", "confirmed": true})
	w = connectionRequest(s, "enroll", string(body))
	if w.Code != 409 || f.calls != 1 {
		t.Fatal(w.Code, w.Body.String(), f.calls)
	}
	if err := s.SaveHost(updated); err != nil {
		t.Fatal("enrollment did not unlock host editing", err)
	}
}
