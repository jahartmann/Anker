package anker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const pbsConfigRoot = "etc/proxmox-backup/"

var pbsRestoreFiles = strings.Fields("acl.cfg user.cfg remote.cfg node.cfg datastore.cfg sync.cfg prune.cfg verification.cfg domains.cfg notifications.cfg notifications-priv.cfg s3.cfg traffic-control.cfg media-pool.cfg tape.cfg authkey.key authkey.pub csrf.key proxy.key proxy.pem shadow.json token.shadow tfa.json")

func pbsRestorePath(path string) bool {
	for _, name := range pbsRestoreFiles {
		if path == pbsConfigRoot+name {
			return true
		}
	}
	return false
}
func pbsPair(source, target Inventory) error {
	if source.Platform() == "pbs" || target.Platform() == "pbs" {
		if source.Platform() != "pbs" || target.Platform() != "pbs" {
			return errors.New("PBS-Konfiguration benötigt eine PBS-Quelle und ein PBS-Ziel; PVE und PBS dürfen nicht gemischt werden")
		}
	}
	return nil
}
func recoveryVersionsSupported(source, target Inventory) bool {
	if pbsPair(source, target) != nil {
		return false
	}
	if source.Platform() == "pbs" {
		major := versionMajor(target.PBSVersion)
		return versionMajor(source.PBSVersion) == major && (major == "3" || major == "4")
	}
	return versionMajor(source.PVEVersion) == versionMajor(target.PVEVersion) && (versionMajor(target.PVEVersion) == "8" || versionMajor(target.PVEVersion) == "9")
}
func pbsRecoveryBlockers(target Inventory) []string {
	out := requiredCommandFailures(target, []string{"pbs_version", "pbs_services", "pbs_datastore_identity", "file_inventory", "users", "groups"}, true)
	var caps struct {
		Protocol int `json:"pbs_restore_protocol"`
	}
	if json.Unmarshal(target.Details["capabilities"], &caps) != nil || caps.Protocol != 1 {
		out = append(out, "PBS-Hosthelfer für die Konfigurationswiederherstellung aktualisieren: Verbindung erneut einrichten")
	}
	var services map[string]string
	if json.Unmarshal(target.Details["pbs_services"], &services) != nil || services["proxmox-backup"] != "inactive" || services["proxmox-backup-proxy"] != "inactive" {
		out = append(out, "Vor Zielprüfung und Übernahme auf dem PBS-Ziel proxmox-backup und proxmox-backup-proxy stoppen; Dienste bleiben bis zur manuellen Abnahme gestoppt")
	}
	var identities map[string]json.RawMessage
	if json.Unmarshal(target.Details["pbs_datastore_identity"], &identities) != nil || identities == nil {
		out = append(out, "PBS-Datastore-Mounts und vorhandene Chunk-Verzeichnisse sind nicht nachgewiesen; vorhandene Datastores separat einbinden")
	}
	for _, store := range target.PBSDatastores {
		if len(identities[store.Name]) == 0 {
			out = append(out, "PBS-Datastore-Identität fehlt: "+store.Name)
		}
	}
	return out
}

type pbsSection struct {
	Name    string
	Options map[string]string
}

var pbsSectionID = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

func parsePBSConfig(data []byte, kind string) ([]pbsSection, error) {
	if !utf8.Valid(data) {
		return nil, errors.New("PBS-Konfiguration ist kein UTF-8")
	}
	sections := []pbsSection{}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		clean := strings.TrimSpace(line)
		if clean == "" || strings.HasPrefix(clean, "#") {
			continue
		}
		if line[0] != ' ' && line[0] != '\t' {
			prefix := kind + ":"
			if !strings.HasPrefix(line, prefix) {
				return nil, errors.New("Unbekannte PBS-Abschnittssyntax")
			}
			name := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			if !pbsSectionID.MatchString(name) || seen[name] {
				return nil, errors.New("Ungültiger oder doppelter PBS-Abschnitt")
			}
			seen[name] = true
			sections = append(sections, pbsSection{Name: name, Options: map[string]string{}})
		} else {
			fields := strings.Fields(clean)
			if len(sections) == 0 || len(fields) < 2 {
				return nil, errors.New("Ungültige PBS-Option")
			}
			options := sections[len(sections)-1].Options
			key := fields[0]
			value := strings.TrimSpace(clean[len(key):])
			if prior, exists := options[key]; exists {
				if kind != "sync" || (key != "group-filter" && key != "associated-key") {
					return nil, errors.New("Doppelte PBS-Option")
				}
				value = prior + "\n" + value
			}
			options[key] = value
		}
	}
	return sections, nil
}
func renderPBSConfig(sections []pbsSection, kind string) []byte {
	var out strings.Builder
	for _, section := range sections {
		fmt.Fprintf(&out, "%s: %s\n", kind, section.Name)
		keys := []string{}
		for key := range section.Options {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			for _, value := range strings.Split(section.Options[key], "\n") {
				fmt.Fprintf(&out, "\t%s %s\n", key, value)
			}
		}
		out.WriteByte('\n')
	}
	return []byte(out.String())
}
func pbsTargetDatastoreConfig(snapshot RecoverySnapshot) []byte {
	for _, file := range snapshot.Files {
		if file.Path == pbsConfigRoot+"datastore.cfg" && file.State == "read" && Hash(file.Content) == file.SHA256 {
			return file.Content
		}
	}
	return nil
}
func pbsOperationalKind(path string) string {
	switch path {
	case pbsConfigRoot + "datastore.cfg":
		return "datastore"
	case pbsConfigRoot + "sync.cfg":
		return "sync"
	case pbsConfigRoot + "prune.cfg":
		return "prune"
	case pbsConfigRoot + "verification.cfg":
		return "verification"
	}
	return ""
}
func preparePBSConfig(path string, content []byte, snapshot RecoverySnapshot) ([]byte, error) {
	kind := pbsOperationalKind(path)
	if kind == "" {
		return append([]byte(nil), content...), nil
	}
	source, err := parsePBSConfig(content, kind)
	if err != nil {
		return nil, err
	}
	target, err := parsePBSConfig(pbsTargetDatastoreConfig(snapshot), "datastore")
	if err != nil {
		return nil, err
	}
	stores := map[string]bool{}
	for _, section := range target {
		stores[section.Name] = true
	}
	if kind == "datastore" {
		sourceByName := map[string]pbsSection{}
		for _, section := range source {
			if !stores[section.Name] {
				return nil, fmt.Errorf("Vorhandenen PBS-Datastore %s zuerst separat am Ziel einbinden; keine automatische Anlage", section.Name)
			}
			sourceByName[section.Name] = section
		}
		// Start with the target definition: backing, mount identity, maintenance mode
		// and target-only stores always survive. Only inert settings come from source.
		for index := range target {
			section := &target[index]
			if original, ok := sourceByName[section.Name]; ok {
				for key, value := range original.Options {
					if key == "comment" || key == "notify" || key == "notify-user" || key == "notification-mode" || strings.HasPrefix(key, "keep-") {
						section.Options[key] = value
					}
				}
			}
			for _, key := range []string{"gc-schedule", "prune-schedule", "verify-new", "gc-on-unmount"} {
				delete(section.Options, key)
			}
		}
		return renderPBSConfig(target, kind), nil
	}
	for index := range source {
		section := &source[index]
		if !stores[section.Options["store"]] {
			return nil, fmt.Errorf("PBS-Job %s referenziert keinen eingebundenen Ziel-Datastore", section.Name)
		}
		if kind == "sync" && section.Options["remote-store"] == "" {
			return nil, errors.New("PBS-Sync-Job benötigt remote-store")
		}
		if kind == "prune" {
			if section.Options["schedule"] == "" {
				return nil, errors.New("PBS-Prune-Job benötigt einen gespeicherten Zeitplan; der Job bleibt deaktiviert")
			}
			section.Options["disable"] = "true"
		} else {
			delete(section.Options, "schedule")
			delete(section.Options, "run-on-mount")
			delete(section.Options, "unmount-on-done")
		}
	}
	return renderPBSConfig(source, kind), nil
}
func validatePBSPrepared(path string, data []byte, snapshot RecoverySnapshot) error {
	if !pbsRestorePath(path) {
		return errors.New("Nur freigegebene PBS-Konfigurationsdateien können übernommen werden")
	}
	prepared, err := preparePBSConfig(path, data, snapshot)
	if err != nil {
		return err
	}
	if kind := pbsOperationalKind(path); kind != "" {
		candidate, err := parsePBSConfig(data, kind)
		if err != nil {
			return err
		}
		canonical := renderPBSConfig(candidate, kind)
		if !bytes.Equal(canonical, prepared) {
			return errors.New("PBS-Datastore-Anbindung erhalten und automatische Job-Auslöser deaktivieren; aufbereitete Fassung wählen")
		}
	}
	return nil
}
