package anker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

type ClusterSetMember struct {
	HostID          string   `json:"host_id"`
	HostName        string   `json:"host_name"`
	JobID           string   `json:"job_id,omitempty"`
	BackupID        string   `json:"backup_id,omitempty"`
	ManifestSHA     string   `json:"manifest_sha,omitempty"`
	State           string   `json:"state"`
	StartedAt       string   `json:"started_at,omitempty"`
	CompletedAt     string   `json:"completed_at,omitempty"`
	SharedSHA256    string   `json:"shared_sha256,omitempty"`
	CapturedMembers []string `json:"captured_members,omitempty"`
	MembershipKnown bool     `json:"membership_known"`
	IdentityKnown   bool     `json:"identity_known"`
	IdentityMatches bool     `json:"identity_matches"`
	SharedKnown     bool     `json:"shared_known"`
}
type ClusterBackupSet struct {
	Version     int                `json:"version"`
	ID          string             `json:"id"`
	ClusterID   string             `json:"cluster_id"`
	Purpose     string             `json:"purpose"`
	Atomic      bool               `json:"atomic"`
	CreatedAt   string             `json:"created_at"`
	StartedAt   string             `json:"started_at,omitempty"`
	CompletedAt string             `json:"completed_at,omitempty"`
	StartSkewMS int64              `json:"start_skew_ms"`
	State       string             `json:"state"`
	Members     []ClusterSetMember `json:"members"`
	Notes       []string           `json:"notes"`
}

// Sets contain references and capture evidence only. Actual material remains in
// ordinary backups and is carried by the existing cold-package inventory.
func (s *Service) QueueClusterSet(cluster string) (ClusterBackupSet, error) {
	s.clusterSetMu.Lock()
	defer s.clusterSetMu.Unlock()
	v := ClusterBackupSet{Version: 1, ID: ID(), ClusterID: cluster, Purpose: "manualSourcePlanning", CreatedAt: now(), State: "capturing", Members: []ClusterSetMember{}}
	hosts, err := s.Hosts()
	if err != nil {
		return v, err
	}
	if cluster == "" || len(cluster) > 200 {
		return v, fail(400, "Clusterkennung erforderlich")
	}
	for _, h := range hosts {
		if h.ClusterID == cluster {
			v.Members = append(v.Members, ClusterSetMember{HostID: h.ID, HostName: h.Name, State: "queued"})
		}
	}
	if len(v.Members) == 0 {
		return v, fail(400, "Keine erwarteten Hosts für diesen Cluster eingerichtet")
	}
	if err = s.Store.Put("cluster_sets", v.ID, v); err != nil {
		return v, err
	}
	for i := range v.Members {
		hostID := v.Members[i].HostID
		job, e := s.queueAtForSet(hostID, "backup", nil, v.ID, func(ctx context.Context) (string, error) {
			b, captureErr := s.CreateBackup(ctx, hostID)
			// Even a late error after publication must retain the actual result ID.
			if b.ID != "" {
				if stored, e := s.Backup(b.ID); e == nil && stored.HostID == hostID {
					if e = s.recordClusterCapture(v.ID, hostID, b.ID, captureErr == nil); e != nil {
						return b.ID, e
					}
					if captureErr == nil && b.Status == "partial" {
						captureErr = errors.New("Sicherung hat Pflichtlücken")
					}
					return b.ID, captureErr
				}
			}
			return "", captureErr
		})
		if e != nil {
			v.Members[i].State = "not_admitted"
		} else {
			v.Members[i].JobID = job.ID
		}
	}
	if err = s.Store.Put("cluster_sets", v.ID, v); err != nil {
		return v, err
	}
	return v, nil
}

func (s *Service) recordClusterCapture(id, hostID, backupID string, succeeded bool) error {
	s.clusterSetMu.Lock()
	defer s.clusterSetMu.Unlock()
	var v ClusterBackupSet
	if err := s.Store.Get("cluster_sets", id, &v); err != nil {
		return err
	}
	b, err := s.Backup(backupID)
	if err != nil {
		return err
	}
	m, err := s.Manifest(backupID)
	if err != nil {
		return err
	}
	for i := range v.Members {
		member := &v.Members[i]
		if member.HostID != hostID {
			continue
		}
		member.BackupID = b.ID
		member.ManifestSHA = b.ManifestSHA
		member.StartedAt = m.CreatedAt
		member.CompletedAt = m.CompletedAt
		member.State = "captured"
		if !succeeded || m.Status != "successful" {
			member.State = "partial"
		}
		member.IdentityKnown = m.Inventory.ClusterID != "" && m.Inventory.Hostname != ""
		member.IdentityMatches = m.Inventory.ClusterID == v.ClusterID && m.Inventory.Hostname == member.HostName
		// Read and verify logical bytes, never compare SQLite storage layout/mtime.
		logical := []struct{ Path, Type, Hash string }{}
		known := m.Artifacts["recovery/config.db"] != ""
		var membership []string
		for _, e := range m.Entries {
			if !strings.HasPrefix(e.Path, "etc/pve/") || strings.HasPrefix(e.Path, "etc/pve/nodes/") || e.Path == "etc/pve/nodes" || e.Type == "directory" {
				continue
			}
			if e.Type != "file" || len(e.SHA256) != 64 {
				known = false
				continue
			}
			p, joinErr := safeJoin(filepath.Join(s.backupDir(b), "files"), e.Path)
			if joinErr != nil {
				known = false
				continue
			}
			data, readErr := os.ReadFile(p)
			if readErr != nil || Hash(data) != e.SHA256 {
				known = false
				continue
			}
			logical = append(logical, struct{ Path, Type, Hash string }{e.Path, e.Type, e.SHA256})
			if e.Path == "etc/pve/corosync.conf" {
				membership = clusterMemberNames(string(data))
			}
		}
		sort.Slice(logical, func(i, j int) bool { return logical[i].Path < logical[j].Path })
		member.CapturedMembers = membership
		member.MembershipKnown = len(membership) > 0
		member.SharedKnown = known && len(logical) > 0 && member.MembershipKnown
		if member.SharedKnown {
			raw, _ := json.Marshal(logical)
			member.SharedSHA256 = Hash(raw)
		}
	}
	return s.Store.Put("cluster_sets", v.ID, v)
}

// Accept the regular Corosync section syntax conservatively. Unknown or
// ambiguous syntax yields missing evidence, never inferred membership.
func clusterMemberNames(config string) []string {
	stack := []string{}
	names := []string{}
	nodes := 0
	nodeNamed := false
	clean := []string{}
	for _, line := range strings.Split(config, "\n") {
		clean = append(clean, strings.SplitN(line, "#", 2)[0])
	}
	lines := strings.Split(strings.NewReplacer("{", "{\n", "}", "\n}\n").Replace(strings.Join(clean, "\n")), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		if line == "" {
			continue
		}
		if strings.HasSuffix(line, "{") {
			section := strings.TrimSpace(strings.TrimSuffix(line, "{"))
			if strings.ContainsAny(section, " \t:\"") {
				return nil
			}
			stack = append(stack, section)
			if strings.Join(stack, "/") == "nodelist/node" {
				nodes++
				nodeNamed = false
			}
			continue
		}
		if line == "}" {
			if strings.Join(stack, "/") == "nodelist/node" && !nodeNamed {
				return nil
			}
			if len(stack) == 0 {
				return nil
			}
			stack = stack[:len(stack)-1]
			continue
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			return nil
		}
		if strings.Join(stack, "/") == "nodelist/node" && strings.TrimSpace(key) == "name" {
			if nodeNamed {
				return nil
			}
			nodeNamed = true
			val = strings.TrimSpace(val)
			if !safeHost.MatchString(val) {
				return nil
			}
			names = append(names, val)
		}
	}
	if len(stack) != 0 || nodes == 0 || nodes != len(names) {
		return nil
	}
	sort.Strings(names)
	for i := 1; i < len(names); i++ {
		if names[i] == names[i-1] {
			return nil
		}
	}
	return names
}

func (s *Service) ClusterSet(id string) (ClusterBackupSet, error) {
	s.clusterSetMu.Lock()
	defer s.clusterSetMu.Unlock()
	var v ClusterBackupSet
	if !validID(id) {
		return v, fail(400, "Ungültige Sicherungsset-ID")
	}
	if err := s.Store.Get("cluster_sets", id, &v); err != nil {
		return v, err
	}
	if v.Version != 1 || v.ID != id {
		return v, errors.New("Sicherungssetformat nicht unterstützt")
	}
	v.Purpose = "manualSourcePlanning"
	v.Atomic = false
	v.State = "complete"
	v.Notes = []string{"Zeitversetzte Erfassung; kein atomarer Clustersnapshot. Bewertung gilt für das Erfassungsfenster, nicht für aktuelle Integrität oder Clusterzustand. Nur manuelle Quellplanung, keine Wiederherstellungsfreigabe."}
	expected := []string{}
	for _, m := range v.Members {
		expected = append(expected, m.HostName)
	}
	sort.Strings(expected)
	incomplete := len(expected) == 0
	divergent := false
	active := false
	hash := ""
	var first, last, end time.Time
	for i := range v.Members {
		m := &v.Members[i]
		job, e := s.Job(m.JobID)
		if e == nil && job.SourceSetID == id && job.HostID == m.HostID {
			if activeJob(job) {
				active = true
				m.State = job.State
			} else if m.BackupID == "" {
				m.State = job.State
			}
			if job.State == "interrupted" || job.State == "cancelled" || job.State == "failed" {
				incomplete = true
				m.State = job.State
			}
		}
		if m.BackupID == "" {
			incomplete = true
			continue
		}
		b, e := s.Backup(m.BackupID)
		if e != nil || b.HostID != m.HostID || b.ManifestSHA != m.ManifestSHA {
			incomplete = true
			m.State = "unavailable"
			continue
		}
		// Persisted references do not silently become evidence for altered material.
		if _, e = s.Manifest(b.ID); e != nil {
			incomplete = true
			m.State = "unavailable"
			continue
		}
		start, e1 := time.Parse(time.RFC3339Nano, m.StartedAt)
		finish, e2 := time.Parse(time.RFC3339Nano, m.CompletedAt)
		created, e3 := time.Parse(time.RFC3339Nano, v.CreatedAt)
		if e1 != nil || e2 != nil || e3 != nil || start.Before(created) || finish.Before(start) {
			incomplete = true
			continue
		}
		if first.IsZero() || start.Before(first) {
			first = start
		}
		if start.After(last) {
			last = start
		}
		if finish.After(end) {
			end = finish
		}
		if m.State != "captured" || b.Status != "successful" || !m.IdentityKnown || !m.MembershipKnown || !m.SharedKnown || m.SharedSHA256 == "" {
			incomplete = true
		}
		if (m.IdentityKnown && !m.IdentityMatches) || (m.MembershipKnown && !reflect.DeepEqual(expected, m.CapturedMembers)) {
			divergent = true
		}
		if m.SharedKnown {
			if hash != "" && hash != m.SharedSHA256 {
				divergent = true
			}
			hash = m.SharedSHA256
		}
	}
	if !first.IsZero() {
		v.StartedAt = first.Format(time.RFC3339Nano)
		v.CompletedAt = end.Format(time.RFC3339Nano)
		v.StartSkewMS = last.Sub(first).Milliseconds()
	}
	if incomplete {
		v.State = "incomplete"
		v.Notes = append(v.Notes, "Ergebnisse oder erforderliche Identitäts-, Mitgliedschafts- oder Materialnachweise fehlen.")
	}
	if divergent {
		v.State = "divergent"
		v.Notes = append(v.Notes, "Erfasste Mitgliedschaft, Identität oder gemeinsamer Inhalt weicht ab.")
	}
	if active {
		v.State = "capturing"
		v.CompletedAt = ""
	}
	return v, nil
}
func (s *Service) ClusterSets() ([]ClusterBackupSet, error) {
	raw, err := records[ClusterBackupSet](s.Store, "cluster_sets")
	if err != nil {
		return nil, err
	}
	out := []ClusterBackupSet{}
	for _, r := range raw {
		v, e := s.ClusterSet(r.ID)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}
func publicClusterSet(v ClusterBackupSet) ClusterBackupSet {
	v.Members = append([]ClusterSetMember(nil), v.Members...)
	for i := range v.Members {
		v.Members[i].SharedSHA256 = ""
	}
	return v
}
func (s *Service) clusterSetsHTTP(u User, w http.ResponseWriter, r *http.Request) (bool, error) {
	path := strings.TrimPrefix(r.URL.Path, "/api/")
	if path != "cluster-sets" && !strings.HasPrefix(path, "cluster-sets/") {
		return false, nil
	}
	if path == "cluster-sets" {
		if r.Method == "GET" {
			sets, e := s.ClusterSets()
			if e != nil {
				return true, e
			}
			for i := range sets {
				sets[i] = publicClusterSet(sets[i])
			}
			return true, jsonOut(w, sets)
		}
		if r.Method == "POST" {
			if e := require(u, "restore"); e != nil {
				return true, e
			}
			var in struct {
				ClusterID string `json:"cluster_id"`
			}
			if e := input(r, &in); e != nil {
				return true, e
			}
			v, e := s.QueueClusterSet(in.ClusterID)
			if e != nil {
				return true, e
			}
			s.LogAudit(u.ID, "cluster-set.capture", v.ID)
			return true, jsonOut(w, publicClusterSet(v))
		}
		return true, fail(405, "GET oder POST erforderlich")
	}
	parts := strings.Split(path, "/")
	if len(parts) > 3 || (len(parts) == 3 && parts[2] != "export") {
		return true, fail(404, "Nicht gefunden")
	}
	if r.Method != "GET" {
		return true, fail(405, "GET erforderlich")
	}
	v, e := s.ClusterSet(parts[1])
	if e != nil {
		return true, e
	}
	if len(parts) == 3 {
		w.Header().Set("Content-Disposition", `attachment; filename="cluster-quellsatz-`+v.ID+`.json"`)
	}
	return true, jsonOut(w, publicClusterSet(v))
}
