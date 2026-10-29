package anker

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// Older helpers reject systemd's literal backslash escapes while walking files,
// but their independently collected inventory includes the symlink metadata.
// Recover only that exact, corroborated link in a NEW capture. Never read or
// follow its destination and never rewrite an existing backup.
func reconcileSystemdSymlinks(stage string, collection *Collection) error {
	var metadata map[string]json.RawMessage
	var hashes map[string]string
	if json.Unmarshal(collection.Inventory.Details["file_metadata"], &metadata) != nil || json.Unmarshal(collection.Inventory.Details["file_hashes"], &hashes) != nil {
		return nil
	}
	seen := map[string]Entry{}
	for _, entry := range collection.Entries {
		seen[entry.Path] = entry
	}
	warnings := make([]string, 0, len(collection.Warnings))
	for _, warning := range collection.Warnings {
		keep := func() { warnings = append(warnings, warning) }
		if !strings.HasPrefix(warning, "Cannot capture /") {
			keep()
			continue
		}
		fields := strings.SplitN(strings.TrimPrefix(warning, "Cannot capture /"), ": ", 2)
		if len(fields) != 2 || (fields[1] != "invalid path" && fields[1] != "invalid path: backslash is not supported") {
			keep()
			continue
		}
		name := fields[0]
		if !systemdEscapedPath(name) || ValidPath(name) != nil {
			keep()
			continue
		}
		if _, exists := seen[name]; exists {
			keep()
			continue
		}
		if parent, ok := seen[path.Dir(name)]; !ok || parent.Type != "directory" {
			keep()
			continue
		}
		var record struct {
			Type            string            `json:"type"`
			Mode            *uint32           `json:"mode"`
			UID             *int              `json:"uid"`
			GID             *int              `json:"gid"`
			XAttrs          map[string]string `json:"xattrs"`
			Link            string            `json:"link"`
			MTime           *int64            `json:"mtime"`
			MetadataWarning string            `json:"metadata_warning"`
		}
		if json.Unmarshal(metadata[name], &record) != nil || record.Type != "symlink" || record.Mode == nil || *record.Mode > 07777 || record.UID == nil || *record.UID < 0 || record.GID == nil || *record.GID < 0 || record.XAttrs == nil || record.MetadataWarning != "" || record.Link == "" || len(record.Link) > 4096 || strings.ContainsRune(record.Link, 0) || hashes[name] != "symlink:"+record.Link {
			keep()
			continue
		}
		validAttributes := true
		for key, value := range record.XAttrs {
			if key == "" || strings.ContainsRune(key, 0) {
				validAttributes = false
				break
			}
			if _, err := base64.StdEncoding.DecodeString(value); err != nil {
				validAttributes = false
				break
			}
		}
		if !validAttributes {
			keep()
			continue
		}
		destination, err := safeJoin(filepath.Join(stage, "files"), name)
		if err != nil {
			return fmt.Errorf("systemd-Link im Sicherungsbaum: %w", err)
		}
		if err := atomicWrite(destination, []byte(record.Link), 0600); err != nil {
			return err
		}
		entry := Entry{Path: name, Type: "symlink", Mode: *record.Mode, UID: *record.UID, GID: *record.GID, Link: record.Link, XAttrs: record.XAttrs, Size: int64(len(record.Link)), MTimeUnknown: record.MTime == nil, CaptureOrigin: "inventory_symlink"}
		if record.MTime != nil {
			entry.MTime = *record.MTime
		}
		collection.Entries = append(collection.Entries, entry)
		seen[name] = entry
	}
	collection.Warnings = warnings
	return nil
}
