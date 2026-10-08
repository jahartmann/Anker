package anker

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
)

type RecoveryReader interface {
	ReadRecoveryFiles(context.Context, Host, []string, []string) (RecoveryReadResult, error)
}
type RecoveryReadFile struct {
	Path     string `json:"path"`
	State    string `json:"state"`
	SHA256   string `json:"sha256,omitempty"`
	Metadata *Entry `json:"metadata,omitempty"`
	Content  []byte `json:"-"`
	Error    string `json:"error,omitempty"`
}
type RecoveryVolumeResult struct {
	State   string   `json:"state"`
	Volumes []string `json:"volumes"`
	Error   string   `json:"error,omitempty"`
}
type RecoveryReadResult struct {
	Files          []RecoveryReadFile              `json:"files"`
	StorageVolumes map[string]RecoveryVolumeResult `json:"storage_volumes"`
}

const recoveryResponseLimit = 16 << 20

var recoveryStorageID = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

// Content is accepted only on the private host wire; ordinary JSON output omits it.
func (f *RecoveryReadFile) UnmarshalJSON(data []byte) error {
	type alias RecoveryReadFile
	var wire struct {
		alias
		Content []byte `json:"content"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*f = RecoveryReadFile(wire.alias)
	f.Content = wire.Content

	return nil
}

func validateRecoveryWire(data []byte) error {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return err
	}
	for _, key := range []string{"files", "storage_volumes"} {
		if envelope[key] == nil || string(envelope[key]) == "null" {
			return errors.New("unvollständige Hostantwort")
		}
	}

	var wire struct {
		Files []json.RawMessage `json:"files"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	for _, data := range wire.Files {
		var f RecoveryReadFile
		if err := json.Unmarshal(data, &f); err != nil {
			return err
		}
		if f.State == "read" {
			var fields map[string]json.RawMessage
			json.Unmarshal(data, &fields)
			if fields["content"] == nil || string(fields["content"]) == "null" {
				return errors.New("fehlender Dateiinhalt")
			}
			var metadata map[string]json.RawMessage
			if json.Unmarshal(fields["metadata"], &metadata) != nil {
				return errors.New("fehlende Dateimetadaten")
			}
			for _, key := range []string{"path", "type", "mode", "uid", "gid", "size", "sha256", "xattrs"} {
				if metadata[key] == nil || string(metadata[key]) == "null" {
					return errors.New("unvollständige Dateimetadaten")
				}
			}
		} else {
			var fields map[string]json.RawMessage
			json.Unmarshal(data, &fields)
			if fields["content"] != nil || fields["sha256"] != nil || fields["metadata"] != nil {
				return errors.New("Inhalt ohne erfolgreichen Lesezustand")
			}
		}
	}
	return nil
}

func validateRecoveryRequest(paths, ids []string) error {
	seen := map[string]bool{}
	for _, p := range paths {
		if err := ValidPath(p); err != nil {
			return err
		}
		if len(p) > 4096 || seen[p] {
			return errors.New("ungültige oder doppelte Leseauswahl")
		}
		seen[p] = true
	}
	seen = map[string]bool{}
	for _, id := range ids {
		if !recoveryStorageID.MatchString(id) || seen[id] {
			return errors.New("ungültige oder doppelte Storage-ID")
		}
		seen[id] = true
	}
	return nil
}
func validateRecoveryRead(result RecoveryReadResult, paths, ids []string) error {
	if len(result.Files) != len(paths) || len(result.StorageVolumes) != len(ids) {
		return errors.New("unvollständige Hostantwort")
	}
	expected := map[string]bool{}
	for _, p := range paths {
		expected[p] = true
	}
	total := 0
	for _, f := range result.Files {
		if !expected[f.Path] {
			return errors.New("unerwarteter oder doppelter Dateipfad")
		}
		delete(expected, f.Path)
		switch f.State {
		case "read":
			m := f.Metadata
			if m == nil || m.Path != f.Path || m.Type != "file" || m.Link != "" || m.Mode > 07777 || m.UID < 0 || uint64(m.UID) > 1<<32-1 || m.GID < 0 || uint64(m.GID) > 1<<32-1 || m.Size != int64(len(f.Content)) || m.SHA256 != f.SHA256 || m.XAttrs == nil || Hash(f.Content) != f.SHA256 || f.Error != "" {
				return errors.New("ungültige Dateiprüfsumme oder Metadaten")
			}

			if len(m.XAttrs) > 256 {
				return errors.New("zu viele erweiterte Metadaten")
			}
			xattrBytes := 0
			for name, value := range m.XAttrs {
				decoded, err := base64.StdEncoding.DecodeString(value)
				if err != nil || name == "" || strings.ContainsRune(name, 0) {
					return errors.New("ungültige erweiterte Metadaten")
				}
				xattrBytes += len(name) + len(decoded)
			}
			if xattrBytes > 64<<10 {
				return errors.New("erweiterte Metadaten überschreiten Limit")
			}
			total += len(f.Content)
			if len(f.Content) > 1<<20 || total > 8<<20 {
				return errors.New("Dateiinhalte überschreiten Leselimit")
			}
		case "missing", "unreadable", "unsupported", "too_large", "changed", "limit":
			if f.Content != nil || f.SHA256 != "" || f.Metadata != nil {
				return errors.New("Inhalt ohne erfolgreichen Lesezustand")
			}
		default:
			return errors.New("unbekannter Lesezustand")
		}
	}
	for _, id := range ids {
		v, ok := result.StorageVolumes[id]
		if !ok {
			return errors.New("fehlendes Storageergebnis")
		}
		switch v.State {
		case "ok":
			if v.Volumes == nil || v.Error != "" {
				return errors.New("unvollständiger Volumenbestand")
			}
			seen := map[string]bool{}
			for _, volume := range v.Volumes {
				if !strings.HasPrefix(volume, id+":") || len(volume) <= len(id)+1 || strings.ContainsAny(volume, " \t\r\n\x00") || seen[volume] {
					return errors.New("ungültiger Volumenbestand")
				}
				seen[volume] = true
			}
		case "unknown", "failed", "limit":
			if len(v.Volumes) > 0 {
				return errors.New("unvollständiger Volumenbestand als Erfolg")
			}
		default:
			return errors.New("unbekannter Storagezustand")
		}
	}
	return nil
}

// Reject duplicate object members too, including duplicate storage-result IDs.
func uniqueJSONValue(d *json.Decoder) error {
	token, err := d.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		keys := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || keys[name] {
				return errors.New("doppeltes JSON-Feld")
			}
			keys[name] = true
			if err = uniqueJSONValue(d); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err = uniqueJSONValue(d); err != nil {
				return err
			}
		}
	default:
		return errors.New("ungültiges JSON")
	}
	_, err = d.Token()
	return err
}
func decodeRecoveryRead(r io.Reader, paths, ids []string) (RecoveryReadResult, error) {
	data, err := io.ReadAll(io.LimitReader(r, recoveryResponseLimit+1))
	if err != nil {
		return RecoveryReadResult{}, err
	}
	if len(data) > recoveryResponseLimit {
		return RecoveryReadResult{}, errors.New("Hostantwort überschreitet 16 MiB")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	if err = uniqueJSONValue(d); err != nil {
		return RecoveryReadResult{}, err
	}
	if _, err = d.Token(); err != io.EOF {
		return RecoveryReadResult{}, errors.New("zusätzliche Hostantwort")
	}
	if err = validateRecoveryWire(data); err != nil {
		return RecoveryReadResult{}, err
	}
	var result RecoveryReadResult
	if err = json.Unmarshal(data, &result); err == nil {
		err = validateRecoveryRead(result, paths, ids)
	}
	if err != nil {
		return RecoveryReadResult{}, err
	}
	return result, nil
}
func (c SSHCollector) ReadRecoveryFiles(ctx context.Context, h Host, paths, ids []string) (RecoveryReadResult, error) {
	if err := validateRecoveryRequest(paths, ids); err != nil {
		return RecoveryReadResult{}, err
	}
	if paths == nil {
		paths = []string{}
	}
	if ids == nil {
		ids = []string{}
	}
	var result RecoveryReadResult
	err := sshRun(ctx, h, map[string]any{"version": 1, "operation": "read-recovery", "paths": paths, "storage_ids": ids}, func(r io.Reader) error { var err error; result, err = decodeRecoveryRead(r, paths, ids); return err })
	if err != nil {
		return RecoveryReadResult{}, err
	}
	return result, nil
}

// Make all API-visible inventory envelopes safe, including legacy saved plans.
// Work on a copy so hashing/internal readers retain their original input.
func sanitizeRecoveryInventory(i Inventory) Inventory {
	details := make(map[string]json.RawMessage, len(i.Details))
	for k, v := range i.Details {
		if k != "storage_config" {
			details[k] = v
		}
	}
	if raw := i.Details["storage_config"]; raw != nil {
		var config string
		if json.Unmarshal(raw, &config) == nil {
			details["storage_definitions"], _ = json.Marshal(storageDefinitions(config))
		}
	}
	i.Details = details
	return i
}
func (i Inventory) MarshalJSON() ([]byte, error) {
	type alias Inventory
	return json.Marshal(alias(sanitizeRecoveryInventory(i)))
}

func (f RecoveryReadFile) MarshalJSON() ([]byte, error) {
	type alias RecoveryReadFile
	type metadata struct {
		*Entry
		XAttrs map[string]string `json:"xattrs"`
	}
	var m *metadata
	if f.Metadata != nil {
		m = &metadata{f.Metadata, f.Metadata.XAttrs}
	}
	return json.Marshal(struct {
		alias
		Metadata *metadata `json:"metadata,omitempty"`
	}{alias(f), m})
}
