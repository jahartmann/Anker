package main

import (
	"net"
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
