package updater

import (
	"encoding/json"
	"golang.org/x/sys/unix"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func configurationFixture(t *testing.T) *Server {
	t.Helper()
	return &Server{tlsManager: &TLSManager{Dir: t.TempDir(), ownerUID: uint32(os.Geteuid())}, installer: &Installer{StateDir: t.TempDir()}, state: State{Current: "v1.0.0", Status: "unconfigured"}}
}
func configurationRequest(s *Server, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("X-Anker-Request", "1")
	w := httptest.NewRecorder()
	s.handler(w, r)
	return w
}
func TestConfigureOfficialSourcePersistsAndReloads(t *testing.T) {
	s := configurationFixture(t)
	initial := configurationRequest(s, "GET", "/configuration", "")
	if initial.Code != 200 || !strings.Contains(initial.Body.String(), `"configured":false`) {
		t.Fatal(initial.Code, initial.Body.String())
	}
	s.state.Available = &Release{Version: "v9.0.0"}
	s.state.CheckedAt = "old"
	s.state.Target = "old"
	res := configurationRequest(s, "POST", "/configure", `{"repository":"jahartmann/Anker","confirmed":true}`)
	if res.Code != 200 {
		t.Fatal(res.Code, res.Body.String())
	}
	var c Config
	data, err := os.ReadFile(filepath.Join(s.tlsManager.Dir, "update.json"))
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(data, &c)
	if c.Repository != OfficialRepository || c.PublicKey != OfficialPublicKey || s.source.Config != c || !s.state.Configured || s.state.Available != nil || s.state.CheckedAt != "" || s.state.Target != "" {
		t.Fatal(c, s.state)
	}
	info, _ := os.Stat(filepath.Join(s.tlsManager.Dir, "update.json"))
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	if !strings.Contains(res.Body.String(), `"official":true`) || strings.Contains(res.Body.String(), "token_file") {
		t.Fatal(res.Body.String())
	}
}
func TestConfigureRejectsUnconfirmedInvalidAndBusyWithoutChangingTrust(t *testing.T) {
	for _, body := range []string{`{"repository":"jahartmann/Anker"}`, `{"repository":"../Anker","confirmed":true}`, `{"repository":"example/custom","confirmed":true}`, `{"repository":"example/custom","public_key":"abc","confirmed":true}`, `{"repository":"jahartmann/Anker","confirmed":true,"token_file":"/tmp/token"}`, `{"repository":"jahartmann/Anker","confirmed":true} {}`} {
		s := configurationFixture(t)
		res := configurationRequest(s, "POST", "/configure", body)
		if res.Code != 400 {
			t.Fatal(body, res.Code, res.Body.String())
		}
		if s.state.Configured {
			t.Fatal("trust changed")
		}
	}
	for _, marker := range []string{"busy", "setup-pending.json", "update-maintenance", "pending.json", "setup.lock", "local-install.lock"} {
		t.Run(marker, func(t *testing.T) {
			s := configurationFixture(t)
			if marker == "busy" {
				s.busy = true
			} else {
				dir := s.tlsManager.Dir
				if marker == "pending.json" || marker == "local-install.lock" {
					dir = s.installer.StateDir
				}
				path := filepath.Join(dir, marker)
				if strings.HasSuffix(marker, ".lock") {
					fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR, 0600)
					if err != nil {
						t.Fatal(err)
					}
					defer unix.Close(fd)
					if err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
						t.Fatal(err)
					}
				} else {
					os.WriteFile(path, []byte("pending"), 0600)
				}
			}
			res := configurationRequest(s, "POST", "/configure", `{"confirmed":true}`)
			if res.Code != 409 {
				t.Fatal(res.Code, res.Body.String())
			}
			if s.state.Configured {
				t.Fatal("trust changed")
			}
		})
	}
}
func TestConfigurePreservesSameRepositoryTokenAndClearsChangedRepository(t *testing.T) {
	s := configurationFixture(t)
	tokenPath := filepath.Join(s.tlsManager.Dir, "token")
	os.WriteFile(tokenPath, []byte("secret-credential"), 0600)
	c := Config{Repository: OfficialRepository, PublicKey: OfficialPublicKey, TokenFile: tokenPath}
	data, _ := json.Marshal(c)
	os.WriteFile(filepath.Join(s.tlsManager.Dir, "update.json"), data, 0600)
	s.source = NewGitHub(c, "secret-credential")
	s.state.Configured = true
	res := configurationRequest(s, "POST", "/configure", `{"confirmed":true}`)
	if res.Code != 200 || s.source.Token != "secret-credential" || s.source.Config.TokenFile != tokenPath {
		t.Fatal(res.Code, s.source)
	}
	if strings.Contains(res.Body.String(), "secret-credential") || strings.Contains(res.Body.String(), tokenPath) || !strings.Contains(res.Body.String(), `"has_token":true`) {
		t.Fatal(res.Body.String())
	}
	body := `{"repository":"example/custom","public_key":"` + OfficialPublicKey + `","confirmed":true}`
	res = configurationRequest(s, "POST", "/configure", body)
	if res.Code != 200 || s.source.Token != "" || s.source.Config.TokenFile != "" {
		t.Fatal(res.Code, s.source)
	}
}
func TestConfigureRejectsUnsafeExistingConfiguration(t *testing.T) {
	for _, unsafe := range []string{"symlink", "writable", "invalid"} {
		t.Run(unsafe, func(t *testing.T) {
			s := configurationFixture(t)
			path := filepath.Join(s.tlsManager.Dir, "update.json")
			content := []byte(`{"repository":"jahartmann/Anker","public_key":"` + OfficialPublicKey + `"}`)
			switch unsafe {
			case "symlink":
				target := filepath.Join(s.tlsManager.Dir, "target")
				os.WriteFile(target, content, 0600)
				os.Symlink(target, path)
			case "writable":
				os.WriteFile(path, content, 0666)
				os.Chmod(path, 0666)
			case "invalid":
				os.WriteFile(path, []byte("broken"), 0600)
			}
			res := configurationRequest(s, "POST", "/configure", `{"confirmed":true}`)
			if res.Code < 400 {
				t.Fatal(res.Code, res.Body.String())
			}
			data, _ := os.ReadFile(path)
			if unsafe == "invalid" {
				content = []byte("broken")
			}
			if string(data) != string(content) {
				t.Fatal("existing config overwritten")
			}
		})
	}
}
