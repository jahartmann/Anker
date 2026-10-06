package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func TestSigningKeyValidatesPublicHalf(t *testing.T) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANKER_SIGNING_KEY", base64.StdEncoding.EncodeToString(private))
	key, err := signingKey()
	if err != nil || string(key) != string(private) {
		t.Fatal(err)
	}
	private[63] ^= 1
	t.Setenv("ANKER_SIGNING_KEY", base64.StdEncoding.EncodeToString(private))
	if _, err = signingKey(); err == nil {
		t.Fatal("malformed private key accepted")
	}
}
