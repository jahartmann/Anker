package main

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanPackageOfflineCLI(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "legacy.tar")
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	files := map[string]string{"plan.json": `{"id":"p","backup_id":"b","target_id":"t"}`, "mapping.json": "{}", "manifest.json": "{}", "WIEDERHERSTELLUNG.md": "alt", "original/manifest.json": "{}", "original/inventory/host.json": "{}"}
	files["original/checksums.sha256"] = fmt.Sprintf("%x  manifest.json\n%x  inventory/host.json\n", sha256.Sum256([]byte("{}")), sha256.Sum256([]byte("{}")))
	for n, b := range files {
		if err := tw.WriteHeader(&tar.Header{Name: n, Mode: 0600, Size: int64(len(b))}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(b))
	}
	tw.Close()
	os.WriteFile(input, raw.Bytes(), 0600)
	absent := filepath.Join(dir, "absent-data")
	err := runWithTerminal([]string{"--data", absent, "--socket", filepath.Join(dir, "absent.sock"), "plan-package", "verify", "--input", input}, false, nil)
	if err != nil {
		t.Fatalf("offline legacy verification should succeed with limited report: %v", err)
	}
	if _, err = os.Stat(absent); !os.IsNotExist(err) {
		t.Fatal("offline verifier created data root")
	}
	for _, args := range [][]string{{"plan-package", "verify"}, {"plan-package", "verify", "--input", input, "extra"}, {"plan-package", "extract", "--input", input}} {
		if err = runWithTerminal(args, false, nil); err == nil || strings.Contains(err.Error(), "connect") {
			t.Fatalf("bad args reached service: %v", err)
		}
	}
	os.WriteFile(input, []byte("corrupt"), 0600)
	if err = runWithTerminal([]string{"plan-package", "verify", "--input", input}, false, nil); err == nil {
		t.Fatal("corrupt archive accepted")
	}
}
