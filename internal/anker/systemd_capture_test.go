package anker

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const escapedMountPath = `etc/systemd/system/local-fs.target.wants/run-proxmox\x2dbackup.mount`
const escapedMountLink = `/usr/lib/systemd/system/run-proxmox\x2dbackup.mount`

type legacySystemdCollector struct {
	fixtureCollector
	mutation string
}

func (collector legacySystemdCollector) Collect(ctx context.Context, host Host, destination string) (Collection, error) {
	collection, err := collector.fixtureCollector.Collect(ctx, host, destination)
	if err != nil {
		return collection, err
	}
	parent := filepath.Dir(escapedMountPath)
	if err := os.MkdirAll(filepath.Join(destination, "files", parent), 0700); err != nil {
		return collection, err
	}
	collection.Entries = append(collection.Entries, Entry{Path: parent, Type: "directory", Mode: 0755})
	meta := map[string]any{"type": "symlink", "mode": 0777, "uid": 0, "gid": 0, "xattrs": map[string]string{}, "link": escapedMountLink}
	hash := "symlink:" + escapedMountLink
	switch collector.mutation {
	case "missing metadata":
		meta = nil
	case "wrong hash":
		hash = "symlink:/different"
	case "file":
		meta["type"] = "file"
	case "missing uid":
		delete(meta, "uid")
	case "metadata error":
		meta["metadata_warning"] = "denied"
	case "invalid attributes":
		meta["xattrs"] = map[string]string{"user.test": "not-base64"}
	case "known time":
		meta["mtime"] = int64(1700000000)
	case "no parent":
		collection.Entries = collection.Entries[:len(collection.Entries)-1]
	}
	metadata, _ := json.Marshal(map[string]any{escapedMountPath: meta})
	hashes, _ := json.Marshal(map[string]string{escapedMountPath: hash})
	collection.Inventory.Details = map[string]json.RawMessage{"file_metadata": metadata, "file_hashes": hashes}
	collection.Warnings = []string{"Cannot capture /" + escapedMountPath + ": invalid path"}
	return collection, nil
}

func TestSystemdEscapedPathDoesNotRelaxOtherPathGuards(t *testing.T) {
	for _, path := range []string{escapedMountPath, `etc/systemd/user/user\x20name.service`, "files/" + escapedMountPath, "original/original-files/" + escapedMountPath} {
		if !validPackagePath(path) {
			t.Fatalf("valid package path rejected: %q", path)
		}
		if err := ValidPath(path); err != nil {
			t.Fatalf("valid literal systemd escape: %s: %v", path, err)
		}
	}
	for _, path := range []string{"unknown/" + escapedMountPath, "files/files/" + escapedMountPath, `etc\systemd\system\a.mount`, `etc/systemd/system/..\escape`, `etc/systemd/system/name\xGG.mount`, `etc/app/name\x2d.conf`, `etc/systemd/system/../secret`, `/etc/systemd/system/a\x2d.mount`, `etc/systemd/system/a\x2d.mount/../../../../secret`} {
		if validPackagePath(path) {
			t.Fatalf("unsafe package path accepted: %q", path)
		}
		if err := ValidPath(path); err == nil {
			t.Fatalf("unsafe path accepted: %q", path)
		}
	}
}

func TestNewBackupRecoversEscapedSystemdLinkWithExistingCollector(t *testing.T) {
	service := testService(t)
	service.Collector = legacySystemdCollector{}
	before, _ := service.Host("host1")
	backup, err := service.CreateBackup(context.Background(), "host1")
	if err != nil {
		t.Fatal(err)
	}
	if backup.Status != "successful" || len(backup.Warnings) != 0 {
		t.Fatalf("link not recovered: %+v", backup)
	}
	after, _ := service.Host("host1")
	// A backup refreshes observed inventory, not registration or credentials.
	before.Inventory, after.Inventory = nil, nil
	before.LastProbe, after.LastProbe = "", ""
	before.ProbeError, after.ProbeError = "", ""
	if !reflect.DeepEqual(before, after) {
		t.Fatal("existing host registration changed")
	}
	manifest, err := service.Manifest(backup.ID)
	if err != nil {
		t.Fatal(err)
	}
	var link *Entry
	for index := range manifest.Entries {
		if manifest.Entries[index].Path == escapedMountPath {
			link = &manifest.Entries[index]
		}
	}
	if link == nil || link.Type != "symlink" || link.Link != escapedMountLink || link.CaptureOrigin != "inventory_symlink" || !link.MTimeUnknown || link.MTime != 0 {
		t.Fatalf("invalid recovered evidence: %+v", link)
	}
	read, entry, readErr := service.ReadFile(backup.ID, escapedMountPath)
	if readErr != nil || string(read) != escapedMountLink || entry.Path != escapedMountPath {
		t.Fatal("literal path cannot be read through the backup service", readErr)
	}
	if err := service.VerifyBackup(backup.ID); err != nil {
		t.Fatal("recovered backup failed integrity verification", err)
	}
	contents, err := os.ReadFile(filepath.Join(service.backupDir(backup), "files", escapedMountPath))
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != escapedMountLink {
		t.Fatal("link target was decoded, followed or changed")
	}
	if len(comparisonMetadata(link, &Entry{Type: link.Type, Mode: link.Mode, UID: link.UID, GID: link.GID, Link: link.Link, XAttrs: link.XAttrs, MTime: 12345})) != 0 {
		t.Fatal("unknown mtime claimed as timestamp change")
	}
	readiness, err := service.BackupReadiness(backup.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range readiness.Items {
		if item.ID == "systemd_link_time" && item.State == "unknown" {
			found = true
		}
	}
	if !found {
		t.Fatal("unknown original time was not disclosed")
	}
	var output bytes.Buffer
	writer := tar.NewWriter(&output)
	if err := service.tarOriginalFiles(writer, backup, manifest, ""); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	found = false
	reader := tar.NewReader(&output)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(header.Name, escapedMountPath) {
			found = true
		}
		if strings.HasSuffix(header.Name, escapedMountPath) && (header.Typeflag != tar.TypeSymlink || header.Linkname != escapedMountLink || header.PAXRecords["ANKER.mtime_unknown"] != "1") {
			t.Fatal("export lost literal link or provenance", header)
		}
	}
	if !found {
		t.Fatal("export omitted recovered link")
	}
	if !strings.Contains(recoveryGuide(manifest), "mtime_unknown") {
		t.Fatal("offline guide hides unknown original time")
	}
	// The same literal names must survive cold storage and offline plan packages.
	if err := service.ArchiveBackup(backup.ID); err != nil {
		t.Fatal(err)
	}
	if err := service.VerifyBackup(backup.ID); err != nil {
		t.Fatal("archived backup failed verification", err)
	}
	if err := service.SaveHost(Host{ID: "target", Name: "new-pve", Address: "192.0.2.20", SSHUser: "anker", SSHPort: 22}); err != nil {
		t.Fatal(err)
	}
	service.Collector = targetCollector{inv: Inventory{Hostname: "new-pve", PVEVersion: "8.4", Interfaces: []Interface{{Name: "ens3"}}}}
	plan, err := service.CreatePlan(context.Background(), PlanRequest{BackupID: backup.ID, TargetID: "target", Scenario: "migration", Mapping: Mapping{Interfaces: map[string]string{"eno1": "ens3"}}, ConsoleConfirmed: true, SourceOffline: true})
	if err != nil {
		t.Fatal(err)
	}
	var planOutput bytes.Buffer
	if err := service.ExportPlan(plan.ID, &planOutput); err != nil {
		t.Fatal(err)
	}
	if result, err := VerifyPlanPackage(bytes.NewReader(planOutput.Bytes())); err != nil || result.Coverage != "complete" {
		t.Fatal("offline package lost escaped systemd link", result, err)
	}

}

func TestSystemdCompatibilityPreservesUncorroboratedWarnings(t *testing.T) {
	for _, mutation := range []string{"missing metadata", "wrong hash", "file", "missing uid", "metadata error", "invalid attributes", "no parent"} {
		t.Run(mutation, func(t *testing.T) {
			service := testService(t)
			service.Collector = legacySystemdCollector{mutation: mutation}
			backup, err := service.CreateBackup(context.Background(), "host1")
			if err != nil {
				t.Fatal(err)
			}
			if backup.Status != "partial" || len(backup.Warnings) != 1 {
				t.Fatalf("unproven capture hidden: %+v", backup)
			}
		})
	}
}

func TestLegacySystemdIssueNamesTheActualSafePath(t *testing.T) {
	issues := backupIssues([]string{"Cannot capture /" + escapedMountPath + ": invalid path"})
	if len(issues) != 1 || issues[0].Code != "legacy_systemd_path" || issues[0].Path != "/"+escapedMountPath || issues[0].PathProtected {
		t.Fatal("legacy path still hidden or misclassified", issues)
	}
}
