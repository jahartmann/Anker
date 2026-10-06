package updater

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"golang.org/x/sys/unix"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func tlsFixture(t *testing.T, days int, signed bool) (*TLSManager, string, string) {
	t.Helper()
	dir := t.TempDir()
	certDir := filepath.Join(dir, "tls")
	os.Mkdir(certDir, 0750)
	certPath, keyPath := filepath.Join(certDir, "server-123.crt"), filepath.Join(certDir, "server-123.key")
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	leaf := &x509.Certificate{SerialNumber: big.NewInt(100), Subject: pkix.Name{CommonName: "anker.internal"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Duration(days) * 24 * time.Hour), DNSNames: []string{"anker.internal"}, IPAddresses: []net.IP{net.ParseIP("192.0.2.15")}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
	parent := leaf
	if signed {
		parent = &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "fixture issuer"}, NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(2, 0, 0), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	}
	der, err := x509.CreateCertificate(rand.Reader, leaf, parent, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0640)
	os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: raw}), 0640)
	os.WriteFile(filepath.Join(dir, "service.env"), []byte("ANKER_TLS_CERT="+certPath+"\nANKER_TLS_KEY="+keyPath+"\nANKER_PUBLIC_HOST=anker.internal\n"), 0640)
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	publicHash := sha256.Sum256(parsed.RawSubjectPublicKeyInfo)
	policy, _ := json.Marshal(TLSPolicy{RenewBeforeDays: 30, ManagedCert: certPath, ManagedPublicKey: fmt.Sprintf("%X", publicHash)})
	os.WriteFile(filepath.Join(dir, "tls-renewal.json"), policy, 0600)
	return &TLSManager{Dir: dir, ownerUID: uint32(os.Geteuid())}, certPath, keyPath
}

func TestTLSRenewalPreservesKeyNamesAndPriorCertificate(t *testing.T) {
	m, cert, key := tlsFixture(t, 10, false)
	before, _ := os.ReadFile(cert)
	originalKey, _ := os.ReadFile(key)
	status, err := m.Renew()
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(cert)
	remainingKey, _ := os.ReadFile(key)
	pair, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(before) == string(after) || string(originalKey) != string(remainingKey) {
		t.Fatal("renewal failed or private key replaced")
	}
	for _, name := range []string{"anker.internal", "192.0.2.15"} {
		if err := leaf.VerifyHostname(name); err != nil {
			t.Fatal(err)
		}
	}
	if time.Until(leaf.NotAfter) < 360*24*time.Hour || status.LastRenewedAt == "" {
		t.Fatal("renewal is not persistent", status)
	}
	previous, err := os.ReadFile(cert + ".previous")
	if err != nil || string(previous) != string(before) {
		t.Fatal("previous certificate lost", err)
	}
	if status.Fingerprint == "" || !status.Managed || !status.Enabled {
		t.Fatal(status)
	}
}

func TestTLSAutomaticRenewalOnlyWhenEnabledAndDue(t *testing.T) {
	m, cert, _ := tlsFixture(t, 40, false)
	before, _ := os.ReadFile(cert)
	if _, err := m.SavePolicy(TLSPolicy{Automatic: true, RenewBeforeDays: 30}); err != nil {
		t.Fatal(err)
	}
	if err := m.Maintain(); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(cert)
	if string(after) != string(before) {
		t.Fatal("certificate renewed before configured window")
	}
	m.now = func() time.Time { return time.Now().Add(15 * 24 * time.Hour) }
	if _, err := m.SavePolicy(TLSPolicy{Automatic: false, RenewBeforeDays: 30}); err != nil {
		t.Fatal(err)
	}
	if err := m.Maintain(); err != nil {
		t.Fatal(err)
	}
	after, _ = os.ReadFile(cert)
	if string(after) != string(before) {
		t.Fatal("disabled automation renewed certificate")
	}
	if _, err := m.SavePolicy(TLSPolicy{Automatic: true, RenewBeforeDays: 30}); err != nil {
		t.Fatal(err)
	}
	if err := m.Maintain(); err != nil {
		t.Fatal(err)
	}
	after, _ = os.ReadFile(cert)
	if string(after) == string(before) {
		t.Fatal("due certificate not renewed")
	}
	if err := m.Maintain(); err != nil {
		t.Fatal(err)
	}
	repeated, _ := os.ReadFile(cert)
	if string(repeated) != string(after) {
		t.Fatal("renewed twice")
	}
}

func TestTLSManagerRejectsImportedFilesAndUnsafeConfiguration(t *testing.T) {
	for _, kind := range []string{"signed", "imported-selfsigned", "wrong-key-provenance", "symlink", "writable", "outside", "policy"} {
		t.Run(kind, func(t *testing.T) {
			m, cert, _ := tlsFixture(t, 10, kind == "signed")
			before, _ := os.ReadFile(cert)
			switch kind {
			case "imported-selfsigned":
				os.Remove(filepath.Join(m.Dir, "tls-renewal.json"))
			case "wrong-key-provenance":
				policy, err := m.policy()
				if err != nil {
					t.Fatal(err)
				}
				policy.ManagedPublicKey = strings.Repeat("0", 64)
				data, _ := json.Marshal(policy)
				os.WriteFile(filepath.Join(m.Dir, "tls-renewal.json"), data, 0600)
			case "symlink":
				os.Rename(cert, cert+".target")
				os.Symlink(cert+".target", cert)
			case "writable":
				os.Chmod(cert, 0660)
			case "outside":
				env, _ := os.ReadFile(filepath.Join(m.Dir, "service.env"))
				os.WriteFile(filepath.Join(m.Dir, "service.env"), []byte(strings.ReplaceAll(string(env), "server-123.crt", "other.crt")), 0640)
				os.WriteFile(filepath.Join(m.Dir, "tls", "other.crt"), before, 0640)
			case "policy":
				os.WriteFile(filepath.Join(m.Dir, "tls-renewal.json"), []byte(`{"automatic":true,"renew_before_days":0}`), 0600)
			}
			if _, err := m.Renew(); err == nil {
				t.Fatal("unsafe or imported certificate overwritten")
			}
			after, _ := os.ReadFile(cert)
			if string(before) != string(after) {
				t.Fatal("failed renewal changed original")
			}
		})
	}
}

func TestTLSRenewalDoesNotRaceSetup(t *testing.T) {
	m, cert, _ := tlsFixture(t, 10, false)
	before, _ := os.ReadFile(cert)
	fd, err := unix.Open(filepath.Join(m.Dir, "setup.lock"), unix.O_CREAT|unix.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Renew(); err == nil {
		t.Fatal("renewed while setup owns configuration")
	}
	after, _ := os.ReadFile(cert)
	if string(before) != string(after) {
		t.Fatal("locked certificate changed")
	}
}

func TestTLSAPIIsBoundedAndIndependentOfReleaseConfiguration(t *testing.T) {
	m, cert, key := tlsFixture(t, 10, false)
	s := &Server{tlsManager: m}
	for _, item := range []struct {
		method, path, body, header string
		want                       int
	}{
		{"GET", "/tls", "", "", 200},
		{"POST", "/tls", `{"automatic":true,"renew_before_days":30}`, "", 403},
		{"POST", "/tls", `{"automatic":true,"renew_before_days":30,"key":"/tmp/arbitrary"}`, "1", 400},
		{"POST", "/tls", `{"automatic":true,"renew_before_days":30}`, "1", 200},
		{"POST", "/tls/renew", "{}", "1", 200},
		{"POST", "/check", "{}", "1", 503},
	} {
		req := httptest.NewRequest(item.method, item.path, strings.NewReader(item.body))
		req.Header.Set("X-Anker-Request", item.header)
		res := httptest.NewRecorder()
		s.handler(res, req)
		if res.Code != item.want {
			t.Fatal(item.path, res.Code, res.Body.String())
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/tls/certificate", nil)
	res := httptest.NewRecorder()
	s.handler(res, req)
	public, _ := os.ReadFile(cert)
	private, _ := os.ReadFile(key)
	if res.Code != 200 || res.Body.String() != string(public) || strings.Contains(res.Body.String(), string(private)) {
		t.Fatal("certificate download incorrect")
	}
	var stored TLSPolicy
	b, _ := os.ReadFile(filepath.Join(m.Dir, "tls-renewal.json"))
	if err := json.Unmarshal(b, &stored); err != nil || !stored.Automatic {
		t.Fatal(err, stored)
	}
	s.busy = true
	req = httptest.NewRequest("POST", "/tls/renew", strings.NewReader("{}"))
	req.Header.Set("X-Anker-Request", "1")
	res = httptest.NewRecorder()
	s.handler(res, req)
	if res.Code != 409 {
		t.Fatal("certificate changed during update", res.Code)
	}
}

func TestTLSAutomaticFailureRemainsVisibleUntilSuccessfulRenewal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission failure requires an unprivileged test process")
	}
	m, cert, _ := tlsFixture(t, 10, false)
	if _, err := m.SavePolicy(TLSPolicy{Automatic: true, RenewBeforeDays: 30}); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Dir(cert)
	if err := os.Chmod(directory, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(directory, 0750)
	if err := m.Maintain(); err == nil {
		t.Fatal("failed publication hidden")
	}
	status, err := m.Status()
	if err != nil || status.LastError == "" {
		t.Fatal("automatic failure not visible", status, err)
	}
	if err := os.Chmod(directory, 0750); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Renew(); err != nil {
		t.Fatal(err)
	}
	status, err = m.Status()
	if err != nil || status.LastError != "" {
		t.Fatal("successful renewal retained error", status, err)
	}
}

func TestTLSStatusKeepsTheLastHoursValid(t *testing.T) {
	m, _, _ := tlsFixture(t, 1, false)
	m.now = func() time.Time { return time.Now().Add(12 * time.Hour) }
	status, err := m.Status()
	if err != nil || status.DaysRemaining != 1 {
		t.Fatal("certificate with twelve hours left appears expired", status, err)
	}
	m.now = func() time.Time { return time.Now().Add(25 * time.Hour) }
	status, err = m.Status()
	if err != nil || status.DaysRemaining > 0 {
		t.Fatal("expired certificate appears valid", status, err)
	}
}
