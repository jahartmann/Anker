package anker

import (
	"anker/internal/updater"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"
)

func NewService(root string, store *Store, collector Collector) (*Service, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	s := &Service{Root: root, Store: store, Collector: collector, locks: map[string]bool{}, cancels: map[string]context.CancelFunc{}}
	if _, err := os.Stat(updater.Maintenance); err == nil {
		s.maintenance = true
	}
	for _, d := range []string{"hosts", "plans", "exports", "staging", "clusters"} {
		if err = os.MkdirAll(filepath.Join(root, d), 0700); err != nil {
			return nil, err
		}
	}
	var settings Settings
	if err = store.Get("settings", "main", &settings); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("Betriebseinstellungen sind nicht lesbar; Katalog prüfen: %w", err)
		}
		if err = store.Put("settings", "main", Settings{SessionDays: 30, Timezone: "Europe/Berlin", Schedule: "02:00", Parallel: 4, Retries: 3, Daily: 30, Weekly: 12, Monthly: 12, ArchiveDays: 90, StaleHours: 26}); err != nil {
			return nil, err
		}
	}
	return s, nil
}
func (s *Service) Settings() (Settings, error) {
	var v Settings
	err := s.Store.Get("settings", "main", &v)
	if v.SessionDays == 0 {
		v.SessionDays = 30
	}
	return v, err
}

var safeHost = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]*$`)

func (s *Service) SaveHost(h Host) error {
	s.hostMu.Lock()
	defer s.hostMu.Unlock()
	// Operator forms may be older than the last completed probe/backup.
	if current, err := s.Host(h.ID); err == nil {
		h.Inventory, h.LastProbe, h.ProbeError = current.Inventory, current.LastProbe, current.ProbeError
		if !sameProbeTarget(current, h) {
			h.Inventory = nil
			h.LastProbe = ""
			h.ProbeError = ""
		}
	}
	return s.saveHost(h)
}
func sameProbeTarget(a, b Host) bool {
	return a.Name == b.Name && a.Address == b.Address && a.SSHPort == b.SSHPort && a.SSHUser == b.SSHUser && a.KeyPath == b.KeyPath && a.KnownHostsPath == b.KnownHostsPath
}
func (s *Service) updateHostInventory(expected Host, inv *Inventory, probeError string) error {
	s.hostMu.Lock()
	defer s.hostMu.Unlock()
	h, err := s.Host(expected.ID)
	if err != nil {
		return err
	}
	if !sameProbeTarget(expected, h) {
		return nil
	}
	if inv != nil {
		h.Inventory = inv
	}
	h.LastProbe, h.ProbeError = now(), probeError
	return s.saveHost(h)
}
func (s *Service) saveHost(h Host) error {
	if h.ID == "" {
		h.ID = ID()
	}
	if !validID(h.ID) || !safeHost.MatchString(h.Name) || (!safeHost.MatchString(h.Address) && net.ParseIP(h.Address) == nil) {
		return errors.New("Host-ID, Hostname oder Adresse ungültig")
	}
	if h.SSHUser == "" {
		h.SSHUser = "anker"
	}
	if h.KeyPath == "" {
		h.KeyPath = "/etc/anker/keys/backup"
	}
	if h.KnownHostsPath == "" {
		h.KnownHostsPath = "/etc/anker/known_hosts"
	}
	if !safeHost.MatchString(h.SSHUser) || strings.Contains(h.SSHUser, ":") {
		return errors.New("SSH-Benutzer ungültig")
	}
	if h.RestoreKeyPath != "" {
		if h.RestoreSSHUser == "" {
			h.RestoreSSHUser = "anker-restore"
		}
		if h.RestoreKeyPath == h.KeyPath || h.RestoreSSHUser == h.SSHUser || !filepath.IsAbs(h.RestoreKeyPath) || !safeHost.MatchString(h.RestoreSSHUser) || strings.Contains(h.RestoreSSHUser, ":") {
			return errors.New("separater gültiger Wiederherstellungszugang erforderlich")
		}
	}
	if h.SSHPort == 0 {
		h.SSHPort = 22
	}
	if h.SSHPort < 1 || h.SSHPort > 65535 {
		return errors.New("SSH-Port ungültig")
	}
	if h.Schedule != "" {
		if _, err := time.Parse("15:04", h.Schedule); err != nil {
			return errors.New("Zeitplan muss HH:MM sein")
		}
	}
	for _, p := range h.ExtraPaths {
		if !strings.HasPrefix(p, "/") || strings.Contains(p, "..") || strings.ContainsAny(p, "\x00\n\r") {
			return errors.New("Zusatzpfad ungültig")
		}
	}
	return s.Store.Put("hosts", h.ID, h)
}
func (s *Service) Host(id string) (Host, error) {
	var h Host
	if !validID(id) {
		return h, errors.New("ungültige Host-ID")
	}
	err := s.Store.Get("hosts", id, &h)
	return h, err
}
func (s *Service) Hosts() ([]Host, error) {
	hosts, err := records[Host](s.Store, "hosts")
	sort.Slice(hosts, func(i, j int) bool { return hosts[i].Name < hosts[j].Name })
	return hosts, err
}
func (s *Service) ListBackups(hostID string) ([]Backup, error) {
	all, err := records[Backup](s.Store, "backups")
	if err != nil {
		return nil, err
	}
	out := []Backup{}
	for _, b := range all {
		if hostID == "" || b.HostID == hostID {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}
func (s *Service) Backup(id string) (Backup, error) {
	var b Backup
	if !validID(id) {
		return b, errors.New("ungültige Sicherungs-ID")
	}
	err := s.Store.Get("backups", id, &b)
	if err == nil && (!validID(b.HostID) || b.ID != id) {
		return b, errors.New("ungültiger Katalogeintrag")
	}
	return b, err
}
func (s *Service) backupDir(b Backup) string {
	return filepath.Join(s.Root, "hosts", b.HostID, "backups", b.ID)
}
func (s *Service) acquire(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locks[id] {
		return errors.New("auf diesem Host läuft bereits ein Auftrag")
	}
	s.locks[id] = true
	return nil
}
func (s *Service) release(id string) { s.mu.Lock(); delete(s.locks, id); s.mu.Unlock() }
func (s *Service) CreateBackup(ctx context.Context, hostID string) (Backup, error) {
	if err := s.acquire(hostID); err != nil {
		return Backup{}, err
	}
	defer s.release(hostID)
	return s.createBackup(ctx, hostID)
}
func (s *Service) createBackup(ctx context.Context, hostID string) (Backup, error) {
	h, err := s.Host(hostID)
	if err != nil {
		return Backup{}, err
	}
	previous, _ := s.ListBackups(hostID)
	estimate := int64(64 << 20)
	if len(previous) > 0 {
		estimate = previous[0].Size*2 + (64 << 20)
	}
	if err = s.CheckSpace(estimate); err != nil {
		return Backup{}, err
	}
	b := Backup{ID: ID(), HostID: h.ID, HostName: h.Name, CreatedAt: now()}
	stage := filepath.Join(s.Root, "staging", b.ID)
	defer os.RemoveAll(stage)
	if err = os.MkdirAll(filepath.Join(stage, "files"), 0700); err != nil {
		return b, err
	}
	c, err := s.Collector.Collect(ctx, h, stage)
	if err != nil {
		return b, err
	}
	if err = ctx.Err(); err != nil {
		return b, err
	}
	// Collectors place the raw tree under stage/files.
	seen := map[string]bool{}
	for i := range c.Entries {
		e := &c.Entries[i]
		if err = ValidPath(e.Path); err != nil {
			return b, err
		}
		if seen[e.Path] {
			return b, errors.New("doppelter Dateipfad")
		}
		seen[e.Path] = true
		e.Secret = e.Secret || isSecret(e.Path)
		p, err := safeJoin(filepath.Join(stage, "files"), e.Path)
		if err != nil {
			return b, err
		}
		switch e.Type {
		case "directory":
			if err = os.MkdirAll(p, 0700); err != nil {
				return b, err
			}
		case "symlink":
			if err = atomicWrite(p, []byte(e.Link), 0600); err != nil {
				return b, err
			}
			e.SHA256 = Hash([]byte(e.Link))
			e.Size = int64(len(e.Link))
		case "file":
			data, err := os.ReadFile(p)
			if err != nil {
				return b, err
			}
			e.SHA256 = Hash(data)
			e.Secret = e.Secret || secretContent(data)
			e.Size = int64(len(data))
			if err = os.Chmod(p, 0600); err != nil {
				return b, err
			}
		default:
			return b, errors.New("nicht unterstützter Dateityp")
		}
		b.Size += e.Size
	}
	for _, required := range h.ExtraPaths {
		rel := strings.TrimPrefix(required, "/")
		if !seen[rel] {
			c.Warnings = append(c.Warnings, "Zusätzlicher Pflichtpfad fehlt im Hostprofil: "+required)
		}
	}
	sort.Slice(c.Entries, func(i, j int) bool { return c.Entries[i].Path < c.Entries[j].Path })
	b.Files = len(c.Entries)
	b.Status = "successful"
	if len(c.Warnings) > 0 {
		b.Status = "partial"
	}
	b.Warnings = c.Warnings
	c.Inventory.Fingerprint = Fingerprint(c.Inventory)
	m := Manifest{Version: 1, ID: b.ID, HostID: h.ID, CreatedAt: b.CreatedAt, CompletedAt: now(), Status: b.Status, Entries: c.Entries, Warnings: c.Warnings, Inventory: c.Inventory, Consistency: "SQLite snapshot; ordinary files captured over documented interval", Artifacts: map[string]string{}}
	if err = writeJSON(filepath.Join(stage, "inventory/host.json"), c.Inventory); err != nil {
		return b, err
	}
	if err = atomicWrite(filepath.Join(stage, "WIEDERHERSTELLUNG.md"), []byte(recoveryGuide(m)), 0600); err != nil {
		return b, err
	}
	for _, p := range []string{"inventory/host.json", "WIEDERHERSTELLUNG.md", "recovery/config.db"} {
		full, _ := safeJoin(stage, p)
		data, err := os.ReadFile(full)
		if os.IsNotExist(err) && p == "recovery/config.db" {
			continue
		}
		if err != nil {
			return b, err
		}
		m.Artifacts[p] = Hash(data)
	}
	if err = writeJSON(filepath.Join(stage, "manifest.json"), m); err != nil {
		return b, err
	}
	data, err := os.ReadFile(filepath.Join(stage, "manifest.json"))
	if err != nil {
		return b, err
	}
	b.ManifestSHA = Hash(data)
	var sums strings.Builder
	for _, e := range m.Entries {
		if e.Type != "directory" {
			fmt.Fprintf(&sums, "%s  files/%s\n", e.SHA256, e.Path)
		}
	}
	for p, hash := range m.Artifacts {
		fmt.Fprintf(&sums, "%s  %s\n", hash, p)
	}
	fmt.Fprintf(&sums, "%s  manifest.json\n", b.ManifestSHA)
	if err = atomicWrite(filepath.Join(stage, "checksums.sha256"), []byte(sums.String()), 0600); err != nil {
		return b, err
	}
	target := s.backupDir(b)
	if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return b, err
	}
	if err = syncTree(stage); err != nil {
		return b, err
	}
	if err = os.Rename(stage, target); err != nil {
		return b, err
	}
	if err = syncDir(filepath.Dir(target)); err != nil {
		return b, err
	}
	if err = s.Store.Put("backups", b.ID, b); err != nil {
		return b, err
	}
	if err = s.updateHostInventory(h, &c.Inventory, ""); err != nil {
		return b, err
	}
	return b, nil
}
func (s *Service) Manifest(id string) (Manifest, error) {
	b, err := s.Backup(id)
	if err != nil {
		return Manifest{}, err
	}
	data, err := os.ReadFile(filepath.Join(s.backupDir(b), "manifest.json"))
	if err != nil {
		return Manifest{}, err
	}
	if Hash(data) != b.ManifestSHA {
		return Manifest{}, errors.New("Manifest wurde verändert")
	}
	var m Manifest
	if err = json.Unmarshal(data, &m); err != nil {
		return m, err
	}
	if m.ID != b.ID || m.HostID != b.HostID || m.Version != FormatVersion {
		return m, errors.New("nicht unterstütztes oder falsches Manifest")
	}
	return m, nil
}
func (s *Service) VerifyBackup(id string) error {
	lock := s.backupLock(id)
	lock.RLock()
	defer lock.RUnlock()
	return s.verifyBackupUnlocked(id)
}
func (s *Service) verifyBackupUnlocked(id string) error {
	err := s.checkBackupUnlocked(id)
	b, getErr := s.Backup(id)
	if getErr != nil {
		return err
	}
	// Resource/access failures do not establish corruption.
	if errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EACCES) {
		return err
	}
	b.VerifiedAt = now()
	if err != nil {
		b.Status, b.VerificationError = "damaged", err.Error()
	} else {
		b.VerificationError = ""
		if b.Status == "damaged" {
			if m, e := s.Manifest(id); e == nil {
				b.Status = m.Status
			}
		}
	}
	if storeErr := s.Store.Put("backups", id, b); err == nil {
		err = storeErr
	}
	return err
}
func (s *Service) checkBackupUnlocked(id string) error {
	b, err := s.Backup(id)
	if err != nil {
		return err
	}
	if b.Archived {
		temp, err := s.extractArchive(b)
		if err != nil {
			return err
		}
		defer os.RemoveAll(temp)
		return nil
	}
	m, err := s.Manifest(id)
	if err != nil {
		return err
	}
	for _, e := range m.Entries {
		if e.Type == "directory" {
			continue
		}
		p, err := safeJoin(filepath.Join(s.backupDir(b), "files"), e.Path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if Hash(data) != e.SHA256 {
			return fmt.Errorf("Prüfsumme falsch: %s", e.Path)
		}
	}
	for p, hash := range m.Artifacts {
		full, err := safeJoin(s.backupDir(b), p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return err
		}
		if Hash(data) != hash {
			return fmt.Errorf("Prüfsumme falsch: %s", p)
		}
	}
	return nil
}
func (s *Service) ReadFile(id, p string) ([]byte, Entry, error) {
	release, err := s.readableLease(id)
	if err != nil {
		return nil, Entry{}, err
	}
	defer release()
	return s.readFileUnlocked(id, p, 8<<20)
}
func (s *Service) readFileUnlocked(id, p string, limit int64) ([]byte, Entry, error) {
	m, err := s.Manifest(id)
	if err != nil {
		return nil, Entry{}, err
	}
	var selected *Entry
	for _, e := range m.Entries {
		if e.Path == p {
			v := e
			selected = &v
			break
		}
	}
	if selected == nil || selected.Type == "directory" {
		return nil, Entry{}, errors.New("Datei nicht vorhanden")
	}
	b, _ := s.Backup(id)
	full, err := safeJoin(filepath.Join(s.backupDir(b), "files"), p)
	if err != nil {
		return nil, Entry{}, err
	}
	if selected.Size > limit {
		return nil, Entry{}, errors.New("Datei zu groß für Vorschau; vollständigen Export verwenden")
	}
	data, err := os.ReadFile(full)
	if err == nil && Hash(data) != selected.SHA256 {
		err = errors.New("Datei-Prüfsumme falsch")
	}
	selected.Secret = selected.Secret || isSecret(selected.Path) || secretContent(data)
	return data, *selected, err
}
