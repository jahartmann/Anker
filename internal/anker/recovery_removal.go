package anker

import (
	"encoding/json"
	"strings"
)

func containsPath(paths []string, path string) bool {
	for _, p := range paths {
		if p == path {
			return true
		}
	}
	return false
}

// Only readable ordinary files may be deleted. Target bytes and write metadata
// are independently captured; absence in a source is never deletion authority.
func removalReason(path string, before RecoveryReadFile, target Inventory) string {
	if protectedPath(path) || isSecret(path) || before.State != "read" || before.Metadata == nil || before.Metadata.Type != "file" || before.Metadata.Secret || secretContent(before.Content) || len(before.Metadata.XAttrs) > 0 || before.SHA256 != Hash(before.Content) || len(before.Content) > 1<<20 {
		return "Datei kann nicht sicher entfernt werden; gewöhnlichen, vollständig lesbaren Zielinhalt und Eigenschaften prüfen"
	}
	if failures := requiredCommandFailures(target, []string{"pve_version", "file_inventory", "users", "groups"}, true); len(failures) > 0 {
		return failures[0]
	}
	var caps struct {
		RemoveFiles bool `json:"remove_files"`
	}
	json.Unmarshal(target.Details["capabilities"], &caps)
	if !caps.RemoveFiles {
		return "Hosthelfer für sichere Dateientfernungen aktualisieren"
	}
	e := *before.Metadata
	e.Path = path
	if reason := fileManualReason(e, target, target); reason != "" {
		return reason
	}
	// Keep deletion away from executable payloads and helper installation material.
	if strings.HasPrefix(path, "usr/local/") {
		return "Programme und Hosthelfer ausschließlich manuell entfernen"
	}
	return ""
}

func hasRemovalDecision(r PlanRequest) bool {
	for _, path := range r.RemoveFiles {
		if r.Decisions[path] == DecisionRemove {
			return true
		}
	}
	return false
}
