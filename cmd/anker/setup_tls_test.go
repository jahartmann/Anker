package main

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSetupCreatesUsableTLSForDNSAndIP(t *testing.T) {
	for _, host := range []string{"anker.internal", "10.2.3.4", "fd00::1"} {
		cert, key, reused, err := setupLocalTLS(host, "", "")
		if err != nil || reused {
			t.Fatal(host, err)
		}
		pair, err := tls.X509KeyPair(cert, key)
		if err != nil {
			t.Fatal(err)
		}
		leaf, err := x509.ParseCertificate(pair.Certificate[0])
		if err != nil {
			t.Fatal(err)
		}
		if err := leaf.VerifyHostname(host); err != nil {
			t.Fatal(err)
		}
		roots := x509.NewCertPool()
		roots.AddCert(leaf)
		if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: host}); err != nil {
			t.Fatal(err)
		}
		if time.Until(leaf.NotAfter) < 360*24*time.Hour {
			t.Fatal("short-lived generated certificate")
		}
	}
}

func TestRepeatedSetupPreservesTLSIdentity(t *testing.T) {
	cert, key, _, err := setupLocalTLS("anker.internal", "", "")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	certPath, keyPath := filepath.Join(root, "server.crt"), filepath.Join(root, "server.key")
	if err := os.WriteFile(certPath, cert, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"anker.internal", "new-anker.internal"} {
		nextCert, nextKey, reused, err := setupLocalTLS(host, certPath, keyPath)
		if err != nil {
			t.Fatal(err)
		}
		if host == "anker.internal" {
			if !reused || string(nextCert) != string(cert) || string(nextKey) != string(key) {
				t.Fatal("existing TLS identity replaced")
			}
		} else {
			pair, err := tls.X509KeyPair(nextCert, nextKey)
			if err != nil || reused || pair.Leaf.VerifyHostname(host) != nil {
				t.Fatal("new address not covered", err)
			}
		}
	}
	if err := os.Remove(keyPath); err != nil {
		t.Fatal(err)
	}
	nextCert, nextKey, reused, err := setupLocalTLS("anker.internal", certPath, keyPath)
	if err != nil || reused {
		t.Fatal("explicit automatic setup cannot repair missing key", err)
	}
	if _, err := tls.X509KeyPair(nextCert, nextKey); err != nil {
		t.Fatal(err)
	}
}

func TestSetupTLSRejectsInvalidNames(t *testing.T) {
	for _, host := range []string{"", "https://anker.internal", "0.0.0.0", "::", "anker/name", "*.internal", "host\nOTHER=value", "anker:8087"} {
		if _, _, _, err := setupLocalTLS(host, "", ""); err == nil {
			t.Fatal("invalid name accepted", host)
		}
	}
}
