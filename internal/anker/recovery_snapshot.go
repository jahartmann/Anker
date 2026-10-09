package anker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

type RecoverySnapshot struct {
	SourceManifestSHA   string                          `json:"source_manifest_sha,omitempty"`
	Source              Inventory                       `json:"-"`
	SourceStorageConfig []byte                          `json:"-"`
	ID                  string                          `json:"id"`
	BackupID            string                          `json:"backup_id"`
	TargetID            string                          `json:"target_id"`
	Situation           RecoverySituation               `json:"situation"`
	Scenario            string                          `json:"scenario"`
	SelectionHash       string                          `json:"selection_hash"`
	CapturedAt          string                          `json:"captured_at"`
	ExpiresAt           string                          `json:"expires_at"`
	Target              Inventory                       `json:"target"`
	ConnectionSHA       string                          `json:"connection_sha"`
	Files               []RecoveryReadFile              `json:"files"`
	StorageVolumes      map[string]RecoveryVolumeResult `json:"storage_volumes"`
}
type recoverySnapshotRecord struct {
	ID        string `json:"id"`
	SHA256    string `json:"sha256"`
	ExpiresAt string `json:"expires_at"`
}

// Connection binding includes the checked host-key reference and its contents.
// Demo/test collectors have no verified SSH transport; their explicit reference is still bound.
func (s *Service) recoveryConnectionSHA(h Host) (string, error) {
	known, err := os.ReadFile(h.KnownHostsPath)
	if err != nil {
		switch s.Collector.(type) {
		case SSHCollector, *SSHCollector:
			return "", fmt.Errorf("Hostschlüsselbezug nicht lesbar: %w", err)
		}
		known = nil
	}
	material := struct {
		ID, Address, User, KnownPath, KnownSHA string
		Port                                   int
	}{h.ID, h.Address, h.SSHUser, h.KnownHostsPath, Hash(known), h.SSHPort}
	raw, err := json.Marshal(material)
	return Hash(raw), err
}
func selectionSHA(selection RecoverySelection, r PlanRequest) string {
	storage := r.Mapping.Storage
	if len(storage) == 0 {
		storage = nil
	}
	raw, _ := json.Marshal(struct {
		Selection RecoverySelection
		Scenario  string
		Storage   map[string]string
	}{selection, r.Scenario, storage})
	return Hash(raw)
}
func inventoryComplete(i Inventory) bool {
	var results map[string]struct {
		State string `json:"state"`
	}
	var hashes map[string]string
	return json.Unmarshal(i.Details["command_results"], &results) == nil && results["file_inventory"].State == "ok" && json.Unmarshal(i.Details["file_hashes"], &hashes) == nil && hashes != nil
}
func validateSnapshotProbe(i Inventory, result RecoveryReadResult) error {
	hashes := fileHashes(i)
	var metadata map[string]json.RawMessage
	json.Unmarshal(i.Details["file_metadata"], &metadata)
	for _, f := range result.Files {
		hash := hashes[f.Path]
		if f.State == "changed" {
			return errors.New("Zielinhalt während Aufnahme geändert; Vergleich wiederholen")
		}
		if f.State == "read" {
			if hash != f.SHA256 {
				return errors.New("Zielinhalt weicht von Inventar ab; Vergleich wiederholen")
			}
			var m struct {
				Type    string            `json:"type"`
				Mode    *uint32           `json:"mode"`
				UID     *int              `json:"uid"`
				GID     *int              `json:"gid"`
				XAttrs  map[string]string `json:"xattrs"`
				Warning string            `json:"metadata_warning"`
			}
			if json.Unmarshal(metadata[f.Path], &m) != nil || m.Mode == nil || m.UID == nil || m.GID == nil || m.XAttrs == nil || m.Warning != "" || m.Type != "file" || *m.Mode != f.Metadata.Mode || *m.UID != f.Metadata.UID || *m.GID != f.Metadata.GID || !reflect.DeepEqual(m.XAttrs, f.Metadata.XAttrs) {
				return errors.New("Zielmetadaten weichen von Inventar ab oder sind unvollständig")
			}
		}
		if f.State == "missing" && hash != "" && hash != "missing" {
			return errors.New("Zieldatei seit Inventar entfernt; Vergleich wiederholen")
		}
	}
	return nil
}

// recoveryEvidencePaths derives evidence from the selected restore files and the
// target's captured inventory. It never broadens the host reader's authority.
func recoveryEvidencePaths(selection RecoverySelection, target Inventory) []string {
	paths := append([]string{}, selection.Paths...)
	paths = appendUnique(paths, "etc/pve/storage.cfg")
	for _, name := range selection.Paths {
		if strings.HasPrefix(name, "etc/network/") {
			paths = appendUnique(paths, "etc/network/interfaces")
			for targetPath := range fileHashes(target) {
				if networkEvidencePath(targetPath) {
					paths = appendUnique(paths, targetPath)
				}
			}
			break
		}
	}
	sort.Strings(paths)
	return paths
}

// validateSnapshotEvidenceSet can also validate a privately copied plan snapshot
// against its bound selection. Content/hash validation remains mandatory too.
func validateSnapshotEvidenceSet(snapshot RecoverySnapshot, selection RecoverySelection) error {
	expected := map[string]bool{}
	for _, name := range recoveryEvidencePaths(selection, snapshot.Target) {
		expected[name] = true
	}
	if len(expected) != len(snapshot.Files) {
		return errors.New("Aufnahme enthält nicht die gebundene Dateiauswahl samt erforderlichen Nachweisen")
	}
	for _, f := range snapshot.Files {
		if !expected[f.Path] {
			return errors.New("unerwarteter oder doppelter Nachweispfad in Zielaufnahme")
		}
		delete(expected, f.Path)
	}
	return nil
}

func (s *Service) CaptureRecovery(ctx context.Context, r PlanRequest) (RecoveryInspection, error) {
	release, err := s.readableLease(r.BackupID)
	if err != nil {
		return RecoveryInspection{}, err
	}
	defer release()
	return s.captureRecoveryUnlocked(ctx, r)
}

// captureRecoveryUnlocked requires a readable backup lease.
func (s *Service) captureRecoveryUnlocked(ctx context.Context, r PlanRequest) (RecoveryInspection, error) {
	if err := s.checkBackupUnlocked(r.BackupID); err != nil {
		return RecoveryInspection{}, err
	}
	b, err := s.Backup(r.BackupID)
	if err != nil {
		return RecoveryInspection{}, err
	}
	m, err := s.Manifest(b.ID)
	if err != nil {
		return RecoveryInspection{}, err
	}
	selection, err := scenarioSelection(m, r)
	if err != nil {
		return RecoveryInspection{}, err
	}
	h, err := s.Host(r.TargetID)
	if err != nil {
		return RecoveryInspection{}, err
	}
	connection := ""
	if _, ok := s.Collector.(RecoveryReader); ok {
		connection, err = s.recoveryConnectionSHA(h)
		if err != nil {
			return RecoveryInspection{}, err
		}
	}
	target, err := s.Collector.Probe(ctx, h)
	if err != nil {
		return RecoveryInspection{}, err
	}
	target.Fingerprint = Fingerprint(target)
	inspection, err := s.analyzeRecovery(b, m, target, r)
	if err != nil {
		return inspection, err
	}
	inspection.Source = sanitizeRecoveryInventory(inspection.Source)
	inspection.Target = sanitizeRecoveryInventory(inspection.Target)
	unavailable := func() (RecoveryInspection, error) {
		inspection.Automatic = false
		inspection.Warnings = appendUnique(inspection.Warnings, "Inhaltsvergleich nicht verfügbar – Hostverbindung aktualisieren")
		return inspection, nil
	}
	reader, ok := s.Collector.(RecoveryReader)
	if !ok {
		return unavailable()
	}
	switch s.Collector.(type) {
	case SSHCollector, *SSHCollector:
		var caps struct {
			Protocol int `json:"recovery_read_protocol"`
		}
		json.Unmarshal(target.Details["capabilities"], &caps)
		if caps.Protocol != 1 {
			return unavailable()
		}
	}
	paths := recoveryEvidencePaths(selection, target)
	ids := []string{}
	for _, storage := range inspection.Storage {
		id := storage.ID
		if mapped := r.Mapping.Storage[id]; mapped != "" {
			id = mapped
		}
		if recoveryStorageID.MatchString(id) {
			ids = appendUnique(ids, id)
		}
	}
	sort.Strings(ids)
	if err = validateRecoveryRequest(paths, ids); err != nil {
		return RecoveryInspection{}, err
	}
	result, err := reader.ReadRecoveryFiles(ctx, h, paths, ids)
	if err != nil {
		return RecoveryInspection{}, err
	}
	if err = validateRecoveryRead(result, paths, ids); err != nil {
		return RecoveryInspection{}, err
	}
	if err = validateSnapshotProbe(target, result); err != nil {
		return RecoveryInspection{}, err
	}
	complete := true
	for index, f := range result.Files {
		if f.State == "missing" && !inventoryComplete(target) {
			result.Files[index] = RecoveryReadFile{Path: f.Path, State: "unreadable", Error: "Zielinventar ist unvollständig; Fehlen nicht nachgewiesen"}
			f = result.Files[index]
		}
		if f.State != "read" && f.State != "missing" {
			complete = false
		}
		if f.Path == "etc/pve/storage.cfg" && f.State == "read" {
			if inspection.Target.Details == nil {
				inspection.Target.Details = map[string]json.RawMessage{}
			}
			inspection.Target.Details["storage_definitions"], _ = json.Marshal(storageDefinitions(string(f.Content)))
			inspection.TargetStorage = storageDefinitions(string(f.Content))
		}
	}
	// Incomplete optional storage inventories remain explicit; no empty-success inference.
	for _, v := range result.StorageVolumes {
		if v.State != "ok" {
			complete = false
		}
	}
	situation, _ := recoverySituation(r.Scenario)
	captured := time.Now().UTC()
	snapshot := RecoverySnapshot{SourceManifestSHA: b.ManifestSHA, ID: ID(), BackupID: b.ID, TargetID: h.ID, Situation: situation, Scenario: r.Scenario, SelectionHash: selectionSHA(selection, r), CapturedAt: captured.Format(time.RFC3339Nano), ExpiresAt: captured.Add(time.Hour).Format(time.RFC3339Nano), Target: inspection.Target, ConnectionSHA: connection, Files: result.Files, StorageVolumes: result.StorageVolumes}
	// Catch a connection edit while probing/reading before publishing evidence.
	current, err := s.Host(h.ID)
	if err != nil {
		return RecoveryInspection{}, err
	}
	currentSHA, err := s.recoveryConnectionSHA(current)
	if err != nil {
		return RecoveryInspection{}, err
	}
	if currentSHA != connection {
		return RecoveryInspection{}, errors.New("Zielverbindung während Aufnahme geändert")
	}
	if err = s.saveRecoverySnapshot(snapshot); err != nil {
		return RecoveryInspection{}, err
	}
	inspection.Files = append([]RecoveryReadFile{}, result.Files...)
	for index := range inspection.Files {
		inspection.Files[index].Content = nil
	}
	inspection.StorageVolumes = result.StorageVolumes
	inspection.SnapshotID = snapshot.ID
	inspection.ManifestSHA = b.ManifestSHA
	inspection.ComparisonComplete = complete
	if !complete {
		inspection.Automatic = false
		inspection.Warnings = appendUnique(inspection.Warnings, "Inhaltsvergleich unvollständig; Ergebniszustände der Zielaufnahme prüfen")
	}
	return inspection, nil
}

func (s *Service) saveRecoverySnapshot(snapshot RecoverySnapshot) error {
	s.snapshotMu.Lock()
	defer s.snapshotMu.Unlock()
	base, err := safeJoin(s.Root, "recovery-inspections")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(base, 0700); err != nil {
		return err
	}
	if err = os.Chmod(base, 0700); err != nil {
		return err
	}
	temp, err := os.MkdirTemp(base, ".capture-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	for _, f := range snapshot.Files {
		if f.State != "read" {
			continue
		}
		p, err := safeJoin(filepath.Join(temp, "files"), f.Path)
		if err != nil {
			return err
		}
		if err = atomicWrite(p, f.Content, 0600); err != nil {
			return err
		}
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	if err = atomicWrite(filepath.Join(temp, "snapshot.json"), data, 0600); err != nil {
		return err
	}
	// Synchronize every nested directory before publishing its root.
	if err = filepath.WalkDir(temp, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return syncDir(p)
		}
		return nil
	}); err != nil {
		return err
	}
	dest := filepath.Join(base, snapshot.ID)
	if err = os.Rename(temp, dest); err != nil {
		return err
	}
	if err = syncDir(base); err != nil {
		return err
	}
	record := recoverySnapshotRecord{ID: snapshot.ID, SHA256: Hash(data), ExpiresAt: snapshot.ExpiresAt}
	if err = s.Store.Put("recovery-inspections", snapshot.ID, record); err != nil {
		return err
	}
	return s.cleanupRecoverySnapshotsLocked(time.Now())
}
func (s *Service) cleanupRecoverySnapshotsLocked(at time.Time) error {
	records, err := records[recoverySnapshotRecord](s.Store, "recovery-inspections")
	if err != nil {
		return err
	}
	for _, record := range records {
		expires, err := time.Parse(time.RFC3339Nano, record.ExpiresAt)
		if err != nil || at.Before(expires) || !validID(record.ID) {
			continue
		}
		p, err := safeJoin(s.Root, "recovery-inspections/"+record.ID)
		if err != nil {
			return err
		}
		if err = os.RemoveAll(p); err != nil {
			return err
		}
		if err = s.Store.Delete("recovery-inspections", record.ID); err != nil {
			return err
		}
	}
	return syncDir(filepath.Join(s.Root, "recovery-inspections"))
}
func (s *Service) loadRecoverySnapshotLocked(id string) (RecoverySnapshot, error) {
	var snapshot RecoverySnapshot
	if !validID(id) {
		return snapshot, errors.New("ungültige Aufnahme-ID")
	}
	var record recoverySnapshotRecord
	if err := s.Store.Get("recovery-inspections", id, &record); err != nil {
		return snapshot, err
	}
	expires, err := time.Parse(time.RFC3339Nano, record.ExpiresAt)
	if err != nil || !time.Now().Before(expires) {
		return snapshot, errors.New("Zielaufnahme ist abgelaufen; Vergleich wiederholen")
	}
	p, err := safeJoin(s.Root, "recovery-inspections/"+id+"/snapshot.json")
	if err != nil {
		return snapshot, err
	}
	data, err := readSnapshotBytes(p, 32<<20)
	if err != nil {
		return snapshot, err
	}
	if Hash(data) != record.SHA256 {
		return snapshot, errors.New("Zielaufnahme wurde verändert")
	}
	// Snapshot manifests intentionally contain no content. Decode through metadata aliases.
	if err = json.Unmarshal(data, &snapshot); err != nil {
		return RecoverySnapshot{}, err
	}
	captured, err := time.Parse(time.RFC3339Nano, snapshot.CapturedAt)
	if err != nil || snapshot.ID != id || record.ID != id || snapshot.ExpiresAt != record.ExpiresAt || expires.Sub(captured) != time.Hour {
		return RecoverySnapshot{}, errors.New("ungültiger Aufnahmekontext")
	}
	h, err := s.Host(snapshot.TargetID)
	if err != nil {
		return RecoverySnapshot{}, err
	}
	sha, err := s.recoveryConnectionSHA(h)
	if err != nil {
		return RecoverySnapshot{}, err
	}
	if sha != snapshot.ConnectionSHA {
		return RecoverySnapshot{}, errors.New("Zielverbindung wurde geändert; Vergleich wiederholen")
	}
	for index, f := range snapshot.Files {
		if f.State != "read" {
			continue
		}
		p, err := safeJoin(s.Root, "recovery-inspections/"+id+"/files/"+f.Path)
		if err != nil {
			return RecoverySnapshot{}, err
		}
		content, err := readSnapshotBytes(p, 1<<20)
		if err != nil {
			return RecoverySnapshot{}, err
		}
		if Hash(content) != f.SHA256 || f.Metadata == nil || int64(len(content)) != f.Metadata.Size {
			return RecoverySnapshot{}, errors.New("Aufnahmeinhalt wurde verändert")
		}
		snapshot.Files[index].Content = content
	}
	return snapshot, nil
}
func readSnapshotBytes(path string, limit int64) ([]byte, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Mode().Perm() != 0600 {
		return nil, errors.New("Aufnahmedatei nicht privat oder regulär")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	bytes, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(bytes)) > limit {
		return nil, errors.New("Aufnahmedatei überschreitet Limit")
	}
	return bytes, nil
}
func (s *Service) LoadRecoverySnapshot(id string, r PlanRequest) (RecoverySnapshot, error) {
	release, err := s.readableLease(r.BackupID)
	if err != nil {
		return RecoverySnapshot{}, err
	}
	defer release()
	return s.loadRecoverySnapshotUnlocked(id, r)
}

// loadRecoverySnapshotUnlocked requires the caller's readable backup lease.
// Source context is reloaded from the immutable verified backup, never JSON input.
func (s *Service) loadRecoverySnapshotUnlocked(id string, r PlanRequest) (RecoverySnapshot, error) {
	s.snapshotMu.Lock()
	defer s.snapshotMu.Unlock()
	snapshot, err := s.loadRecoverySnapshotLocked(id)
	if err != nil {
		return RecoverySnapshot{}, err
	}
	if snapshot.BackupID != r.BackupID || snapshot.TargetID != r.TargetID || snapshot.Scenario != r.Scenario {
		return RecoverySnapshot{}, errors.New("Aufnahme gehört zu anderem Wiederherstellungskontext")
	}
	if err = s.checkBackupUnlocked(r.BackupID); err != nil {
		return RecoverySnapshot{}, err
	}
	m, err := s.Manifest(r.BackupID)
	if err != nil {
		return RecoverySnapshot{}, err
	}
	selection, err := scenarioSelection(m, r)
	if err != nil {
		return RecoverySnapshot{}, err
	}
	if snapshot.SelectionHash != selectionSHA(selection, r) {
		return RecoverySnapshot{}, errors.New("Dateiauswahl wurde geändert; Vergleich wiederholen")
	}
	if err = validateSnapshotEvidenceSet(snapshot, selection); err != nil {
		return RecoverySnapshot{}, err
	}
	b, err := s.Backup(r.BackupID)
	if err != nil {
		return RecoverySnapshot{}, err
	}
	if snapshot.SourceManifestSHA != "" && snapshot.SourceManifestSHA != b.ManifestSHA {
		return RecoverySnapshot{}, errors.New("Quellstand wurde geändert; Vergleich wiederholen")
	}
	snapshot.Source = m.Inventory
	for _, e := range m.Entries {
		if e.Path != "etc/pve/storage.cfg" {
			continue
		}
		if e.Type != "file" {
			break
		}
		content, _, readErr := s.readFileUnlocked(r.BackupID, e.Path, 64<<20)
		if readErr != nil {
			return RecoverySnapshot{}, readErr
		}
		snapshot.SourceStorageConfig = append([]byte{}, content...)
		break
	}
	return snapshot, nil
}
func (s *Service) ReadSnapshotFile(id, path string) (RecoveryReadFile, error) {
	s.snapshotMu.Lock()
	defer s.snapshotMu.Unlock()
	if err := ValidPath(path); err != nil {
		return RecoveryReadFile{}, err
	}
	snapshot, err := s.loadRecoverySnapshotLocked(id)
	if err != nil {
		return RecoveryReadFile{}, err
	}
	for _, f := range snapshot.Files {
		if f.Path == path {
			return f, nil
		}
	}
	return RecoveryReadFile{}, errors.New("Datei nicht in Zielaufnahme")
}
