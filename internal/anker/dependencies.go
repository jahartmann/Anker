package anker

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var legacyDependencyWarning = regexp.MustCompile(`^Referenced config/secret is not captured: (/[^()\r\n]+) \(([^()\r\n]+)\)$`)
var dependencyReference = regexp.MustCompile(`(?i)\b(keyfile|key_file|ssl_keyfile|privatekeyfile|certificatefile|certfile|ca_file|credentials|secret_file|include|source|script)\s*(?:[:=]\s*|\s+)["']?(/[^\s"';]+)`)
var pveVMConfig = regexp.MustCompile(`^etc/pve/(?:nodes/[^/]+/)?qemu-server/[0-9]+\.conf$`)
var runtimeRNGLine = regexp.MustCompile(`^rng0\s*:\s*source=/dev/(?:urandom|random)(?:,(?:max_bytes|period)=[0-9]+)*$`)
var vimReadableGuard = regexp.MustCompile(`^if\s+filereadable\(\s*(?:"(/[^"\r\n]+)"|'(/[^'\r\n]+)')\s*\)$`)
var vimIf = regexp.MustCompile(`^if\s+`)
var vimElse = regexp.MustCompile(`^(?:else|elseif)\b`)
var vimEnd = regexp.MustCompile(`^endif\b`)

// Old installed helpers over-report runtime RNG devices and optional Vim sources.
// Reconcile only newly collected, validated files; stored backups remain immutable.
func reconcileDependencyWarnings(stage string, c Collection) []string {
	entries := make(map[string]Entry, len(c.Entries))
	for _, e := range c.Entries {
		entries[e.Path] = e
	}
	var hashes map[string]string
	if err := json.Unmarshal(c.Inventory.Details["file_hashes"], &hashes); err != nil {
		hashes = nil
	}
	contents := map[string][]byte{}
	filtered := make([]string, 0, len(c.Warnings))
	for _, warning := range c.Warnings {
		match := legacyDependencyWarning.FindStringSubmatch(warning)
		if match == nil {
			filtered = append(filtered, warning)
			continue
		}
		candidate, config := match[1], match[2]
		device := strings.SplitN(candidate, ",", 2)[0]
		rng := pveVMConfig.MatchString(config) && (device == "/dev/urandom" || device == "/dev/random")
		vim := config == "etc/vim/vimrc" && candidate == "/etc/vim/vimrc.local"
		e, captured := entries[config]
		if (!rng && !vim) || !captured || e.Type != "file" || e.Size > 1<<20 {
			filtered = append(filtered, warning)
			continue
		}
		data, loaded := contents[config]
		if !loaded {
			// config came from the validated entry map, never from an absolute reference.
			p, err := safeJoin(filepath.Join(stage, "files"), config)
			if err == nil {
				data, _ = os.ReadFile(p)
			}
			contents[config] = data
		}
		if data == nil || bytes.IndexByte(data, 0) >= 0 || Hash(data) != e.SHA256 || !onlyOptionalReferences(string(data), candidate, vim) {
			filtered = append(filtered, warning)
			continue
		}
		if vim {
			// A matching vimrc hash proves its directory was enumerated by the host
			// inventory. A present/unreadable sibling appears in that same scan.
			_, present := hashes["etc/vim/vimrc.local"]
			_, captured := entries["etc/vim/vimrc.local"]
			if hashes[config] != e.SHA256 || present || captured || failedDependencyCapture(c.Warnings, "etc/vim/vimrc.local") {
				filtered = append(filtered, warning)
				continue
			}
		}
	}
	return filtered
}

// Every occurrence must be optional: one required include using the same path
// keeps the helper's deduplicated warning, even beside a legitimate RNG/guard.
func onlyOptionalReferences(content, candidate string, vim bool) bool {
	found := false
	var guards []string
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || (vim && strings.HasPrefix(line, `"`)) {
			continue
		}
		if vim {
			switch {
			case vimIf.MatchString(line):
				guard := ""
				if match := vimReadableGuard.FindStringSubmatch(line); match != nil {
					guard = match[1] + match[2]
				}
				guards = append(guards, guard)
			case vimElse.MatchString(line) && len(guards) > 0:
				guards[len(guards)-1] = ""
			case vimEnd.MatchString(line) && len(guards) > 0:
				guards = guards[:len(guards)-1]
			}
		}
		for _, match := range dependencyReference.FindAllStringSubmatch(line, -1) {
			if match[2] != candidate {
				continue
			}
			found = true
			if !strings.EqualFold(match[1], "source") {
				return false
			}
			if !vim {
				if !runtimeRNGLine.MatchString(line) {
					return false
				}
				continue
			}
			guarded := false
			for _, guard := range guards {
				guarded = guarded || guard == candidate
			}
			if !guarded || !strings.HasPrefix(line, "source ") || strings.Contains(line, "|") {
				return false
			}
		}
	}
	return found
}

func failedDependencyCapture(warnings []string, path string) bool {
	for _, warning := range warnings {
		failed, ok := strings.CutPrefix(warning, "Cannot capture /")
		if !ok {
			continue
		}
		failed, _, ok = strings.Cut(failed, ": ")
		if ok && (failed == path || strings.HasPrefix(path, failed+"/")) {
			return true
		}
	}
	return false
}
