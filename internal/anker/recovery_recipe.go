package anker

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

// A recipe is reviewed intent, never an executable plan or verification result.
type RecoveryRecipe struct {
	ID           string `json:"id"`
	Version      int    `json:"version"`
	OriginPlanID string `json:"origin_plan_id"`
	Label        string `json:"label"`
	ReviewedBy   string `json:"reviewed_by"`
	ReviewedAt   string `json:"reviewed_at"`
}
type recoveryRecipePrivate struct {
	RecoveryRecipe
	OriginSnapshotID string            `json:"origin_snapshot_id"`
	BackupID         string            `json:"backup_id"`
	ManifestSHA      string            `json:"manifest_sha"`
	Scenario         string            `json:"scenario"`
	Files            []string          `json:"files"`
	Mapping          Mapping           `json:"mapping"`
	Constraints      map[string]string `json:"constraints"`
	MissingProof     []string          `json:"missing_proof,omitempty"`
}
type recoveryRecipeRecord struct {
	RecoveryRecipe
	SHA256 string `json:"sha256"`
}
type RecoveryRecipePreview struct {
	Recipe      RecoveryRecipe `json:"recipe"`
	SnapshotID  string         `json:"snapshot_id"`
	Applicable  bool           `json:"applicable"`
	Suggestions Mapping        `json:"suggestions"`
	Mismatches  []string       `json:"mismatches"`
}

func recipeHash(value any) string { raw, _ := json.Marshal(value); return Hash(raw) }
func recipeConstraints(snapshot RecoverySnapshot) map[string]string {
	identity, _ := verificationBaseline(snapshot.Target)
	constraints := map[string]string{
		"Zielidentität":                  identity,
		"Zielverbindung":                 snapshot.ConnectionSHA,
		"Version":                        recipeHash([]string{snapshot.Target.PVEVersion, snapshot.Target.Debian, snapshot.Target.Kernel}),
		"Netzwerkports (Name, MAC, PCI)": recipeHash(snapshot.Target.Interfaces),
		"Topologie":                      recipeHash([]any{snapshot.Target.ClusterID, snapshot.Target.BootMode, snapshot.Target.Details["cluster"]}),
		"Datenträger und Mounts":         recipeHash(snapshot.Target.Disks),
	}
	// Bind configuration evidence including metadata. Never use display names or
	// public provenance as a substitute for private, successfully captured proof.
	evidence := []RecoveryReadFile{}
	for _, f := range snapshot.Files {
		if strings.HasPrefix(f.Path, "etc/network/") || f.Path == "etc/pve/storage.cfg" || f.Path == "etc/pve/corosync.conf" {
			evidence = append(evidence, f)
		}
	}
	sort.Slice(evidence, func(i, j int) bool { return evidence[i].Path < evidence[j].Path })
	constraints["Netzwerk- und Speicherbestand"] = recipeHash(evidence)
	constraints["Volumes"] = recipeHash(snapshot.StorageVolumes)
	return constraints
}
func recipeProof(snapshot RecoverySnapshot, mapping Mapping) []string {
	missing := []string{}
	identity, _ := verificationBaseline(snapshot.Target)
	if identity == "" {
		missing = append(missing, "Zielidentität: privater Maschinennachweis fehlt")
	}
	var results map[string]InventoryQueryState
	json.Unmarshal(snapshot.Target.Details["command_results"], &results)
	if results["cluster"].State != "ok" && results["cluster"].State != "not_applicable" {
		missing = append(missing, "Topologie: privater Clusternachweis fehlt")
	}
	if snapshot.Target.PVEVersion == "" || results["pve_version"].State != "ok" {
		missing = append(missing, "Version: Zielnachweis fehlt")
	}
	if len(mapping.Interfaces) > 0 || len(mapping.Network) > 0 {
		topology := false
		for _, file := range snapshot.Files {
			if strings.HasPrefix(file.Path, "etc/network/") {
				topology = true
				if file.State != "read" && file.State != "missing" {
					missing = append(missing, "Netzwerktopologie: Dateinachweis nicht vollständig")
				}
			}
		}
		if !topology {
			missing = append(missing, "Netzwerktopologie: aufgenommene Konfiguration fehlt")
		}
		var results map[string]InventoryQueryState
		json.Unmarshal(snapshot.Target.Details["command_results"], &results)
		if results["interfaces"].State != "ok" {
			missing = append(missing, "Netzwerkports: erfolgreicher Zielnachweis fehlt")
		}
		for _, name := range mapping.Interfaces {
			found := false
			for _, p := range snapshot.Target.Interfaces {
				if p.Name == name && physicalPort(p) && p.MAC != "" && p.PCI != "" {
					found = true
				}
			}
			if !found {
				missing = append(missing, "Netzwerkport "+name+": Name, MAC oder PCI nicht belegt")
			}
		}
	}
	for _, id := range mapping.Storage {
		if id == "manual" {
			continue
		}
		var config []byte
		for _, f := range snapshot.Files {
			if f.Path == "etc/pve/storage.cfg" && f.State == "read" {
				config = f.Content
			}
		}
		found := false
		for _, storage := range storageDefinitions(string(config)) {
			if storage.ID != id || !storage.Verified {
				continue
			}
			found = true
			// Current collectors expose no stable pool/remote backend IDs. For
			// directory storage require an exact observed filesystem mount, not
			// a name, parent path guess, zpool/vgs display name or config alone.
			backing := false
			if storage.Kind == "dir" && results["disks"].State == "ok" {
				for _, disk := range snapshot.Target.Disks {
					if disk.Mount == storage.Backing.Path && disk.UUID != "" && disk.ID != "" && disk.Filesystem != "" {
						backing = true
					}
				}
			}
			if !backing {
				missing = append(missing, "Speicher "+id+": tatsächlicher Datenträger-/Mountnachweis fehlt")
			}
		}
		if !found {
			missing = append(missing, "Speicher "+id+": Backing nicht belegt")
		}
		if v, ok := snapshot.StorageVolumes[id]; !ok || v.State != "ok" {
			missing = append(missing, "Speicher "+id+": Volumebestand nicht belegt")
		}
	}
	return missing
}
func (s *Service) SaveRecoveryRecipe(planID, label, actor string, reviewed bool) (RecoveryRecipe, error) {
	var zero RecoveryRecipe
	label = strings.TrimSpace(label)
	if !reviewed || label == "" || len(label) > 120 || actor == "" {
		return zero, errors.New("Bezeichnung und ausdrückliche Prüfung der Zuordnungen erforderlich")
	}
	lock := s.planLock(planID)
	lock.RLock()
	defer lock.RUnlock()
	p, err := s.Plan(planID)
	if err != nil {
		return zero, err
	}
	release, err := s.readableLease(p.BackupID)
	if err != nil {
		return zero, err
	}
	defer release()
	if p.SchemaVersion != 2 || p.TargetSnapshotID == "" {
		return zero, errors.New("Plan ohne privaten Zielnachweis: neu vorbereiten")
	}
	if err = s.verifyBackupUnlocked(p.BackupID); err != nil {
		return zero, err
	}
	if err = s.verifyPlanFiles(p); err != nil {
		return zero, err
	}
	snapshot, err := s.loadPlanSnapshot(p)
	if err != nil {
		return zero, err
	}
	b, err := s.Backup(p.BackupID)
	if err != nil {
		return zero, err
	}
	files := []string{}
	for _, step := range p.Steps {
		files = append(files, step.Path)
	}
	sort.Strings(files)
	meta := RecoveryRecipe{ID: ID(), Version: 1, OriginPlanID: p.ID, Label: label, ReviewedBy: actor, ReviewedAt: now()}
	value := recoveryRecipePrivate{RecoveryRecipe: meta, OriginSnapshotID: p.TargetSnapshotID, BackupID: p.BackupID, ManifestSHA: b.ManifestSHA, Scenario: p.Scenario, Files: files, Mapping: p.Mapping, Constraints: recipeConstraints(snapshot), MissingProof: recipeProof(snapshot, p.Mapping)}
	raw, err := json.Marshal(value)
	if err != nil {
		return zero, err
	}
	if err = atomicWrite(filepath.Join(s.Root, "recovery-recipes", meta.ID+".json"), raw, 0600); err != nil {
		return zero, err
	}
	if err = s.Store.Put("recovery-recipes", meta.ID, recoveryRecipeRecord{meta, Hash(raw)}); err != nil {
		return zero, err
	}
	return meta, nil
}
func (s *Service) RecoveryRecipes() ([]RecoveryRecipe, error) {
	rows, err := records[recoveryRecipeRecord](s.Store, "recovery-recipes")
	if err != nil {
		return nil, err
	}
	out := []RecoveryRecipe{}
	for _, row := range rows {
		out = append(out, row.RecoveryRecipe)
	}
	return out, nil
}
func (s *Service) PreviewRecoveryRecipe(id string, r PlanRequest) (RecoveryRecipePreview, error) {
	out := RecoveryRecipePreview{Suggestions: Mapping{Interfaces: map[string]string{}, Storage: map[string]string{}}, Mismatches: []string{}}
	if !validID(id) {
		return out, errors.New("Ungültige Vorlagen-ID")
	}
	var rec recoveryRecipeRecord
	if err := s.Store.Get("recovery-recipes", id, &rec); err != nil {
		return out, err
	}
	raw, err := readSnapshotBytes(filepath.Join(s.Root, "recovery-recipes", id+".json"), 1<<20)
	if err != nil {
		return out, err
	}
	var value recoveryRecipePrivate
	if Hash(raw) != rec.SHA256 || json.Unmarshal(raw, &value) != nil || !reflect.DeepEqual(value.RecoveryRecipe, rec.RecoveryRecipe) || value.Version != 1 {
		return out, errors.New("Zuordnungsvorlage verändert oder Version nicht unterstützt")
	}
	out.Recipe = value.RecoveryRecipe
	if r.SnapshotID == value.OriginSnapshotID {
		out.Mismatches = append(out.Mismatches, "Neue Zielaufnahme erforderlich; ursprünglicher Plannachweis bleibt historisch")
		return out, nil
	}
	if r.SnapshotID == "" {
		out.Mismatches = append(out.Mismatches, "Frische Zielaufnahme fehlt")
		return out, nil
	}
	snapshot, err := s.LoadRecoverySnapshot(r.SnapshotID, r)
	if err != nil {
		out.Mismatches = append(out.Mismatches, "Quelle oder Zielnachweis ungültig: "+err.Error())
		return out, nil
	}
	out.SnapshotID = snapshot.ID
	if snapshot.BackupID != value.BackupID || snapshot.SourceManifestSHA != value.ManifestSHA {
		out.Mismatches = append(out.Mismatches, "Quellsicherung stimmt nicht überein")
	}
	if snapshot.Scenario != value.Scenario {
		out.Mismatches = append(out.Mismatches, "Wiederherstellungssituation stimmt nicht überein")
	}
	if r.Scenario == "files" {
		files := append([]string{}, r.Files...)
		sort.Strings(files)
		if !reflect.DeepEqual(files, value.Files) {
			out.Mismatches = append(out.Mismatches, "Dateiauswahl stimmt nicht überein")
		}
	}
	for _, missing := range value.MissingProof {
		out.Mismatches = append(out.Mismatches, "Ursprünglicher Plan: "+missing)
	}
	out.Mismatches = append(out.Mismatches, recipeProof(snapshot, value.Mapping)...)
	current := recipeConstraints(snapshot)
	for name, want := range value.Constraints {
		if want == "" || current[name] != want {
			out.Mismatches = append(out.Mismatches, name+": geändert oder nicht belegt")
		}
	}
	sort.Strings(out.Mismatches)
	if len(out.Mismatches) == 0 {
		out.Applicable = true
		out.Suggestions = value.Mapping
	}
	return out, nil
}
func (s *Service) recipeHTTP(u User, w http.ResponseWriter, r *http.Request) (bool, error) {
	path := strings.Trim(r.URL.Path, "/")
	if path != "api/recovery-recipes" && path != "api/recovery-recipes/preview" {
		return false, nil
	}
	if err := require(u, "restore"); err != nil {
		return true, err
	}
	if r.Method == "GET" && path == "api/recovery-recipes" {
		rows, err := s.RecoveryRecipes()
		if err != nil {
			return true, err
		}
		return true, jsonOut(w, redactRecovery(rows))
	}
	if r.Method != "POST" {
		return true, fail(405, "POST erforderlich")
	}
	if path == "api/recovery-recipes/preview" {
		var in struct {
			RecipeID string      `json:"recipe_id"`
			Request  PlanRequest `json:"request"`
		}
		if err := input(r, &in); err != nil {
			return true, err
		}
		out, err := s.PreviewRecoveryRecipe(in.RecipeID, in.Request)
		if err != nil {
			return true, err
		}
		return true, jsonOut(w, redactRecovery(out))
	}
	var in struct {
		PlanID   string `json:"plan_id"`
		Label    string `json:"label"`
		Reviewed bool   `json:"reviewed"`
	}
	if err := input(r, &in); err != nil {
		return true, err
	}
	out, err := s.SaveRecoveryRecipe(in.PlanID, in.Label, u.ID, in.Reviewed)
	if err != nil {
		return true, err
	}
	s.LogAudit(u.ID, "recovery-recipe.review", out.ID)
	return true, jsonOut(w, redactRecovery(out))
}
