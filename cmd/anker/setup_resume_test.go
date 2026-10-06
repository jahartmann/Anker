package main

import (
	"anker/internal/updater"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupReusePreservesCustomPortAndTLSIdentity(t *testing.T) {
	cert, key, _, err := setupLocalTLS("anker.internal", "", "")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	cp, kp := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	os.WriteFile(cp, cert, 0640)
	os.WriteFile(kp, key, 0600)
	leaf, _ := setupTLSLeaf(cert, key)
	hash := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	policy := updater.TLSPolicy{Automatic: false, ManagedCert: cp, ManagedPublicKey: fmt.Sprintf("%X", hash)}
	prior := "ANKER_LISTEN=0.0.0.0:9443\nANKER_PUBLIC_HOST=anker.internal\nANKER_TLS_CERT=" + cp + "\nANKER_TLS_KEY=" + kp + "\n"
	c, err := setupExistingWeb(prior, policy)
	if err != nil || c.URL() != "https://anker.internal:9443" || c.Cert != cp || c.Key != kp || !c.Managed {
		t.Fatal("existing configuration changed", c, err)
	}
	if string(c.CertBytes) != string(cert) || string(c.KeyBytes) != string(key) {
		t.Fatal("identity changed")
	}
	// A partial or invalid pair must go through configuration again.
	os.Remove(kp)
	if _, err = setupExistingWeb(prior, policy); err == nil {
		t.Fatal("missing key accepted")
	}
}

func TestSetupReuseLoopbackAndMergeKeepLocalCustomizations(t *testing.T) {
	c, err := setupExistingWeb("ANKER_LISTEN=[::1]:9090\n", updater.TLSPolicy{})
	if err != nil || c.URL() != "http://[::1]:9090" || setupTunnelCommand(c) != "ssh -N -L 9090:[::1]:9090 BENUTZER@ANKER-SERVER" {
		t.Fatal(c, err)
	}
	merged := setupMergeWebEnv("# Local tuning\nANKER_LISTEN=127.0.0.1:8087\nANKER_TLS_KEY=old\nANKER_CUSTOM=value\n", "ANKER_LISTEN=127.0.0.1:9090\n")
	if !strings.Contains(merged, "ANKER_CUSTOM=value\n") || strings.Contains(merged, "old") || strings.Count(merged, "ANKER_LISTEN=") != 1 {
		t.Fatal(merged)
	}
}
