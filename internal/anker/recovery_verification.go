package anker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"time"
)

// RecoveryVerifier is optional; Collector implementations remain compatible.
type RecoveryVerifier interface {
	VerifyRecovery(context.Context, Host, []string, []string) (RecoveryVerificationResult, error)
}
type VerificationQuery struct {
	State string `json:"state"`
	Value string `json:"value"`
}
type RecoveryVerificationResult struct {
	Protocol       int                             `json:"protocol"`
	ObservedAt     string                          `json:"observed_at"`
	Identity       VerificationQuery               `json:"identity"`
	Boot           VerificationQuery               `json:"boot"`
	Services       map[string]VerificationQuery    `json:"services"`
	StorageVolumes map[string]RecoveryVolumeResult `json:"storage_volumes"`
}
type RecoveryVerificationAttempt struct {
	ID                 string                      `json:"id"`
	PlanID             string                      `json:"plan_id"`
	BackupID           string                      `json:"backup_id"`
	TargetID           string                      `json:"target_id"`
	StartedAt          string                      `json:"started_at"`
	FinishedAt         string                      `json:"finished_at"`
	Actor              string                      `json:"actor"`
	Origin             string                      `json:"origin"`
	State              string                      `json:"state"`
	Reason             string                      `json:"reason"`
	SourceManifestSHA  string                      `json:"source_manifest_sha"`
	SnapshotSHA        string                      `json:"snapshot_sha"`
	ConnectionSHA      string                      `json:"connection_sha"`
	BaselineIdentity   string                      `json:"baseline_identity"`
	BaselineBootID     string                      `json:"baseline_boot_id"`
	BaselineCapturedAt string                      `json:"baseline_captured_at"`
	IdentityState      string                      `json:"identity_state"`
	RebootState        string                      `json:"reboot_state"`
	ServicesState      string                      `json:"services_state"`
	VolumesState       string                      `json:"volumes_state"`
	Result             *RecoveryVerificationResult `json:"result,omitempty"`
}

const verificationResponseLimit = 256 << 10

var verificationServices = []string{"pve-cluster", "pvedaemon", "pveproxy"}
var identityHash = regexp.MustCompile(`^[0-9a-f]{64}$`)
var bootIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func validateVerificationSelection(services, ids []string) error {
	if len(services) > 3 || len(ids) > 32 {
		return errors.New("Prüfauswahl überschreitet Limit")
	}
	seen := map[string]bool{}
	for _, v := range services {
		if (v != "pve-cluster" && v != "pvedaemon" && v != "pveproxy" && v != "proxmox-backup" && v != "proxmox-backup-proxy") || seen[v] {
			return errors.New("ungültiger Prüfdienst")
		}
		seen[v] = true
	}
	return validateRecoveryRequest(nil, ids)
}
func validateVerificationResult(v RecoveryVerificationResult, services, ids []string) error {
	raw, _ := json.Marshal(v)
	if len(raw) > verificationResponseLimit {
		return errors.New("Prüfantwort überschreitet Limit")
	}
	if v.Protocol != 1 || v.Services == nil || v.StorageVolumes == nil || len(v.Services) != len(services) {
		return errors.New("unvollständige Prüfantwort")
	}
	if _, err := time.Parse(time.RFC3339Nano, v.ObservedAt); err != nil {
		return errors.New("ungültiger Prüfzeitpunkt")
	}
	for _, q := range []VerificationQuery{v.Identity, v.Boot} {
		if q.State != "ok" && q.State != "unknown" && q.State != "failed" {
			return errors.New("ungültiger Abfragezustand")
		}
		if q.State != "ok" && q.Value != "" {
			return errors.New("Wert ohne erfolgreiche Abfrage")
		}
	}
	if v.Identity.State == "ok" && !identityHash.MatchString(v.Identity.Value) || v.Boot.State == "ok" && !bootIDPattern.MatchString(v.Boot.Value) {
		return errors.New("ungültige Maschinen- oder Boot-ID")
	}
	for _, name := range services {
		q, ok := v.Services[name]
		if !ok || (q.State != "ok" && q.State != "failed" && q.State != "unknown") || (q.State == "ok" && q.Value != "active") || (q.State != "ok" && q.Value != "") {
			return errors.New("ungültiges Dienstergebnis")
		}
	}
	if err := validateRecoveryRead(RecoveryReadResult{Files: []RecoveryReadFile{}, StorageVolumes: v.StorageVolumes}, nil, ids); err != nil {
		return err
	}
	for _, v := range v.StorageVolumes {
		if len(v.Volumes) > 10000 || len(v.Error) > 256 {
			return errors.New("Volumenergebnis überschreitet Limit")
		}
		for _, name := range v.Volumes {
			if len(name) > 256 {
				return errors.New("Volumenname überschreitet Limit")
			}
		}
	}
	return nil
}

// encoding/json matches struct field names case-insensitively. Validate exact
// protocol names before typed decoding, so aliases cannot override a canonical
// negative observation. All supplied fields must be non-null; optional volume
// lists/errors may be omitted on failed queries, as in the helper wire format.
func verificationWireObject(data json.RawMessage, required []string, optional ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return nil, errors.New("Prüfobjekt fehlt oder ist ungültig")
	}
	allowed := map[string]bool{}
	for _, key := range required {
		allowed[key] = true
		if fields[key] == nil {
			return nil, errors.New("fehlendes Prüffeld")
		}
	}
	for _, key := range optional {
		allowed[key] = true
	}
	for key, value := range fields {
		if !allowed[key] || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, errors.New("unbekanntes oder leeres Prüffeld")
		}
	}
	return fields, nil
}
func validateVerificationWireFields(data []byte) error {
	fields, err := verificationWireObject(data, []string{"protocol", "observed_at", "identity", "boot", "services", "storage_volumes"})
	if err != nil {
		return err
	}
	for _, key := range []string{"identity", "boot"} {
		if _, err = verificationWireObject(fields[key], []string{"state", "value"}); err != nil {
			return err
		}
	}
	for _, group := range []string{"services", "storage_volumes"} {
		var entries map[string]json.RawMessage
		if err = json.Unmarshal(fields[group], &entries); err != nil || entries == nil {
			return errors.New("Prüfergebnisliste fehlt oder ist ungültig")
		}
		for _, entry := range entries {
			if group == "services" {
				_, err = verificationWireObject(entry, []string{"state", "value"})
			} else {
				_, err = verificationWireObject(entry, []string{"state"}, "volumes", "error")
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}
func decodeVerification(r io.Reader, services, ids []string) (RecoveryVerificationResult, error) {
	var v RecoveryVerificationResult
	data, err := io.ReadAll(io.LimitReader(r, verificationResponseLimit+1))
	if err != nil {
		return v, err
	}
	if len(data) > verificationResponseLimit {
		return v, errors.New("Prüfantwort überschreitet Limit")
	}
	if !json.Valid(data) {
		return v, errors.New("ungültige Prüfantwort")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	if err = uniqueJSONValue(d); err != nil {
		return v, err
	}
	if _, err = d.Token(); err != io.EOF {
		return v, errors.New("zusätzliche Prüfantwort")
	}
	if err = validateVerificationWireFields(data); err != nil {
		return v, err
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err = d.Decode(&v); err != nil {
		return v, err
	}

	return v, validateVerificationResult(v, services, ids)
}
func (c SSHCollector) VerifyRecovery(ctx context.Context, h Host, services, ids []string) (RecoveryVerificationResult, error) {
	var v RecoveryVerificationResult
	if err := validateVerificationSelection(services, ids); err != nil {
		return v, err
	}
	err := sshRun(ctx, h, map[string]any{"version": 1, "operation": "verify-recovery", "services": services, "storage_ids": ids}, func(r io.Reader) error { var err error; v, err = decodeVerification(r, services, ids); return err })
	return v, err
}
func verificationBaseline(i Inventory) (string, string) {
	var results map[string]InventoryQueryState
	json.Unmarshal(i.Details["command_results"], &results)
	var identity, boot string
	json.Unmarshal(i.Details["machine_identity_sha256"], &identity)
	json.Unmarshal(i.Details["boot_id"], &boot)
	if results["machine_identity"].State != "ok" || !identityHash.MatchString(identity) {
		identity = ""
	}
	if results["boot_id"].State != "ok" || !bootIDPattern.MatchString(boot) {
		boot = ""
	}
	return identity, boot
}

// Attempts are stored separately from mutable human/operation state. Integrity
// failures can therefore be retained without repairing a corrupt private plan.
func (s *Service) VerifyRecovery(ctx context.Context, id string) (Plan, error) {
	lock := s.planLock(id)
	lock.Lock()
	defer lock.Unlock()
	p, err := s.Plan(id)
	if err != nil {
		return p, err
	}
	if len(p.VerificationAttempts) >= 100 {
		return p, errors.New("100 Prüfversuche erreicht; Historie bleibt erhalten")
	}
	a := RecoveryVerificationAttempt{ID: ID(), PlanID: p.ID, BackupID: p.BackupID, TargetID: p.TargetID, StartedAt: now(), Origin: "machine-read", State: "unknown", IdentityState: "unknown", RebootState: "unknown", ServicesState: "unknown", VolumesState: "unknown", SnapshotSHA: p.TargetSnapshotSHA}
	a.Actor, _ = ctx.Value(recoveryActorKey{}).(string)
	provisional := a
	provisional.State = "interrupted"
	provisional.Reason = "Prüfversuch gestartet; Abschluss noch nicht nachgewiesen"
	pending := append(append([]RecoveryVerificationAttempt{}, p.VerificationAttempts...), provisional)
	if err = s.Store.Put("recovery-verifications", id, pending); err != nil {
		return p, err
	}

	run := func() error {
		if err := s.disasterPlanAllowed(id); err != nil {
			return err
		}
		if p.SchemaVersion != 2 || p.TargetSnapshotID == "" {
			a.Reason = "Alter Plan oder fehlende Zielaufnahme; neuer Plan erforderlich"
			return nil
		}
		release, err := s.readableLease(p.BackupID)
		if err != nil {
			return err
		}
		defer release()
		if err = s.acquire(p.TargetID); err != nil {
			return err
		}
		defer s.release(p.TargetID)
		check := func() (RecoverySnapshot, error) {
			if e := s.checkBackupUnlocked(p.BackupID); e != nil {
				return RecoverySnapshot{}, e
			}
			if e := s.verifyPlanFiles(p); e != nil {
				return RecoverySnapshot{}, e
			}
			snap, e := s.loadPlanSnapshot(p)
			if e != nil {
				return snap, e
			}
			h, e := s.Host(p.TargetID)
			if e != nil {
				return snap, e
			}
			sha, e := s.recoveryConnectionSHA(h)
			if e != nil {
				return snap, e
			}
			if sha != snap.ConnectionSHA {
				return snap, errors.New("Zielverbindung oder SSH-Vertrauen geändert")
			}
			return snap, nil
		}
		snap, err := check()
		if err != nil {
			return err
		}
		b, err := s.Backup(p.BackupID)
		if err != nil {
			return err
		}
		if snap.SourceManifestSHA == "" || snap.SourceManifestSHA != b.ManifestSHA {
			return errors.New("Quellbindung der Zielaufnahme fehlt oder wurde geändert")
		}
		a.SourceManifestSHA = b.ManifestSHA
		a.ConnectionSHA = snap.ConnectionSHA
		a.BaselineCapturedAt = snap.CapturedAt
		a.BaselineIdentity, a.BaselineBootID = verificationBaseline(snap.Target)
		c, ok := s.Collector.(RecoveryVerifier)
		if !ok {
			a.State = "unsupported"
			a.Reason = "Hostzugang unterstützt keine Maschinenprüfung"
			return nil
		}
		var caps struct {
			Protocol int `json:"recovery_verify_protocol"`
		}
		json.Unmarshal(snap.Target.Details["capabilities"], &caps)
		if caps.Protocol != 1 {
			a.State = "unsupported"
			a.Reason = "Aufgenommener Helfer unterstützt Prüfprotokoll 1 nicht; neue Zielaufnahme erforderlich"
			return nil
		}
		ids := []string{}
		for id := range snap.StorageVolumes {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		services := verificationServices
		if p.Source.Platform() == "pbs" {
			services = []string{"proxmox-backup", "proxmox-backup-proxy"}
			ids = []string{}
		}
		if err = validateVerificationSelection(services, ids); err != nil {
			return err
		}
		h, err := s.Host(p.TargetID)
		if err != nil {
			return err
		}
		result, readErr := c.VerifyRecovery(ctx, h, append([]string{}, services...), ids)
		if _, err = check(); err != nil {
			return err
		}
		if readErr != nil {
			a.State = "failed"
			a.Reason = "SSH-Prüfung fehlgeschlagen, unterbrochen oder Antwort ungültig"
			return nil
		}
		if err = validateVerificationResult(result, services, ids); err != nil {
			return err
		}
		observed, _ := time.Parse(time.RFC3339Nano, result.ObservedAt)
		started, _ := time.Parse(time.RFC3339Nano, a.StartedAt)
		if observed.Before(started.Add(-5*time.Minute)) || observed.After(time.Now().Add(5*time.Minute)) {
			return errors.New("Prüfzeitpunkt außerhalb frischer Abfrage")
		}
		a.Result = &result
		if a.BaselineIdentity != "" && result.Identity.State == "ok" {
			a.IdentityState = "failed"
			if a.BaselineIdentity == result.Identity.Value {
				a.IdentityState = "passed"
			}
		}
		if a.IdentityState == "passed" && a.BaselineBootID != "" && result.Boot.State == "ok" {
			a.RebootState = "unchanged"
			if a.BaselineBootID != result.Boot.Value {
				a.RebootState = "passed"
			}
		}
		if a.IdentityState == "passed" {
			a.ServicesState = "passed"
			for _, q := range result.Services {
				if q.State == "failed" {
					a.ServicesState = "failed"
				} else if q.State != "ok" && a.ServicesState != "failed" {
					a.ServicesState = "unknown"
				}
			}
			if len(ids) > 0 {
				a.VolumesState = "passed"
				for _, id := range ids {
					current := result.StorageVolumes[id]
					baseline := snap.StorageVolumes[id]
					if current.State == "failed" {
						a.VolumesState = "failed"
						continue
					}
					if current.State != "ok" || baseline.State != "ok" {
						if a.VolumesState != "failed" {
							a.VolumesState = "unknown"
						}
						continue
					}
					found := map[string]bool{}
					for _, v := range current.Volumes {
						found[v] = true
					}
					for _, v := range baseline.Volumes {
						if !found[v] {
							a.VolumesState = "failed"
						}
					}
				}
			}
		}
		a.State = "observed"
		if a.IdentityState == "failed" || a.ServicesState == "failed" || a.VolumesState == "failed" {
			a.State = "failed"
		}
		a.Reason = "Nur beobachteter Zustand; keine automatische Aktivierung oder Betriebsfreigabe"
		return nil
	}
	if err = run(); err != nil {
		a.State = "failed"
		a.Reason = "Plan-, Quellen-, Verbindungs- oder Prüfantwortintegrität nicht bestätigt"
		a.Result = nil
	}
	a.FinishedAt = now()
	p.VerificationAttempts = append(p.VerificationAttempts, a)
	if err = s.Store.Put("recovery-verifications", id, p.VerificationAttempts); err != nil {
		return p, err
	}
	return p, nil
}
func verificationGuide(p Plan) string {
	out := "\n## Maschinelle Prüfversuche (getrennt von menschlichen Angaben)\n\n"
	if len(p.VerificationAttempts) == 0 {
		return out + "Keine frische maschinelle Prüfung vorhanden.\n"
	}
	for _, a := range p.VerificationAttempts {
		out += fmt.Sprintf("- Versuch %s · %s → %s · Ursprung %s · Akteur %s · Plan %s · Sicherung %s · Ziel %s\n  Zustand: %s; Identität: %s; Neustart: %s; Dienste: %s; Volumes: %s. %s\n  Quelle SHA256 %s · Zielaufnahme SHA256 %s · Verbindung SHA256 %s\n  Baseline %s · Maschinenhash %s · Boot-ID %s\n", a.ID, a.StartedAt, a.FinishedAt, a.Origin, evidenceLabel(a.Actor), a.PlanID, a.BackupID, a.TargetID, a.State, a.IdentityState, a.RebootState, a.ServicesState, a.VolumesState, a.Reason, a.SourceManifestSHA, a.SnapshotSHA, a.ConnectionSHA, a.BaselineCapturedAt, a.BaselineIdentity, a.BaselineBootID)
		if a.Result != nil {
			raw, _ := json.Marshal(a.Result)
			out += "\n```json\n" + string(raw) + "\n```\n"
		}
	}
	return out
}
