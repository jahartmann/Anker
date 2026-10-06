package updater

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestSignedManifestRejectsTamperingAndWrongPlatform(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	m := Manifest{Version: "v0.2.0", Format: 1, Assets: []Artifact{{Name: "anker-linux-amd64", OS: "linux", Arch: "amd64", Size: 4, SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}}
	raw, _ := json.Marshal(m)
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, raw))
	key := base64.StdEncoding.EncodeToString(pub)
	if _, _, err := VerifyManifest(raw, []byte(sig), key, "v0.2.0", "linux", "amd64"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		raw                    []byte
		key, version, os, arch string
	}{{append(raw, ' '), key, "v0.2.0", "linux", "amd64"}, {raw, key, "v0.3.0", "linux", "amd64"}, {raw, key, "v0.2.0", "linux", "arm64"}, {raw, "bad", "v0.2.0", "linux", "amd64"}} {
		if _, _, err := VerifyManifest(tc.raw, []byte(sig), tc.key, tc.version, tc.os, tc.arch); err == nil {
			t.Fatal("untrusted manifest accepted", tc)
		}
	}
}
func TestVersionOrdering(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{{"v0.10.0", "0.9.0", true}, {"v1.0.0", "1.0.0", false}, {"v0.2.0", "0.2.0-dev", true}, {"v0.1.9", "0.2.0", false}, {"bad", "0.1.0", false}} {
		if Newer(tc.a, tc.b) != tc.want {
			t.Fatal(tc)
		}
	}
}
