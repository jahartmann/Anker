package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
)

func TestGitHubPrivateAssetsAndBoundedDownloads(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	content := []byte("binary")
	sum := sha256.Sum256(content)
	a := Artifact{Name: "anker-" + runtime.GOOS + "-" + runtime.GOARCH, OS: runtime.GOOS, Arch: runtime.GOARCH, Size: int64(len(content)), SHA256: hex.EncodeToString(sum[:])}
	raw, _ := json.Marshal(Manifest{Version: "v0.2.0", Format: 1, Assets: []Artifact{a}})
	sig := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, raw)))
	oversize := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Error("private credential missing")
		}
		switch r.URL.Path {
		case "/repos/example/anker/releases/latest":
			json.NewEncoder(w).Encode(map[string]any{"tag_name": "v0.2.0", "html_url": "https://github.com/example/anker/releases/tag/v0.2.0", "assets": []any{map[string]any{"name": "release.json", "id": 1}, map[string]any{"name": "release.json.sig", "id": 2}, map[string]any{"name": a.Name, "id": 3}}})
		case "/repos/example/anker/releases/assets/1":
			w.Write(raw)
		case "/repos/example/anker/releases/assets/2":
			w.Write(sig)
		case "/repos/example/anker/releases/assets/3":
			if oversize {
				w.Write(append(content, 'x'))
			} else {
				w.Write(content)
			}
		default:
			t.Error("unexpected endpoint", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	g := NewGitHub(Config{Repository: "example/anker", PublicKey: base64.StdEncoding.EncodeToString(pub)}, "fixture-token")
	g.base = server.URL
	g.HTTP = server.Client()
	release, err := g.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b, err := g.Download(context.Background(), release)
	if err != nil || string(b) != "binary" {
		t.Fatal(err)
	}
	oversize = true
	if _, err = g.Download(context.Background(), release); err == nil {
		t.Fatal("oversize response accepted")
	}
}
func TestRedirectNeverForwardsTokenToAssetStorage(t *testing.T) {
	g := NewGitHub(Config{}, "fixture-token")
	req, _ := http.NewRequest("GET", "https://release-assets.githubusercontent.com/file", nil)
	req.Header.Set("Authorization", "Bearer fixture-token")
	if err := g.HTTP.CheckRedirect(req, nil); err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("Authorization") != "" {
		t.Fatal("credential leaked to storage")
	}
	req, _ = http.NewRequest("GET", "http://storage.example/file", nil)
	if err := g.HTTP.CheckRedirect(req, nil); err == nil {
		t.Fatal("insecure redirect accepted")
	}
}
