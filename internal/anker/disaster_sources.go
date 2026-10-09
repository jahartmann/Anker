package anker

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Source inventory is package coverage, never a new integrity receipt. Digests
// are the catalog's known manifest bindings, including for unavailable media.
type DisasterSource struct {
	BackupID    string `json:"backup_id"`
	HostID      string `json:"host_id"`
	ManifestSHA string `json:"manifest_sha"`
	State       string `json:"state"` // included, missing, separate
}

func (s DisasterSource) prefix() string { return "hosts/" + s.HostID + "/backups/" + s.BackupID }
func disasterSourceInventory(p *disasterPackage) error {
	previous := map[string]DisasterSource{}
	for _, r := range p.Records {
		if r.Bucket == "disaster-sources" {
			var old DisasterSource
			if json.Unmarshal(r.Value, &old) == nil {
				previous[r.ID] = old
			}
		}
	}
	p.Sources = []DisasterSource{}
	p.sourceIndex = map[string]int{}
	for _, r := range p.Records {
		if r.Bucket != "backups" {
			continue
		}
		var b Backup
		if err := json.Unmarshal(r.Value, &b); err != nil {
			return err
		}
		if b.ID != r.ID || !validID(b.ID) || !validID(b.HostID) || (b.ManifestSHA != "" && !validSHA256(b.ManifestSHA)) {
			return errors.New("Ungültiger Sicherungseintrag")
		}
		source := DisasterSource{b.ID, b.HostID, b.ManifestSHA, "missing"}
		if old, ok := previous[b.ID]; ok && old.HostID == b.HostID && old.ManifestSHA == b.ManifestSHA && old.State == "separate" {
			source.State = "separate"
		}
		p.sourceIndex[source.prefix()] = len(p.Sources)
		p.Sources = append(p.Sources, source)
	}
	return nil
}
func disasterValidateSources(p *disasterPackage) error {
	if p.Version == 1 {
		if len(p.Sources) != 0 {
			return errors.New("Altes Disaster-Format mit neuem Quelleninventar")
		}
		return nil
	}
	if len(p.Sources) > disasterMaxEntries {
		return errors.New("Quelleninventarlimit überschritten")
	}
	expected := map[string]Backup{}
	for _, r := range p.Records {
		if r.Bucket == "backups" {
			var b Backup
			if err := json.Unmarshal(r.Value, &b); err != nil {
				return err
			}
			if b.ID != r.ID || !validID(b.ID) || !validID(b.HostID) || (b.ManifestSHA != "" && !validSHA256(b.ManifestSHA)) {
				return errors.New("Ungültiger Sicherungseintrag")
			}
			expected[b.ID] = b
		}
	}
	if len(expected) != len(p.Sources) {
		return errors.New("Quelleninventar unvollständig")
	}
	dataPresent := map[string]bool{}
	manifests := map[string]string{}
	for _, f := range p.Files {
		parts := strings.SplitN(f.Path, "/", 5)
		if len(parts) >= 4 && parts[0] == "hosts" && parts[2] == "backups" {
			prefix := strings.Join(parts[:4], "/")
			dataPresent[prefix] = true
			if len(parts) == 5 && parts[4] == "manifest.json" {
				manifests[prefix] = f.SHA256
			}
		}
	}
	seen := map[string]bool{}
	for _, source := range p.Sources {
		b, ok := expected[source.BackupID]
		if !ok || seen[source.BackupID] || source.HostID != b.HostID || source.ManifestSHA != b.ManifestSHA {
			return errors.New("Quelleninventar widerspricht Katalogbindung")
		}
		seen[source.BackupID] = true
		if source.State != "included" && source.State != "missing" && source.State != "separate" {
			return errors.New("Unbekannte Quellenabdeckung")
		}
		prefix := source.prefix()
		if source.State != "included" && dataPresent[prefix] {
			return errors.New("Ausgelassene Quelle enthält teilweise Sicherungsdaten")
		}
		if source.State == "included" {
			digest, ok := manifests[prefix]
			if !ok {
				return errors.New("Enthaltene Quelle ohne Manifest")
			}
			if digest != source.ManifestSHA {
				return errors.New("Quellenmanifest widerspricht Katalogbindung")
			}
		}
	}
	return nil
}

// Split mode captures and verifies one source at a time using the same pinned
// walker and byte limits as included sources. The aggregate package need not
// contain all retained backups. Nothing follows a catalog/external pathname.
func disasterCheckSeparate(dir *os.File, prefix string, p *disasterPackage, source DisasterSource) error {
	piece := &disasterPackage{Version: 2, Sources: []DisasterSource{{source.BackupID, source.HostID, source.ManifestSHA, "included"}}}
	for _, r := range p.Records {
		if r.Bucket == "backups" && r.ID == source.BackupID {
			piece.Records = append(piece.Records, r)
		}
	}
	var total int64
	nodes := 0
	if err := disasterWalk(dir, prefix, piece, false, &total, &nodes); err != nil {
		return err
	}
	// Archived material validation checks the manifest inside the archive; the
	// captured loose manifest must independently exist and match the catalog.
	if err := disasterValidateSources(piece); err != nil {
		return fmt.Errorf("Separate Quelle ist vorhanden, aber ungültig: %w", err)
	}
	stage, err := os.MkdirTemp("", "anker-separate-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err = disasterMaterialize(piece, stage); err != nil {
		return err
	}
	if err = disasterCheckMaterial(piece, stage); err != nil {
		return fmt.Errorf("Separate Quelle ist vorhanden, aber ungültig: %w", err)
	}
	return nil
}
