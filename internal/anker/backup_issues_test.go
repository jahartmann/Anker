package anker

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBackupIssuesExplainProtectedWarningsWithoutExposingSecrets(t *testing.T) {
	warnings := []string{"Referenced config/secret is not captured: /opt/app/settings.conf (etc/app/main.conf)", "Referenced config/secret is not captured: /opt/private.key (etc/app/main.conf)", "Cannot capture /etc/app.conf: invalid path", "Required path missing: /opt/application"}
	b := redactRecovery(Backup{Warnings: warnings, Issues: backupIssues(warnings)})
	if len(b.Issues) != 4 || b.Issues[0].Code != "dependency_not_captured" || b.Issues[0].Path != "/opt/app/settings.conf" {
		t.Fatal(b.Issues)
	}
	if b.Warnings[0] != "[Verdeckter Inhalt]" {
		t.Fatal("fixture must reproduce opaque old diagnostic", b.Warnings)
	}
	if b.Issues[1].Path != "/opt/private.key" {
		t.Fatal("safe pathname should remain useful metadata", b.Issues[1])
	}
	if b.Issues[2].Code != "invalid_path" || !strings.Contains(b.Issues[2].Detail, "kein Nachweis") {
		t.Fatal("invalid path treated as missing", b.Issues[2])
	}
	if b.Issues[3].Code != "required_path_missing" || b.Issues[3].Path != "/opt/application" {
		t.Fatal(b.Issues[3])
	}
	for _, i := range b.Issues {
		if i.Title == "[Verdeckter Inhalt]" || i.Action == "[Verdeckter Inhalt]" {
			t.Fatal("cause or action hidden", i)
		}
	}
	secret := "token=very-sensitive-value"
	encoded, _ := json.Marshal(redactRecovery(Backup{Warnings: []string{"Cannot capture /opt/" + secret + ": permission denied"}, Issues: backupIssues([]string{"Cannot capture /opt/" + secret + ": permission denied"})}))
	if strings.Contains(string(encoded), "very-sensitive-value") {
		t.Fatal("secret exposed", string(encoded))
	}
}

func TestLegacyBackupIssuesAvailableBeforeOpeningFiles(t *testing.T) {
	s := testService(t)
	b := Backup{ID: ID(), HostID: "host1", Status: "partial", Warnings: []string{"Required path missing: /opt/config"}}
	if err := s.Store.Put("backups", b.ID, b); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListBackups("host1")
	if err != nil || len(list) != 1 || len(list[0].Issues) != 1 {
		t.Fatal(list, err)
	}
	loaded, err := s.Backup(b.ID)
	if err != nil || len(loaded.Issues) != 1 || loaded.Issues[0].Code != "required_path_missing" {
		t.Fatal(loaded, err)
	}
	job := Job{ID: ID(), HostID: "host1", Kind: "backup", State: "failed", Error: "invalid path"}
	if err := s.Store.Put("jobs", job.ID, job); err != nil {
		t.Fatal(err)
	}
	jobs, err := s.Jobs()
	if err != nil || len(jobs) != 1 || len(jobs[0].Issues) != 1 || jobs[0].Issues[0].Code != "invalid_path" {
		t.Fatal(jobs, err)
	}
}

func TestInvalidPathFormatDoesNotClaimFileAbsence(t *testing.T) {
	for _, tc := range []struct{ warning, part string }{
		{"invalid path: expected a relative path", "relativer Pfad"},
		{"invalid path: backslash is not supported", "Backslash"},
		{"invalid path: empty, dot or parent component is not allowed", "übergeordneten"},
		{"invalid path: empty or non-text path", "leer"},
	} {
		issues := backupIssues([]string{tc.warning})
		if len(issues) != 1 || issues[0].Code != "invalid_path" || !strings.Contains(issues[0].Detail, tc.part) {
			t.Fatal(tc, issues)
		}
	}
}

func TestMetadataPermissionFailureIsNotReportedAsLostFileContents(t *testing.T) {
	issues := backupIssues([]string{"Metadata capture incomplete: /etc/config: permission denied", "config.db capture failed: permission denied", "Permission denied (publickey)"})
	if issues[0].Code != "metadata_incomplete" || issues[1].Code != "pve_database" || issues[2].Title != "Zugriff wurde verweigert" {
		t.Fatal(issues)
	}
}
