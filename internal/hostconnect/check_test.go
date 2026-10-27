package hostconnect

import (
	"context"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckAuthenticatesWithoutCommandsOrTrustWrites(t *testing.T) {
	f := fixture(t)
	m := managerFixture(t, f)
	os.WriteFile(filepath.Join(m.Dir, "known_hosts"), nil, 0640)
	before, err := os.ReadFile(filepath.Join(m.Dir, "known_hosts"))
	if err != nil {
		t.Fatal(err)
	}
	for _, password := range []string{"wrong", "top-secret"} {
		request := enrollmentRequest(f)
		request.Password = password
		result, err := m.Check(context.Background(), request)
		if err != nil || result.Authenticated != (password == "top-secret") {
			t.Fatal(result, err)
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.commands) != 0 || len(f.uploads) != 0 {
		t.Fatal("check changed remote host", f.commands)
	}
	after, _ := os.ReadFile(filepath.Join(m.Dir, "known_hosts"))
	if string(before) != string(after) {
		t.Fatal("trust changed")
	}
}
func TestCheckRejectsUntrustedIdentityBeforePassword(t *testing.T) {
	for _, kind := range []string{"unconfirmed", "mismatch", "changed"} {
		t.Run(kind, func(t *testing.T) {
			f := fixture(t)
			m := managerFixture(t, f)
			r := enrollmentRequest(f)
			switch kind {
			case "unconfirmed":
				r.Confirmed = false
			case "mismatch":
				r.Fingerprint = ssh.FingerprintSHA256(signer(t).PublicKey())
			case "changed":
				os.WriteFile(filepath.Join(m.Dir, "known_hosts"), []byte(knownhosts.Line([]string{f.listener.Addr().String()}, signer(t).PublicKey())+"\n"), 0640)
			}
			if _, err := m.Check(context.Background(), r); err == nil {
				t.Fatal("untrusted check")
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.passwords != 0 {
				t.Fatal("password sent", f.passwords)
			}
		})
	}
}
