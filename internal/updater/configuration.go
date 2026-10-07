package updater

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

const OfficialRepository = "jahartmann/Anker"

//go:embed official_public.key
var officialPublicKey string

var OfficialPublicKey = strings.TrimSpace(officialPublicKey)

// Configuration exposes public trust information only; credentials and paths stay in the helper.
type Configuration struct {
	Repository          string `json:"repository"`
	PublicKey           string `json:"public_key"`
	Fingerprint         string `json:"fingerprint"`
	Configured          bool   `json:"configured"`
	Official            bool   `json:"official"`
	HasToken            bool   `json:"has_token"`
	OfficialRepository  string `json:"official_repository"`
	OfficialPublicKey   string `json:"official_public_key"`
	OfficialFingerprint string `json:"official_fingerprint"`
	Demo                bool   `json:"demo,omitempty"`
}
type ConfigureRequest struct {
	Repository string `json:"repository"`
	PublicKey  string `json:"public_key"`
	Confirmed  bool   `json:"confirmed"`
}

func PublicKeyFingerprint(key string) (string, error) {
	raw, err := base64.StdEncoding.Strict().DecodeString(strings.TrimSpace(key))
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return "", errors.New("Öffentlicher Ed25519-Signierschlüssel muss 32 Bytes als Base64 enthalten")
	}
	hash := sha256.Sum256(raw)
	return "SHA256:" + hex.EncodeToString(hash[:]), nil
}
func validateConfig(c Config) error {
	if !repoPattern.MatchString(c.Repository) || strings.Contains(c.Repository, "..") || len(c.Repository) > 200 {
		return errors.New("GitHub-Repository muss owner/repo sein")
	}
	_, err := PublicKeyFingerprint(c.PublicKey)
	return err
}
func DefaultConfiguration() Configuration {
	fingerprint, _ := PublicKeyFingerprint(OfficialPublicKey)
	return Configuration{Repository: OfficialRepository, PublicKey: OfficialPublicKey, Fingerprint: fingerprint, Official: true, OfficialRepository: OfficialRepository, OfficialPublicKey: OfficialPublicKey, OfficialFingerprint: fingerprint}
}
func (s *Server) configuration() Configuration {
	c := DefaultConfiguration()
	if s.state.Configured && s.source != nil {
		c.Repository = s.source.Config.Repository
		c.PublicKey = s.source.Config.PublicKey
		c.Fingerprint, _ = PublicKeyFingerprint(c.PublicKey)
		c.Configured = true
		c.Official = c.Repository == OfficialRepository && c.PublicKey == OfficialPublicKey
		c.HasToken = s.source.Token != ""
	}
	return c
}
func (s *Server) configurationManager() *TLSManager {
	if s.tlsManager != nil {
		return s.tlsManager
	}
	return &TLSManager{Dir: filepath.Dir(ConfigPath)}
}
func (s *Server) configurationHandler(w http.ResponseWriter, r *http.Request) {
	fail := func(code int, message string) {
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]string{"error": message})
	}
	if r.URL.Path == "/configuration" && r.Method == "GET" {
		s.mu.Lock()
		defer s.mu.Unlock()
		json.NewEncoder(w).Encode(s.configuration())
		return
	}
	if r.URL.Path != "/configure" || r.Method != "POST" {
		fail(405, "Methode nicht unterstützt")
		return
	}
	if r.Header.Get("X-Anker-Request") != "1" {
		fail(403, "Anfrageherkunft ungültig")
		return
	}
	var in ConfigureRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		fail(400, "Ungültige Updatequelle")
		return
	}
	if decoder.Decode(new(any)) != io.EOF {
		fail(400, "Zusätzliche Daten nicht erlaubt")
		return
	}
	if !in.Confirmed {
		fail(400, "Vertrauen in Repository und Signierschlüssel ausdrücklich bestätigen")
		return
	}
	in.Repository = strings.TrimSpace(in.Repository)
	in.PublicKey = strings.TrimSpace(in.PublicKey)
	if in.Repository == "" {
		in.Repository = OfficialRepository
	}
	if in.PublicKey == "" && in.Repository == OfficialRepository {
		in.PublicKey = OfficialPublicKey
	}
	next := Config{Repository: in.Repository, PublicKey: in.PublicKey}
	if err := validateConfig(next); err != nil {
		fail(400, err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy {
		fail(409, "Updateprüfung oder Systemänderung läuft; erneut versuchen")
		return
	}
	manager := s.configurationManager()
	unlock, err := manager.lock()
	if err != nil {
		if errors.Is(err, ErrTLSBusy) {
			fail(409, "Einrichtung läuft oder ist unvollständig; zuerst abschließen")
		} else {
			fail(400, "Einrichtungssperre prüfen")
		}
		return
	}
	defer unlock()
	// Recheck after acquiring setup.lock: a setup may have completed its write meanwhile.
	if err := setupConfigurationIdle(manager.Dir); err != nil {
		fail(409, "Einrichtung unvollständig; zuerst abschließen")
		return
	}
	stateDir := StateDir
	if s.installer != nil {
		stateDir = s.installer.StateDir
	}
	for _, path := range []string{filepath.Join(manager.Dir, "update-maintenance"), filepath.Join(stateDir, "pending.json")} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			fail(409, "Update oder Wartung läuft; zuerst abschließen")
			return
		}
	}
	fd, err := unix.Open(filepath.Join(stateDir, "local-install.lock"), unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0600)
	if err != nil {
		fail(400, "Updatesperre nicht verfügbar")
		return
	}
	defer unix.Close(fd)
	var info unix.Stat_t
	if err = unix.Fstat(fd, &info); err != nil || info.Mode&unix.S_IFMT != unix.S_IFREG || info.Uid != manager.ownerUID || info.Mode&0022 != 0 {
		fail(400, "Ungültige Updatesperre")
		return
	}
	if err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		fail(409, "Lokales Update läuft; erneut versuchen")
		return
	}
	path := filepath.Join(manager.Dir, "update.json")
	previous, _, readErr := manager.read(path, 64<<10)
	if readErr != nil && !os.IsNotExist(readErr) {
		fail(400, "Vorhandene Update-Konfiguration ist nicht sicher lesbar")
		return
	}
	token := ""
	if readErr == nil {
		var old Config
		d := json.NewDecoder(bytes.NewReader(previous))
		d.DisallowUnknownFields()
		if d.Decode(&old) != nil || d.Decode(new(any)) != io.EOF || validateConfig(old) != nil {
			fail(400, "Vorhandene Update-Konfiguration ungültig; auf dem Server prüfen")
			return
		}
		if old.Repository == next.Repository && old.TokenFile != "" {
			data, _, err := manager.read(old.TokenFile, 4096)
			if err != nil {
				fail(400, "Vorhandener Repositoryzugang nicht sicher lesbar")
				return
			}
			next.TokenFile = old.TokenFile
			token = strings.TrimSpace(string(data))
		}
	}
	encoded, _ := json.Marshal(next)
	if err := atomic(path, encoded, 0600, -1, -1); err != nil {
		// atomic can report a directory sync failure after rename. Restore previous trust before returning an error.
		var restoreErr error
		if readErr == nil {
			restoreErr = atomic(path, previous, 0600, -1, -1)
		} else {
			restoreErr = os.Remove(path)
			if os.IsNotExist(restoreErr) {
				restoreErr = nil
			}
			if restoreErr == nil {
				restoreErr = syncDir(manager.Dir)
			}
		}
		if restoreErr != nil {
			s.source = nil
			s.state.Configured = false
			s.state.Repository = ""
			s.state.Target = ""
			s.state.Available = nil
			s.state.CheckedAt = ""
			s.state.Status = "unconfigured"
			s.state.Message = "Update-Konfiguration nach Speicherfehler unklar; auf dem Server prüfen"
			fail(500, s.state.Message)
			return
		}
		fail(500, "Updatequelle konnte nicht gespeichert werden")
		return
	}
	s.source = NewGitHub(next, token)
	s.state.Configured = true
	s.state.Repository = next.Repository
	s.state.Available = nil
	s.state.CheckedAt = ""
	s.state.Target = ""
	s.state.Status = "idle"
	s.state.Message = "Updatequelle eingerichtet. Jetzt nach signierten Releases suchen."
	// The config is authoritative. Status persistence is best effort; losing it cannot change trust.
	if s.installer != nil {
		s.save()
	}
	json.NewEncoder(w).Encode(s.configuration())
}
