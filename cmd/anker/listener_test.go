package main

import (
	"crypto/tls"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestWebListenerRejectsBindAndTLSFailureBeforeReadiness(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	if listener, _, err := bindWeb(busy.Addr().String(), "", ""); err == nil {
		listener.Close()
		t.Fatal("busy web port accepted")
	}
	if listener, _, err := bindWeb("127.0.0.1:0", "missing.crt", "missing.key"); err == nil {
		listener.Close()
		t.Fatal("invalid TLS accepted")
	}
	listener, config, err := bindWeb("127.0.0.1:0", "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if config != nil {
		t.Fatal("plain local listener unexpectedly TLS")
	}
}

func TestWebListenerReloadsCertificateAndRetainsLastValidPair(t *testing.T) {
	cert, key, _, err := setupLocalTLS("localhost", "", "")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certPath, keyPath := filepath.Join(dir, "server.crt"), filepath.Join(dir, "server.key")
	os.WriteFile(certPath, cert, 0600)
	os.WriteFile(keyPath, key, 0600)
	listener, config, err := bindWeb("127.0.0.1:0", certPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	// Use an empty SNI too: IP-address clients must also see the new certificate.
	handshake := func() string {
		done := make(chan error, 1)
		go func() {
			connection, err := listener.Accept()
			if err == nil {
				server := tls.Server(connection, config)
				err = server.Handshake()
				server.Close()
			}
			done <- err
		}()
		client, err := tls.Dial("tcp", listener.Addr().String(), &tls.Config{InsecureSkipVerify: true})
		if err != nil {
			t.Fatal(err)
		}
		raw := string(client.ConnectionState().PeerCertificates[0].Raw)
		client.Close()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		return raw
	}
	first := handshake()
	// Replacement has a different address/key solely to create a distinct valid fixture.
	nextCert, nextKey, _, err := setupLocalTLS("new.localhost", "", "")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(keyPath, nextKey, 0600)
	os.WriteFile(certPath, nextCert, 0600)
	second := handshake()
	if first == second {
		t.Fatal("new TLS certificate requires service restart")
	}
	os.WriteFile(certPath, []byte("broken"), 0600)
	if third := handshake(); third != second {
		t.Fatal("invalid replacement discarded last valid TLS identity")
	}
}
