package anker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type legacyDependencyCollector struct {
	fixtureCollector
	files     map[string]string
	warnings  []string
	hashes    map[string]string
	rawHashes json.RawMessage
}

func (f legacyDependencyCollector) Collect(ctx context.Context, h Host, dest string) (Collection, error) {
	c, err := f.fixtureCollector.Collect(ctx, h, dest)
	if err != nil {
		return c, err
	}
	for name, content := range f.files {
		p := filepath.Join(dest, "files", name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			return c, err
		}
		if err := os.WriteFile(p, []byte(content), 0600); err != nil {
			return c, err
		}
		c.Entries = append(c.Entries, Entry{Path: name, Type: "file", Mode: 0600})
	}
	c.Warnings = append(c.Warnings, f.warnings...)
	if f.hashes != nil {
		raw, err := json.Marshal(f.hashes)
		if err != nil {
			return c, err
		}
		c.Inventory.Details = map[string]json.RawMessage{"file_hashes": raw}
	}
	if f.rawHashes != nil {
		c.Inventory.Details = map[string]json.RawMessage{"file_hashes": f.rawHashes}
	}
	return c, nil
}

func TestBackupIgnoresLegacyRNGWarningConfirmedByCapturedVMConfig(t *testing.T) {
	for _, config := range []string{"etc/pve/qemu-server/9901.conf", "etc/pve/nodes/zp01/qemu-server/9901.conf"} {
		for _, device := range []string{"/dev/urandom", "/dev/random"} {
			for _, options := range []string{"", ",max_bytes=1024,period=1000"} {
				t.Run(config+device+options, func(t *testing.T) {
					s := testService(t)
					candidate := device + options
					s.Collector = legacyDependencyCollector{files: map[string]string{config: "rng0: source=" + candidate + "\n"}, warnings: []string{"Referenced config/secret is not captured: " + candidate + " (" + config + ")"}}
					b, err := s.CreateBackup(context.Background(), "host1")
					if err != nil {
						t.Fatal(err)
					}
					if b.Status != "successful" || len(b.Warnings) != 0 {
						t.Fatalf("runtime RNG marked backup incomplete: %+v", b)
					}
					m, err := s.Manifest(b.ID)
					if err != nil || m.Status != "successful" || len(m.Warnings) != 0 {
						t.Fatal(m, err)
					}
					if err := s.VerifyBackup(b.ID); err != nil {
						t.Fatal(err)
					}
				})
			}
		}
	}
}

func TestBackupPreservesGenuineDependenciesBesideLegacyRNG(t *testing.T) {
	config := "etc/pve/nodes/zp01/qemu-server/9901.conf"
	genuine := []string{"Referenced config/secret is not captured: /opt/secrets/auth (" + config + ")", "Referenced hookscript is not captured: /var/lib/vz/snippets/hook.sh (" + config + ")", "Cannot capture /etc/ssl/private/key.pem: permission denied"}
	s := testService(t)
	s.Collector = legacyDependencyCollector{files: map[string]string{config: "rng0: source=/dev/urandom\ncredentials=/opt/secrets/auth\nhookscript: local:snippets/hook.sh\n"}, warnings: append([]string{"Referenced config/secret is not captured: /dev/urandom (" + config + ")"}, genuine...)}
	b, err := s.CreateBackup(context.Background(), "host1")
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != "partial" || !reflect.DeepEqual(b.Warnings, genuine) {
		t.Fatalf("required dependencies lost: %+v", b)
	}
}

func TestBackupPreservesUnconfirmedLegacyRNGWarnings(t *testing.T) {
	config := "etc/pve/nodes/zp01/qemu-server/9901.conf"
	cases := []struct{ path, content, candidate string }{
		{config, "source=/dev/urandom\n", "/dev/urandom"},
		{config, "keyfile=/dev/urandom\n", "/dev/urandom"},
		{config, "# rng0: source=/dev/urandom\n", "/dev/urandom"},
		{config, "rng0: source=/opt/secrets/random.seed\n", "/opt/secrets/random.seed"},
		{config, "rng0: source=/dev/hwrng\n", "/dev/hwrng"},
		{"etc/app.conf", "rng0: source=/dev/urandom\n", "/dev/urandom"},
		{config, "rng0: source=/dev/random\n", "/dev/urandom"},
		{config, "rng0: source=/dev/urandom\ninclude /dev/urandom\n", "/dev/urandom"},
	}
	for _, tc := range cases {
		t.Run(tc.path+tc.content, func(t *testing.T) {
			s := testService(t)
			warning := "Referenced config/secret is not captured: " + tc.candidate + " (" + tc.path + ")"
			s.Collector = legacyDependencyCollector{files: map[string]string{tc.path: tc.content}, warnings: []string{warning}}
			b, err := s.CreateBackup(context.Background(), "host1")
			if err != nil {
				t.Fatal(err)
			}
			if b.Status != "partial" || !reflect.DeepEqual(b.Warnings, []string{warning}) {
				t.Fatalf("unconfirmed dependency suppressed: %+v", b)
			}
		})
	}
}

func TestBackupIgnoresAbsentGuardedLegacyVimSourceWithInventoryEvidence(t *testing.T) {
	config := "etc/vim/vimrc"
	content := "if filereadable(\"/etc/vim/vimrc.local\")\n  source /etc/vim/vimrc.local\nendif\n"
	s := testService(t)
	s.Collector = legacyDependencyCollector{files: map[string]string{config: content}, hashes: map[string]string{config: Hash([]byte(content))}, warnings: []string{"Referenced config/secret is not captured: /etc/vim/vimrc.local (etc/vim/vimrc)"}}
	b, err := s.CreateBackup(context.Background(), "host1")
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != "successful" || len(b.Warnings) != 0 {
		t.Fatalf("proven absent optional Vim source marked incomplete: %+v", b)
	}
}

func TestBackupPreservesVimDependencyWithoutProofOfOptionalAbsence(t *testing.T) {
	config := "etc/vim/vimrc"
	guarded := "if filereadable(\"/etc/vim/vimrc.local\")\n source /etc/vim/vimrc.local\nendif\n"
	warning := "Referenced config/secret is not captured: /etc/vim/vimrc.local (etc/vim/vimrc)"
	cases := []struct {
		name, content string
		hashes        map[string]string
		extra         string
	}{
		{"no inventory", guarded, nil, ""},
		{"directory scan unproven", guarded, map[string]string{}, ""},
		{"source changed", guarded, map[string]string{config: Hash([]byte("changed"))}, ""},
		{"present omitted source", guarded, map[string]string{config: Hash([]byte(guarded)), "etc/vim/vimrc.local": Hash([]byte("set number"))}, ""},
		{"unreadable source", guarded, map[string]string{config: Hash([]byte(guarded)), "etc/vim/vimrc.local": "unreadable"}, ""},
		{"source capture failed", guarded, map[string]string{config: Hash([]byte(guarded))}, "Cannot capture /etc/vim/vimrc.local: permission denied"},
		{"parent capture failed", guarded, map[string]string{config: Hash([]byte(guarded))}, "Cannot capture /etc/vim: permission denied"},
		{"unguarded", "source /etc/vim/vimrc.local\n", map[string]string{config: Hash([]byte("source /etc/vim/vimrc.local\n"))}, ""},
		{"required source later", guarded + "source /etc/vim/vimrc.local\n", map[string]string{config: Hash([]byte(guarded + "source /etc/vim/vimrc.local\n"))}, ""},
		{"required source inline", "if filereadable(\"/etc/vim/vimrc.local\")\n source /etc/vim/vimrc.local | endif | source /etc/vim/vimrc.local\n", map[string]string{config: Hash([]byte("if filereadable(\"/etc/vim/vimrc.local\")\n source /etc/vim/vimrc.local | endif | source /etc/vim/vimrc.local\n"))}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := testService(t)
			warnings := []string{warning}
			if tc.extra != "" {
				warnings = append(warnings, tc.extra)
			}
			s.Collector = legacyDependencyCollector{files: map[string]string{config: tc.content}, hashes: tc.hashes, warnings: warnings}
			b, err := s.CreateBackup(context.Background(), "host1")
			if err != nil {
				t.Fatal(err)
			}
			if b.Status != "partial" || !reflect.DeepEqual(b.Warnings, warnings) {
				t.Fatalf("unproven optional absence suppressed: %+v", b)
			}
		})
	}
}

func TestBackupPreservesVimWarningWithInvalidInventoryScan(t *testing.T) {
	config := "etc/vim/vimrc"
	content := "if filereadable(\"/etc/vim/vimrc.local\")\n source /etc/vim/vimrc.local\nendif\n"
	warning := "Referenced config/secret is not captured: /etc/vim/vimrc.local (etc/vim/vimrc)"
	s := testService(t)
	// Valid JSON with an invalid scan value can otherwise leave a partially decoded map.
	raw, err := json.Marshal(map[string]any{config: Hash([]byte(content)), "etc/invalid": 42})
	if err != nil {
		t.Fatal(err)
	}
	s.Collector = legacyDependencyCollector{files: map[string]string{config: content}, rawHashes: raw, warnings: []string{warning}}
	b, err := s.CreateBackup(context.Background(), "host1")
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != "partial" || !reflect.DeepEqual(b.Warnings, []string{warning}) {
		t.Fatalf("invalid inventory evidence suppressed warning: %+v", b)
	}
}
