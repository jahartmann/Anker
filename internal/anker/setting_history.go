package anker

import (
	"context"
	"net"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// These readers describe explicit saved declarations, never the running system
// or an effective value inferred from includes, defaults or file precedence.
type TrackedSetting struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Key  string `json:"key"`
	Kind string `json:"kind"`
}

var trackedSettings = []TrackedSetting{
	{"ipv4-forwarding", "IPv4-Weiterleitung", "net.ipv4.ip_forward", "sysctl"},
	{"ipv6-forwarding", "IPv6-Weiterleitung", "net.ipv6.conf.all.forwarding", "sysctl"},
	{"swappiness", "Swap-Neigung", "vm.swappiness", "sysctl"},
	{"hostname", "Node-Name", "hostname", "hostname"},
	{"dns-servers", "DNS-Server", "nameserver", "dns"},
}

type SettingObservation struct {
	OrderUncertain bool     `json:"order_uncertain,omitempty"`
	BackupID       string   `json:"backup_id"`
	CreatedAt      string   `json:"created_at"`
	State          string   `json:"state"`
	Value          string   `json:"value,omitempty"`
	Sources        []string `json:"sources"`
}
type SettingHistory struct {
	Setting      TrackedSetting       `json:"setting"`
	Settings     []TrackedSetting     `json:"settings"`
	Observations []SettingObservation `json:"observations"`
	Next         string               `json:"next,omitempty"`
	Undated      int                  `json:"undated"`
}

func (s *Service) settingHistoryHTTP(u User, w http.ResponseWriter, r *http.Request) (bool, error) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) == 4 && parts[0] == "api" && parts[1] == "backups" && parts[3] == "setting-snapshot" {
		return true, s.settingSnapshotHTTP(u, w, r, parts[2])
	}
	if len(parts) != 4 || parts[0] != "api" || parts[1] != "hosts" || parts[3] != "setting-history" {
		return false, nil
	}
	if r.Method != "GET" {
		return true, fail(405, "GET erforderlich")
	}
	if err := require(u, "read"); err != nil {
		return true, err
	}
	if !validID(parts[2]) {
		return true, fail(400, "Ungültige Node-ID")
	}
	if _, err := s.Host(parts[2]); err != nil {
		return true, fail(404, "Node nicht gefunden")
	}
	settingID := r.URL.Query().Get("setting")
	if settingID == "" {
		settingID = trackedSettings[0].ID
	}
	var setting TrackedSetting
	for _, v := range trackedSettings {
		if v.ID == settingID {
			setting = v
		}
	}
	if setting.ID == "" {
		return true, fail(400, "Einstellung wird noch nicht unterstützt")
	}
	result, err := s.settingHistory(r.Context(), parts[2], setting, r.URL.Query().Get("before"))
	if err != nil {
		return true, err
	}
	return true, jsonOut(w, result)
}

const settingHistoryPage = 20

func (s *Service) settingHistory(ctx context.Context, hostID string, setting TrackedSetting, before string) (SettingHistory, error) {
	result := SettingHistory{Setting: setting, Settings: trackedSettings, Observations: []SettingObservation{}}
	all, err := s.ListBackups(hostID)
	if err != nil {
		return result, err
	}
	dated := []Backup{}
	for _, b := range all {
		t, err := time.Parse(time.RFC3339Nano, b.CreatedAt)
		if err != nil || t.After(time.Now().Add(5*time.Minute)) {
			result.Undated++
			continue
		}
		dated = append(dated, b)
	}
	sort.Slice(dated, func(i, j int) bool {
		a, _ := time.Parse(time.RFC3339Nano, dated[i].CreatedAt)
		b, _ := time.Parse(time.RFC3339Nano, dated[j].CreatedAt)
		if a.Equal(b) {
			return dated[i].ID > dated[j].ID
		}
		return a.After(b)
	})
	start := 0
	if before != "" {
		found := false
		for i, b := range dated {
			if b.ID == before {
				start = i + 1
				found = true
				break
			}
		}
		if !found {
			return result, fail(409, "Verlauf hat sich geändert. Bitte neu laden.")
		}
	}
	end := min(start+settingHistoryPage, len(dated))
	for _, b := range dated[start:end] {
		if err := ctx.Err(); err != nil {
			return result, err
		}

		observation := s.observeSetting(b, setting)
		index := start + len(result.Observations)
		stamp, _ := time.Parse(time.RFC3339Nano, b.CreatedAt)
		for _, adjacent := range []int{index - 1, index + 1} {
			if adjacent >= 0 && adjacent < len(dated) {
				other, _ := time.Parse(time.RFC3339Nano, dated[adjacent].CreatedAt)
				if stamp.Equal(other) {
					observation.OrderUncertain = true
				}
			}
		}
		result.Observations = append(result.Observations, observation)
	}
	if end < len(dated) {
		result.Next = dated[end-1].ID
	}
	return result, nil
}
func settingPath(setting TrackedSetting, p string) bool {
	switch setting.Kind {
	case "sysctl":
		return p == "etc/sysctl.conf" || (path.Dir(p) == "etc/sysctl.d" && strings.HasSuffix(p, ".conf"))
	case "hostname":
		return p == "etc/hostname"
	case "dns":
		return p == "etc/resolv.conf"
	}
	return false
}
func (s *Service) observeSetting(b Backup, setting TrackedSetting) SettingObservation {
	out := SettingObservation{BackupID: b.ID, CreatedAt: b.CreatedAt, State: "unavailable", Sources: []string{}}
	if b.Status != "successful" || b.VerificationError != "" {
		out.State = "unreliable"
		return out
	}
	release, err := s.readableLease(b.ID)
	if err != nil {
		return out
	}
	defer release()
	// Recheck under the read lease; a verification may have completed meanwhile.
	current, err := s.Backup(b.ID)
	if err != nil {
		return out
	}
	if current.Status != "successful" || current.VerificationError != "" {
		out.State = "unreliable"
		return out
	}
	m, err := s.Manifest(b.ID)
	if err != nil {
		return out
	}
	if m.Status != "successful" {
		out.State = "unreliable"
		return out
	}
	paths := []string{}
	entries := map[string]Entry{}
	for _, e := range m.Entries {
		if settingPath(setting, e.Path) {
			if e.Type != "file" {
				return out
			}
			if e.Size > 64<<10 {
				out.State = "limited"
				return out
			}
			paths = append(paths, e.Path)
			entries[e.Path] = e
		}
	}
	if len(paths) > 24 {
		out.State = "limited"
		return out
	}
	sort.Strings(paths)
	values := []string{}
	sources := []string{}
	total := 0
	for _, p := range paths {
		entry := entries[p]
		full, err := safeJoin(filepath.Join(s.backupDir(current), "files"), p)
		if err != nil {
			return out
		}
		data, err := readSnapshotBytes(full, 64<<10)
		if err != nil || Hash(data) != entry.SHA256 {
			return out
		}
		total += len(data)
		if total > 256<<10 {
			out.State = "limited"
			return out
		}
		if comparisonSecret(data, &entry) {
			out.State = "protected"
			return out
		}
		if !utf8.Valid(data) || strings.ContainsRune(string(data), 0) {
			return out
		}
		value, state := parseTrackedSetting(setting, string(data))
		if state != "value" && state != "not_recorded" {
			out.State = state
			return out
		}
		if state == "value" {
			values = append(values, value)
			sources = append(sources, p)
		}
	}
	if len(values) == 0 {
		out.State = "not_recorded"
		return out
	}
	for _, value := range values[1:] {
		if value != values[0] {
			out.State = "ambiguous"
			return out
		}
	}
	out.State = "value"
	out.Value = values[0]
	out.Sources = sources
	return out
}

var trackedHostname = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,252}$`)

func parseTrackedSetting(setting TrackedSetting, text string) (string, string) {
	values := []string{}
	for _, line := range strings.Split(text, "\n") {
		if i := strings.IndexAny(line, "#;"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch setting.Kind {
		case "hostname":
			if !trackedHostname.MatchString(line) {
				return "", "unsupported"
			}
			values = append(values, line)
		case "sysctl":
			line = strings.TrimPrefix(line, "-")
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				if strings.HasPrefix(strings.TrimSpace(key), setting.Key) {
					return "", "unsupported"
				}
				continue
			}
			key = strings.ReplaceAll(strings.TrimSpace(key), "/", ".")
			if key != setting.Key {
				continue
			}
			value = strings.TrimSpace(value)
			if setting.ID == "swappiness" {
				n, err := strconv.Atoi(value)
				if err != nil || n < 0 || n > 200 {
					return "", "unsupported"
				}
				value = strconv.Itoa(n)
			} else if value != "0" && value != "1" {
				return "", "unsupported"
			}
			values = append(values, value)
		case "dns":
			fields := strings.Fields(line)
			if len(fields) == 0 || fields[0] != "nameserver" {
				continue
			}
			if len(fields) != 2 || net.ParseIP(fields[1]) == nil {
				return "", "unsupported"
			}
			values = append(values, net.ParseIP(fields[1]).String())
		}
	}
	if len(values) == 0 {
		return "", "not_recorded"
	}
	if setting.Kind == "dns" {
		if len(values) > 8 {
			return "", "limited"
		}
		return strings.Join(values, " · "), "value"
	}
	for _, v := range values[1:] {
		if v != values[0] {
			return "", "ambiguous"
		}
	}
	return values[0], "value"
}

// A journey reads a bounded projection for exactly the selected backup, so old
// points never inherit the newest setting-history page or another node's values.
func (s *Service) settingSnapshotHTTP(u User, w http.ResponseWriter, r *http.Request, id string) error {
	if r.Method != "GET" {
		return fail(405, "GET erforderlich")
	}
	if err := require(u, "read"); err != nil {
		return err
	}
	if !validID(id) {
		return fail(400, "Ungültige Sicherungs-ID")
	}
	b, err := s.Backup(id)
	if err != nil {
		return fail(404, "Sicherung nicht gefunden")
	}
	stamp, err := time.Parse(time.RFC3339Nano, b.CreatedAt)
	if err != nil || stamp.After(time.Now().Add(5*time.Minute)) {
		return fail(409, "Zeitpunkt der Sicherung ist nicht belastbar")
	}
	type item struct {
		Setting     TrackedSetting     `json:"setting"`
		Observation SettingObservation `json:"observation"`
	}
	result := struct {
		BackupID string `json:"backup_id"`
		Settings []item `json:"settings"`
	}{BackupID: id, Settings: []item{}}
	for _, setting := range trackedSettings {
		if err := r.Context().Err(); err != nil {
			return err
		}
		result.Settings = append(result.Settings, item{setting, s.observeSetting(b, setting)})
	}
	return jsonOut(w, result)
}
