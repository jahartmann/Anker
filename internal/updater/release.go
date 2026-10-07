// Package updater implements the restricted GitHub release installation boundary.
package updater

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const MaxBinary = 256 << 20
const Socket = "/run/anker-updater/socket"
const ConfigPath = "/etc/anker/update.json"
const StateDir = "/var/lib/anker-updater"
const BinaryPath = "/usr/local/bin/anker"
const HelperPath = "/usr/local/libexec/anker-updater"
const DataDir = "/srv/anker"
const LocalInstallLock = "/run/anker-local-install.lock"

type Config struct {
	Repository string `json:"repository"`
	PublicKey  string `json:"public_key"`
	TokenFile  string `json:"token_file,omitempty"`
}
type Artifact struct {
	Kind   string `json:"kind,omitempty"`
	Name   string `json:"name"`
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type Manifest struct {
	Version string     `json:"version"`
	Format  int        `json:"format"`
	Assets  []Artifact `json:"assets"`
}
type Release struct {
	Version  string   `json:"version"`
	URL      string   `json:"url"`
	Artifact Artifact `json:"artifact"`
	AssetID  int64    `json:"-"`
}
type State struct {
	Busy       bool     `json:"busy"`
	Configured bool     `json:"configured"`
	Repository string   `json:"repository"`
	Current    string   `json:"current"`
	Available  *Release `json:"available,omitempty"`
	Status     string   `json:"status"`
	Message    string   `json:"message,omitempty"`
	CheckedAt  string   `json:"checked_at,omitempty"`
	UpdatedAt  string   `json:"updated_at,omitempty"`
	Target     string   `json:"target,omitempty"`
}

var versionPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func parseVersion(v string) ([3]uint64, bool) {
	var n [3]uint64
	m := versionPattern.FindStringSubmatch(v)
	if m == nil {
		return n, false
	}
	for i := range n {
		var err error
		n[i], err = strconv.ParseUint(m[i+1], 10, 32)
		if err != nil {
			return n, false
		}
	}
	return n, true
}
func Newer(candidate, current string) bool {
	a, ok := parseVersion(candidate)
	if !ok {
		return false
	}
	dev := strings.HasSuffix(current, "-dev")
	b, ok := parseVersion(strings.TrimSuffix(current, "-dev"))
	if !ok {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return dev
}
func VerifyManifest(raw, sig []byte, key, tag, osName, arch string) (Manifest, Artifact, error) {
	var m Manifest
	var selected Artifact
	pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(key))
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return m, selected, errors.New("Update-Signierschlüssel ungültig")
	}
	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil || !ed25519.Verify(pub, raw, signature) {
		return m, selected, errors.New("Release-Signatur ungültig")
	}
	if err = json.Unmarshal(raw, &m); err != nil {
		return m, selected, err
	}
	if _, ok := parseVersion(m.Version); !ok || m.Version != tag || m.Format != 1 {
		return m, selected, errors.New("Release-Version oder Datenformat nicht unterstützt")
	}
	names := map[string]bool{}
	for _, a := range m.Assets {
		kind := a.Kind
		if kind == "" {
			kind = "binary"
		}
		expected := "anker-" + a.OS + "-" + a.Arch
		if kind == "bundle" {
			expected += ".tar.gz"
		}
		checksum, err := hex.DecodeString(a.SHA256)
		if names[a.Name] || (kind != "binary" && kind != "bundle") || a.Name != expected || a.Size < 1 || a.Size > MaxBinary || err != nil || len(checksum) != 32 {
			return m, selected, errors.New("Release-Artefakt ungültig")
		}
		names[a.Name] = true
		if kind == "binary" && a.OS == osName && a.Arch == arch {
			selected = a
		}
	}
	if selected.Name == "" {
		return m, selected, errors.New("Keine passende Linux-Version für diesen Server")
	}
	return m, selected, nil
}

type GitHub struct {
	Config Config
	Token  string
	HTTP   *http.Client
	base   string
}

func NewGitHub(c Config, token string) *GitHub {
	return &GitHub{Config: c, Token: token, base: "https://api.github.com", HTTP: &http.Client{Timeout: 5 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("zu viele Download-Weiterleitungen")
		}
		if req.URL.Scheme != "https" {
			return errors.New("unsichere Download-Weiterleitung")
		}
		// Never forward the repository credential to asset storage.
		if req.URL.Host != "api.github.com" {
			req.Header.Del("Authorization")
		}
		return nil
	}}}
}

var repoPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func (g *GitHub) fetch(ctx context.Context, path, accept string, max int64) ([]byte, error) {
	if !repoPattern.MatchString(g.Config.Repository) || strings.Contains(g.Config.Repository, "..") {
		return nil, errors.New("GitHub-Repository muss owner/repo sein")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", g.base+"/repos/"+g.Config.Repository+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "Anker-Updater")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}
	resp, err := g.HTTP.Do(req)
	if err != nil {
		return nil, errors.New("GitHub nicht erreichbar")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub antwortet mit HTTP %d; Repository, Zugriff und API-Limit prüfen", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, errors.New("Release-Download unterbrochen")
	}
	if int64(len(b)) > max {
		return nil, errors.New("Release-Download überschreitet Größenlimit")
	}
	return b, nil
}
func (g *GitHub) Check(ctx context.Context) (Release, error) {
	var result Release
	b, err := g.fetch(ctx, "/releases/latest", "application/vnd.github+json", 2<<20)
	if err != nil {
		return result, err
	}
	var release struct {
		Tag        string `json:"tag_name"`
		URL        string `json:"html_url"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		}
	}
	if err = json.Unmarshal(b, &release); err != nil {
		return result, errors.New("GitHub-Release ist nicht lesbar")
	}
	if release.Draft || release.Prerelease {
		return result, errors.New("Kein stabiles Release")
	}
	ids := map[string]int64{}
	for _, a := range release.Assets {
		if a.ID <= 0 || ids[a.Name] != 0 {
			return result, errors.New("Release-Assets uneindeutig")
		}
		ids[a.Name] = a.ID
	}
	manifestID, signatureID := ids["release.json"], ids["release.json.sig"]
	if manifestID == 0 || signatureID == 0 {
		return result, errors.New("Signierte Release-Metadaten fehlen")
	}
	raw, err := g.fetch(ctx, fmt.Sprintf("/releases/assets/%d", manifestID), "application/octet-stream", 64<<10)
	if err != nil {
		return result, err
	}
	sig, err := g.fetch(ctx, fmt.Sprintf("/releases/assets/%d", signatureID), "application/octet-stream", 1024)
	if err != nil {
		return result, err
	}
	_, artifact, err := VerifyManifest(raw, sig, g.Config.PublicKey, release.Tag, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return result, err
	}
	if ids[artifact.Name] == 0 {
		return result, errors.New("Signierte Binärdatei fehlt im Release")
	}
	u, err := url.Parse(release.URL)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" {
		return result, errors.New("Release-Link ungültig")
	}
	return Release{Version: release.Tag, URL: release.URL, Artifact: artifact, AssetID: ids[artifact.Name]}, nil
}
func (g *GitHub) Download(ctx context.Context, r Release) ([]byte, error) {
	b, err := g.fetch(ctx, fmt.Sprintf("/releases/assets/%d", r.AssetID), "application/octet-stream", r.Artifact.Size)
	if err != nil {
		return nil, err
	}
	if int64(len(b)) != r.Artifact.Size {
		return nil, errors.New("Release-Download unvollständig")
	}
	return b, nil
}
