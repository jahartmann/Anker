package anker

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var publicCredential = regexp.MustCompile(`(?i)\b(password|passwd|passphrase|psk|secret|token|apikey|api_key|privatekey|private_key)\b\s*(?:[=:]|\s)\s*\S+`)

var publicCredentialURL = regexp.MustCompile(`(?i)[a-z][a-z0-9+.-]*://[^/\s@]+:[^/\s@]+@`)

func publicSecretText(text string) bool {
	return secretContent([]byte(text)) || publicCredential.MatchString(text) || publicCredentialURL.MatchString(text)
}

// Public recovery metadata never carries raw inventory details. Recursive string
// filtering also covers older persisted effects/tasks and remote diagnostic text.
func redactRecovery[T any](value T) T {
	raw, _ := json.Marshal(value)
	var tree any
	json.Unmarshal(raw, &tree)
	tree = scrubRecoveryValue(tree, "")
	raw, _ = json.Marshal(tree)
	var result T
	json.Unmarshal(raw, &result)
	return result
}
func scrubRecoveryValue(v any, key string) any {
	switch x := v.(type) {
	case map[string]any:
		// Effects can contain a secret value without a credential assignment prefix.
		if field, ok := x["field"].(string); ok && publicSecretText(field+"=value") {
			x["before"] = "[Verdeckter Inhalt]"
			x["after"] = "[Verdeckter Inhalt]"
		}
		if path, ok := x["path"].(string); ok && isSecret(path) {
			for _, k := range []string{"before", "after", "link", "xattrs", "diff"} {
				if _, exists := x[k]; exists {
					x[k] = nil
				}
			}
		}
		if attrs, ok := x["xattrs"].(map[string]any); ok {
			encoded := map[string]string{}
			invalid := false
			for name, value := range attrs {
				text, ok := value.(string)
				if !ok {
					invalid = true
					break
				}
				encoded[name] = text
			}
			if invalid || comparisonXAttrsSecret(encoded) {
				x["xattrs"] = nil
			}
		}
		for k, y := range x {
			_, rawContent := y.(string)
			if k == "details" || (k == "content" && rawContent) {
				delete(x, k)
				continue
			}
			x[k] = scrubRecoveryValue(y, k)
		}
		return x
	case []any:
		for i := range x {
			x[i] = scrubRecoveryValue(x[i], key)
		}
		return x
	case string:
		if publicSecretText(x) {
			return "[Verdeckter Inhalt]"
		}
		return x
	default:
		return v
	}
}
func publicPlan(p Plan) Plan {
	p = redactRecovery(p)
	for i := range p.Steps {
		// Legacy diffs are raw content, not list metadata. New comparisons load verified
		// private evidence on demand and enforce the explicit reveal gate there.
		p.Steps[i].Diff = ""
		if p.Steps[i].Secret || isSecret(p.Steps[i].Path) {
			for j := range p.Steps[i].Effects {
				p.Steps[i].Effects[j].Before = "[Verdeckter Inhalt]"
				p.Steps[i].Effects[j].After = "[Verdeckter Inhalt]"
			}
		}
	}
	return p
}

type comparisonSide struct {
	data  []byte
	entry *Entry
	state string
}

func (s *Service) backupComparisonSide(id, path string) (comparisonSide, error) {
	m, err := s.Manifest(id)
	if err != nil {
		return comparisonSide{}, err
	}
	for _, e := range m.Entries {
		if e.Path != path {
			continue
		}
		side := comparisonSide{entry: &e, state: "read"}
		if e.Size > comparisonBytes {
			side.state = "too_large"
			return side, nil
		}
		if e.Type == "directory" {
			return side, nil
		}
		b, err := s.Backup(id)
		if err != nil {
			return side, err
		}
		full, err := safeJoin(filepath.Join(s.backupDir(b), "files"), path)
		if err != nil {
			return side, err
		}
		side.data, err = readSnapshotBytes(full, comparisonBytes)
		if err != nil || Hash(side.data) != e.SHA256 {
			side.data = nil
			side.state = "unavailable"
		}
		return side, nil
	}
	return comparisonSide{state: "missing"}, nil
}
func compareSides(a, b comparisonSide, al, bl, path string, reveal bool) FileComparison {
	c := compareFilesVisible(a.data, b.data, a.entry, b.entry, al, bl, reveal)
	c.Path = path
	if c.Masked {
		return c
	}
	for _, side := range []comparisonSide{a, b} {
		if side.state != "read" && side.state != "missing" {
			c.Hunks = []DiffHunk{}
			c.MetadataChanges = []MetadataChange{}
			c.ContentChanged = false
			c.State = "unavailable"
			c.Error = "Datei konnte nicht sicher gelesen werden"
			if side.state == "too_large" {
				c.State = "too_large"
				c.Error = "Vorschau zu groß; vollständigen Download verwenden"
			}
			return c
		}
	}
	return c
}
func (s *Service) CompareBackupFile(from, to, path string, reveal bool) (FileComparison, error) {
	if path == "" {
		return FileComparison{}, fail(400, "Dateipfad erforderlich")
	}
	if !validID(from) || !validID(to) {
		return FileComparison{}, fail(400, "Ungültige Sicherungs-ID")
	}
	first, second := from, to
	if second < first {
		first, second = second, first
	}
	release, err := s.readableLease(first)
	if err != nil {
		return FileComparison{}, err
	}
	defer release()
	if to != from {
		release, err = s.readableLease(second)
		if err != nil {
			return FileComparison{}, err
		}
		defer release()
	}
	a, err := s.backupComparisonSide(from, path)
	if err != nil {
		return FileComparison{}, err
	}
	b, err := s.backupComparisonSide(to, path)
	if err != nil {
		return FileComparison{}, err
	}
	if a.entry == nil && b.entry == nil {
		return FileComparison{}, fail(404, "Datei nicht gefunden")
	}
	return compareSides(a, b, "Sicherung "+from, "Sicherung "+to, path, reveal), nil
}
func (s *Service) ComparePlanFile(id, path, basis string, reveal bool) (FileComparison, error) {
	if basis == "" {
		basis = "target"
	}
	if basis != "target" && basis != "source" && basis != "source-target" {
		return FileComparison{}, fail(400, "Ungültige Vergleichsbasis")
	}
	if !validID(id) {
		return FileComparison{}, fail(400, "Ungültige Plan-ID")
	}
	lock := s.planLock(id)
	lock.RLock()
	defer lock.RUnlock()
	p, err := s.Plan(id)
	if err != nil {
		return FileComparison{}, fail(404, "Plan nicht gefunden")
	}
	var step *Step
	for _, v := range p.Steps {
		if v.Path == path {
			step = &v
			break
		}
	}
	if step == nil {
		return FileComparison{}, fail(404, "Datei nicht im Plan")
	}
	release, err := s.readableLease(p.BackupID)
	if err != nil {
		return FileComparison{}, err
	}
	defer release()
	source, err := s.backupComparisonSide(p.BackupID, path)
	if err != nil {
		return FileComparison{}, err
	}
	target := comparisonSide{state: "unavailable"}
	if p.SchemaVersion == 2 {
		snapshot, err := s.loadPlanSnapshot(p)
		if err != nil {
			return FileComparison{}, errors.New("Zielnachweis konnte nicht sicher gelesen werden")
		}
		for _, f := range snapshot.Files {
			if f.Path == path {
				target = comparisonSide{data: f.Content, entry: f.Metadata, state: f.State}
				break
			}
		}
	}
	// The gate covers source OR target even when the chosen basis hides one side.
	masked := !reveal && (step.Secret || comparisonSecret(source.data, source.entry) || comparisonSecret(target.data, target.entry))
	prepared := comparisonSide{state: "unavailable"}
	if step.PreparedSHA != "" && source.entry != nil {
		e := *source.entry
		prepared.entry = &e
		full, joinErr := safeJoin(filepath.Join(s.Root, "plans", p.ID, "prepared-files"), path)
		if joinErr != nil {
			return FileComparison{}, joinErr
		}
		if st, statErr := os.Lstat(full); statErr == nil && st.Mode().IsRegular() && st.Size() > comparisonBytes {
			prepared.state = "too_large"
		}
		var data []byte
		var readErr error
		if prepared.state != "too_large" {
			data, readErr = readSnapshotBytes(full, comparisonBytes)
		} else {
			readErr = errors.New("Vorschau zu groß")
		}
		if readErr == nil && Hash(data) == step.PreparedSHA {
			prepared.data = data
			prepared.state = "read"
			e.SHA256 = step.PreparedSHA
			e.Size = int64(len(data))
		}
	} else if step.Decision == DecisionKeep {
		prepared = target
	}
	a, b, al, bl := target, prepared, "Ziel vor Übernahme", "Vorbereitet"
	if step.Decision == DecisionKeep {
		bl = "Ziel behalten"
	}
	if basis == "source" {
		a = source
		al = "Sicherung " + p.BackupID
	}
	if basis == "source-target" {
		a = source
		b = target
		al = "Sicherung " + p.BackupID
		bl = "Ziel vor Übernahme"
	}
	if masked {
		return FileComparison{Path: path, BeforeLabel: al, AfterLabel: bl, State: "masked", Masked: true, MetadataChanges: []MetadataChange{}, Hunks: []DiffHunk{}}, nil
	}
	// An unreadable side cannot establish that its hidden content is nonsecret.
	// Explicit secret-authorized reveal may still compare available selected sides.
	if !reveal {
		for _, side := range []comparisonSide{source, target} {
			if side.state != "read" && side.state != "missing" {
				return compareSides(source, target, al, bl, path, false), nil
			}
		}
	}
	return compareSides(a, b, al, bl, path, reveal), nil
}
func comparisonReveal(u User, r *http.Request, requested bool) (bool, error) {
	requested = requested || r.URL.Query().Get("reveal") == "1"
	if requested {
		if err := require(u, "secrets"); err != nil {
			return false, err
		}
	}
	return requested, nil
}
func (s *Service) recoveryHTTP(u User, w http.ResponseWriter, r *http.Request) (handled bool, resultErr error) {
	defer func() { resultErr = publicRecoveryError(resultErr) }()
	path := strings.TrimPrefix(r.URL.Path, "/api/")
	if path == "backups/compare" || path == "backups/compare/file" {
		if err := require(u, "read"); err != nil {
			return true, err
		}
		if r.Method != "POST" {
			return true, fail(405, "POST erforderlich")
		}
		var in struct {
			From   string `json:"from"`
			To     string `json:"to"`
			Path   string `json:"path"`
			Reveal bool   `json:"reveal"`
		}
		if err := input(r, &in); err != nil {
			return true, err
		}
		if path == "backups/compare" {
			v, err := s.DiffBackups(in.From, in.To, "")
			if err != nil {
				return true, errors.New("Sicherungsmetadaten konnten nicht gelesen werden")
			}
			return true, jsonOut(w, v)
		}
		reveal, err := comparisonReveal(u, r, in.Reveal)
		if err != nil {
			return true, err
		}
		v, err := s.CompareBackupFile(in.From, in.To, in.Path, reveal)
		if err != nil {
			return true, err
		}
		return true, jsonOut(w, v)
	}
	parts := strings.Split(path, "/")
	if len(parts) >= 3 && parts[0] == "plans" && (parts[2] == "comparison" || parts[2] == "checks") {
		if err := require(u, "restore"); err != nil {
			return true, err
		}
		if parts[2] == "comparison" && len(parts) == 3 {
			if r.Method != "GET" {
				return true, fail(405, "GET erforderlich")
			}
			reveal, err := comparisonReveal(u, r, false)
			if err != nil {
				return true, err
			}
			v, err := s.ComparePlanFile(parts[1], r.URL.Query().Get("path"), r.URL.Query().Get("basis"), reveal)
			if err != nil {
				return true, err
			}
			return true, jsonOut(w, v)
		}
		if parts[2] == "checks" && len(parts) == 4 {
			if r.Method != "POST" {
				return true, fail(405, "POST erforderlich")
			}
			var in struct {
				Evidence string `json:"evidence"`
			}
			if err := input(r, &in); err != nil {
				return true, err
			}
			p, err := s.ConfirmRecoveryTask(WithRecoveryActor(r.Context(), u.ID), parts[1], parts[3], u.ID, in.Evidence)
			if err != nil {
				return true, err
			}
			s.LogAudit(u.ID, "plan.check", parts[1]+"/"+parts[3])
			return true, jsonOut(w, publicPlan(p))
		}
		return true, fail(404, "Nicht gefunden")
	}
	return false, nil
}

func publicRecoveryError(err error) error {
	if err == nil || !publicSecretText(err.Error()) {
		return err
	}
	var e apiError
	if errors.As(err, &e) {
		return fail(e.code, "Geschützte Diagnose; private Nachweise prüfen")
	}
	return errors.New("Geschützte Diagnose; private Nachweise prüfen")
}
