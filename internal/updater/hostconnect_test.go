package updater

import (
	"anker/internal/hostconnect"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type rootConnectFixture struct {
	calls  int
	server *Server
}

func (f *rootConnectFixture) Inspect(_ context.Context, in hostconnect.InspectRequest) (hostconnect.Identity, error) {
	f.calls++
	return hostconnect.Identity{Address: in.Address, Port: in.Port}, nil
}
func (f *rootConnectFixture) Enroll(_ context.Context, in hostconnect.EnrollRequest) (hostconnect.Enrollment, error) {
	f.calls++
	if !f.server.snapshot().Busy {
		return hostconnect.Enrollment{}, context.Canceled
	}
	return hostconnect.Enrollment{Address: in.Address, Port: in.Port}, nil
}
func TestRootHostConnectionRequiresStrictRequestAndCoordination(t *testing.T) {
	for _, path := range []string{"inspect", "enroll"} {
		for _, marker := range []string{"busy", "setup-pending.json", "update-maintenance", "pending.json", "malformed", "trailing", "unknown", "origin"} {
			t.Run(path+marker, func(t *testing.T) {
				s := configurationFixture(t)
				f := &rootConnectFixture{server: s}
				s.hostManager = f
				body := `{"address":"pve.example","port":22}`
				if path == "enroll" {
					body = `{"address":"pve.example","port":22,"username":"root","password":"transient-only","fingerprint":"SHA256:test","confirmed":true}`
				}
				expected := 409
				switch marker {
				case "busy":
					s.busy = true
				case "malformed":
					body = `{`
					expected = 400
				case "trailing":
					body += ` {}`
					expected = 400
				case "unknown":
					body = strings.TrimSuffix(body, "}") + `,"transient-only":true}`
					expected = 400
				case "origin":
					expected = 403
				default:
					dir := s.tlsManager.Dir
					if marker == "pending.json" {
						dir = s.installer.StateDir
					}
					os.WriteFile(filepath.Join(dir, marker), []byte("pending"), 0600)
				}
				r := httptest.NewRequest("POST", "/hosts/"+path, strings.NewReader(body))
				if marker != "origin" {
					r.Header.Set("X-Anker-Request", "1")
				}
				w := httptest.NewRecorder()
				s.handler(w, r)
				if w.Code != expected || f.calls != 0 || strings.Contains(w.Body.String(), "transient-only") {
					t.Fatal(w.Code, w.Body.String(), f.calls)
				}
			})
		}
	}
}
func TestRootHostInspectionAndEnrollmentReleaseBusy(t *testing.T) {
	s := configurationFixture(t)
	f := &rootConnectFixture{server: s}
	s.hostManager = f
	for _, item := range []struct{ path, body string }{{"inspect", `{"address":"pve.example","port":22}`}, {"enroll", `{"address":"pve.example","port":22,"username":"root","password":"transient-only","fingerprint":"SHA256:test","confirmed":true}`}} {
		w := configurationRequest(s, "POST", "/hosts/"+item.path, item.body)
		if w.Code != 200 || s.snapshot().Busy {
			t.Fatal(w.Code, w.Body.String())
		}
		var out map[string]any
		if json.Unmarshal(w.Body.Bytes(), &out) != nil || out["address"] != "pve.example" {
			t.Fatal(out)
		}
	}
	if f.calls != 2 {
		t.Fatal(f.calls)
	}
}
