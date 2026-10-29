package hostconnect

import (
	hostassets "anker/host"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"golang.org/x/crypto/ssh/knownhosts"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRefreshRequiresExistingPinAndPreservesKeys(t *testing.T) {
	for _, test := range []struct {
		name                 string
		known, changed, fail bool
	}{{"known", true, false, false}, {"unknown", false, false, false}, {"changed", true, true, false}, {"failed", true, false, true}} {
		t.Run(test.name, func(t *testing.T) {
			f := fixture(t)
			m := managerFixture(t, f)
			r := enrollmentRequest(f)
			f.failInstall = test.fail
			if test.known {
				key := f.key.PublicKey()
				if test.changed {
					key = signer(t).PublicKey()
				}
				os.WriteFile(filepath.Join(m.Dir, "known_hosts"), []byte(knownhosts.Line([]string{f.listener.Addr().String()}, key)+"\n"), 0640)
			}
			before, _ := os.ReadFile(filepath.Join(m.Dir, "keys", "backup"))
			result, e := m.Refresh(context.Background(), r)
			after, _ := os.ReadFile(filepath.Join(m.Dir, "keys", "backup"))
			if !bytes.Equal(before, after) {
				t.Fatal("role key changed")
			}
			if e != nil && strings.Contains(e.Error(), r.Password) {
				t.Fatal("password leaked")
			}
			success := test.known && !test.changed && !test.fail
			if (e == nil) != success {
				t.Fatal(result, e)
			}
			if success && result.SHA256 != HelperHash() {
				t.Fatal(result)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			for _, cmd := range f.commands {
				if strings.Contains(cmd, r.Password) || strings.Contains(cmd, "authorized_keys") || strings.Contains(cmd, "apt-get") {
					t.Fatal("unsafe refresh command")
				}
			}
			if !test.known || test.changed {
				if len(f.commands) != 0 {
					t.Fatal("commands on untrusted identity")
				}
			}
		})
	}
}
func TestRefreshScriptAtomicAndProfilePreserved(t *testing.T) {
	python, e := exec.LookPath("python3")
	if e != nil {
		t.Skip("python3 required")
	}
	for _, unsafe := range []bool{false, true} {
		t.Run(fmt.Sprint(unsafe), func(t *testing.T) {
			root := t.TempDir()
			paths := []string{"usr", "usr/local", "usr/local/lib", "usr/local/lib/anker", "var", "var/lib"}
			for _, p := range paths {
				if e := os.MkdirAll(filepath.Join(root, p), 0755); e != nil {
					t.Fatal(e)
				}
			}
			target := filepath.Join(root, "usr/local/lib/anker/anker-host")
			old := []byte("#!/usr/bin/python3\nprint('old')\n")
			os.WriteFile(target, old, 0755)
			profile := filepath.Join(root, "profile.json")
			os.WriteFile(profile, []byte("custom-profile"), 0600)
			if unsafe {
				os.Chmod(filepath.Dir(target), 0777)
			}
			script := strings.ReplaceAll(refreshScript, "HELPER_BASE64", base64.StdEncoding.EncodeToString(hostassets.Helper))
			script = strings.ReplaceAll(script, "HELPER_SHA256", HelperHash())
			script = strings.ReplaceAll(script, "os.geteuid()==0", fmt.Sprintf("os.geteuid()==%d", os.Geteuid()))
			script = strings.ReplaceAll(script, "s.st_uid==0", fmt.Sprintf("s.st_uid==%d", os.Geteuid()))
			script = strings.ReplaceAll(script, "'/usr", "'TESTROOT/usr")
			script = strings.ReplaceAll(script, "'/var", "'TESTROOT/var")
			script = strings.ReplaceAll(script, "TESTROOT", root)
			output, e := exec.Command(python, "-c", script).CombinedOutput()
			data, _ := os.ReadFile(target)
			value, _ := os.ReadFile(profile)
			if string(value) != "custom-profile" {
				t.Fatal("profile changed")
			}
			if unsafe {
				if e == nil || !bytes.Equal(data, old) {
					t.Fatal("writable helper directory accepted", string(output))
				}
			} else if e != nil || !bytes.Equal(data, hostassets.Helper) || strings.TrimSpace(string(output)) != HelperHash() {
				t.Fatal("refresh failed", e, string(output))
			}
		})
	}
}

func TestHelperVersionMatchesEmbeddedProbe(t *testing.T) {
	if !bytes.Contains(hostassets.Helper, []byte("'helper_version':'"+HelperVersion+"'")) {
		t.Fatal("helper version disagrees with embedded probe")
	}
}
