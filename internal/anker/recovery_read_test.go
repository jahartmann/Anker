package anker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Keep the initial RED behavioral even before the optional interface is implemented.
type recoveryReadAPI interface {
	ReadRecoveryFiles(context.Context, Host, []string, []string) (RecoveryReadResult, error)
}

func TestRecoveryReadUsesBackupSSHAuthority(t *testing.T) {
	reader, ok := any(SSHCollector{}).(recoveryReadAPI)
	if !ok {
		t.Fatal("read-only recovery transport missing")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$ANKER_READ_ARGS\"\ncat > \"$ANKER_READ_REQUEST\"\nprintf '%s' \"$ANKER_READ_REPLY\"\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, name := range []string{"backup", "restore", "known"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("ANKER_READ_ARGS", filepath.Join(dir, "args"))
	t.Setenv("ANKER_READ_REQUEST", filepath.Join(dir, "request"))
	reply := `{"files":[{"path":"etc/a","state":"missing"}],"storage_volumes":{"local":{"state":"ok","volumes":[]}}}`
	t.Setenv("ANKER_READ_REPLY", reply)
	h := Host{Address: "192.0.2.2", SSHPort: 22, SSHUser: "anker", KeyPath: filepath.Join(dir, "backup"), RestoreSSHUser: "anker-restore", RestoreKeyPath: filepath.Join(dir, "restore"), KnownHostsPath: filepath.Join(dir, "known")}
	result, err := reader.ReadRecoveryFiles(t.Context(), h, []string{"etc/a"}, []string{"local"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Files[0].State != "missing" || result.StorageVolumes["local"].Volumes == nil {
		t.Fatalf("lost state: %+v", result)
	}
	args, _ := os.ReadFile(filepath.Join(dir, "args"))
	if !strings.Contains(string(args), "--read-only") || !strings.Contains(string(args), "anker@192.0.2.2") || strings.Contains(string(args), filepath.Join(dir, "restore")) {
		t.Fatalf("wrong authority: %s", args)
	}
	request, _ := os.ReadFile(filepath.Join(dir, "request"))
	var payload map[string]any
	json.Unmarshal(request, &payload)
	if payload["operation"] != "read-recovery" || payload["version"] != float64(1) {
		t.Fatalf("wrong envelope: %s", request)
	}
	for _, bad := range []string{reply + ` {}`, reply + ` garbage`, `{"files":[],"storage_volumes":{}}`, `{"files":[{"path":"etc/a","state":"missing"},{"path":"etc/a","state":"missing"}],"storage_volumes":{}}`, `{"files":[{"path":"etc/other","state":"missing"}],"storage_volumes":{}}`, reply + strings.Repeat(" ", 16<<20)} {
		t.Setenv("ANKER_READ_REPLY", bad)
		// Large responses use a file so the process environment remains within OS limits.
		if len(bad) > 1<<20 {
			if err := os.WriteFile(filepath.Join(dir, "reply"), []byte(bad), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("ANKER_READ_REPLY", "")
			os.WriteFile(filepath.Join(dir, "ssh"), []byte("#!/bin/sh\ncat > /dev/null\ncat \"$ANKER_READ_REPLY_FILE\"\n"), 0700)
			t.Setenv("ANKER_READ_REPLY_FILE", filepath.Join(dir, "reply"))
		}
		got, err := reader.ReadRecoveryFiles(t.Context(), h, []string{"etc/a"}, []string{"local"})
		if err == nil || len(got.Files) > 0 {
			t.Fatalf("malformed response accepted: %v", err)
		}
	}
}

func TestRecoveryReadRejectsInvalidEvidence(t *testing.T) {
	// This is the host's literal empty-file wire shape, including explicit zero-valued metadata.
	good := `{"files":[{"path":"etc/a","state":"read","sha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","content":"","metadata":{"path":"etc/a","type":"file","mode":420,"uid":0,"gid":0,"size":0,"xattrs":{},"sha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}],"storage_volumes":{"local":{"state":"ok","volumes":[]}}}`
	if _, err := decodeRecoveryRead(strings.NewReader(good), []string{"etc/a"}, []string{"local"}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		strings.Replace(good, `"content":"",`, "", 1), strings.Replace(good, `"uid":0,`, "", 1), strings.Replace(good, `"xattrs":{}`, `"xattrs":null`, 1), strings.Replace(good, `"content":""`, `"content":"YQ=="`, 1), strings.Replace(good, `"volumes":[]`, `"volumes":["other:disk"]`, 1), strings.Replace(good, `"volumes":[]`, `"volumes":null`, 1), strings.Replace(good, `"state":"ok"`, `"state":"invented"`, 1), strings.Replace(good, `"state":"read"`, `"state":"missing"`, 1), strings.Replace(good, `"state":"read"`, `"state":"read","state":"read"`, 1), strings.Replace(good, `"mode":420`, `"mode":999999`, 1),
	} {
		if got, err := decodeRecoveryRead(strings.NewReader(bad), []string{"etc/a"}, []string{"local"}); err == nil || len(got.Files) > 0 {
			t.Fatalf("invalid evidence accepted: %s", bad)
		}
	}
}

func TestRecoveryReadRejectsMalformedMetadataAndEnvelope(t *testing.T) {
	for _, raw := range []string{`null`, `{}`, `{"files":null,"storage_volumes":null}`} {
		if _, err := decodeRecoveryRead(strings.NewReader(raw), nil, nil); err == nil {
			t.Fatalf("accepted incomplete envelope: %s", raw)
		}
	}
	f := RecoveryReadFile{Path: "etc/a", State: "read", SHA256: Hash(nil), Content: []byte{}, Metadata: &Entry{Path: "etc/a", Type: "file", Mode: 0644, Size: 0, SHA256: Hash(nil), XAttrs: map[string]string{"user.secret": "not base64!"}}}
	result := RecoveryReadResult{Files: []RecoveryReadFile{f}, StorageVolumes: map[string]RecoveryVolumeResult{}}
	if err := validateRecoveryRead(result, []string{"etc/a"}, nil); err == nil {
		t.Fatal("malformed xattrs accepted")
	}
	f.Metadata.XAttrs = map[string]string{}
	f.Metadata.UID = 1 << 40
	if err := validateRecoveryRead(result, []string{"etc/a"}, nil); err == nil {
		t.Fatal("invalid owner accepted")
	}
}
