package anker

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type SSHCollector struct{}

func SSHArgs(h Host) ([]string, error) { return sshArgs(h, true) }
func RestoreSSHArgs(h Host) ([]string, error) {
	if h.RestoreKeyPath == "" || h.RestoreSSHUser == "" || h.RestoreKeyPath == h.KeyPath || h.RestoreSSHUser == h.SSHUser {
		return nil, errors.New("separater Wiederherstellungszugang erforderlich")
	}
	h.KeyPath = h.RestoreKeyPath
	h.SSHUser = h.RestoreSSHUser
	return sshArgs(h, false)
}
func sshArgs(h Host, readOnly bool) ([]string, error) {
	if !safeHost.MatchString(h.Address) || !safeHost.MatchString(h.SSHUser) || strings.Contains(h.SSHUser, ":") || h.SSHPort < 1 || h.SSHPort > 65535 {
		return nil, errors.New("ungültiges SSH-Ziel")
	}
	if h.KeyPath == "" || h.KnownHostsPath == "" || !filepath.IsAbs(h.KeyPath) || !filepath.IsAbs(h.KnownHostsPath) {
		return nil, errors.New("absolute SSH-Key- und Known-Hosts-Pfade erforderlich")
	}
	command := "sudo -n /usr/local/lib/anker/anker-host"
	if readOnly {
		command += " --read-only"
	}
	return []string{"-T", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "IdentitiesOnly=yes", "-o", "ConnectTimeout=15", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=2", "-o", "ForwardAgent=no", "-o", "UserKnownHostsFile=" + h.KnownHostsPath, "-i", h.KeyPath, "-p", strconv.Itoa(h.SSHPort), h.SSHUser + "@" + h.Address, command}, nil
}
func sshRun(ctx context.Context, h Host, request any, consume func(io.Reader) error) error {
	args, err := SSHArgs(h)
	if requestMap, ok := request.(map[string]any); ok && (requestMap["operation"] == "apply" || requestMap["operation"] == "rollback" || requestMap["operation"] == "restore-status") {
		args, err = RestoreSSHArgs(h)
		h.KeyPath = h.RestoreKeyPath
	}
	if err != nil {
		return err
	}
	for _, p := range []string{h.KeyPath, h.KnownHostsPath} {
		if _, err = os.Stat(p); err != nil {
			return fmt.Errorf("SSH-Datei nicht verfügbar: %w", err)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh", args...)
	b, err := json.Marshal(request)
	if err != nil {
		return err
	}
	cmd.Stdin = bytes.NewReader(append(b, '\n'))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return err
	}
	readErr := consume(io.LimitReader(pipe, 2<<30))
	if readErr != nil {
		cancel()
	}
	waitErr := cmd.Wait()
	if readErr != nil {
		return readErr
	}
	if waitErr != nil {
		return fmt.Errorf("SSH-Hostoperation fehlgeschlagen (%s); Hostschlüssel, Profil und Rechte prüfen", waitErr)
	}
	return nil
}
func (c SSHCollector) Probe(ctx context.Context, h Host) (Inventory, error) {
	var inv Inventory
	err := sshRun(ctx, h, map[string]any{"version": 1, "operation": "probe"}, func(r io.Reader) error { return json.NewDecoder(io.LimitReader(r, 16<<20)).Decode(&inv) })
	if err == nil && inv.PVEVersion == "" {
		err = errors.New("Ziel meldet keine Proxmox-Version")
	}
	inv.Fingerprint = Fingerprint(inv)
	return inv, err
}
func (c SSHCollector) Collect(ctx context.Context, h Host, dest string) (Collection, error) {
	var col Collection
	err := sshRun(ctx, h, map[string]any{"version": 1, "operation": "collect"}, func(r io.Reader) error { return ExtractTar(r, dest) })
	if err != nil {
		return col, err
	}
	data, err := os.ReadFile(filepath.Join(dest, "collection.json"))
	if err != nil {
		return col, err
	}
	err = json.Unmarshal(data, &col)
	if err == nil && col.Inventory.PVEVersion == "" {
		col.Warnings = append(col.Warnings, "keine Proxmox-Version erkannt")
	}
	return col, err
}
func (c SSHCollector) Apply(ctx context.Context, h Host, p Plan, root string) (ApplyResult, error) {
	var result ApplyResult
	files := []map[string]any{}
	mdata, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		return result, err
	}
	var m Manifest
	if err = json.Unmarshal(mdata, &m); err != nil {
		return result, err
	}
	entries := map[string]Entry{}
	for _, e := range m.Entries {
		entries[e.Path] = e
	}
	var metadata map[string]json.RawMessage
	if json.Unmarshal(p.Target.Details["file_metadata"], &metadata) != nil {
		return result, errors.New("Zielmetadaten fehlen; Plan mit aktualisiertem Hosthelfer neu erstellen")
	}
	users, groups := identityNames(p.Source, "users"), identityNames(p.Source, "groups")
	for _, step := range p.Steps {
		if step.Action != "apply" {
			continue
		}
		file, err := safeJoin(filepath.Join(root, "prepared-files"), step.Path)
		if err != nil {
			return result, err
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return result, err
		}
		if Hash(data) != step.PreparedSHA {
			return result, errors.New("vorbereitete Datei wurde verändert")
		}
		e := entries[step.Path]
		var expected any
		if value, ok := metadata[step.Path]; ok {
			if err = json.Unmarshal(value, &expected); err != nil {
				return result, errors.New("Zielmetadaten sind ungültig")
			}
		}
		files = append(files, map[string]any{"path": step.Path, "before_sha": step.BeforeSHA, "content": base64.StdEncoding.EncodeToString(data), "mode": e.Mode, "uid": e.UID, "gid": e.GID, "type": e.Type, "expected_metadata": expected, "user": users[strconv.Itoa(e.UID)], "group": groups[strconv.Itoa(e.GID)]})
	}
	req := map[string]any{"version": 1, "operation": "apply", "plan_id": p.ID, "confirm": p.ID, "expected_inventory": p.Target, "files": files}
	err = sshRun(ctx, h, req, func(r io.Reader) error {
		var decodeErr error
		result, decodeErr = decodeRecoveryResult(r)
		return decodeErr
	})
	if result.Error != "" {
		err = errors.Join(err, fmt.Errorf("Wiederherstellung: %s", result.Error))
	}
	return result, err
}

func (c SSHCollector) RestoreStatus(ctx context.Context, h Host, p Plan) (ApplyResult, error) {
	var result ApplyResult
	err := sshRun(ctx, h, map[string]any{"version": 1, "operation": "restore-status", "plan_id": p.ID}, func(r io.Reader) error {
		var decodeErr error
		result, decodeErr = decodeRecoveryResult(r)
		return decodeErr
	})
	if err != nil {
		return result, fmt.Errorf("Hostprotokoll nicht erreichbar oder lesbar: %w", err)
	}
	return result, nil
}
func (c SSHCollector) Rollback(ctx context.Context, h Host, p Plan, confirmation string) (ApplyResult, error) {
	var result ApplyResult
	err := sshRun(ctx, h, map[string]any{"version": 1, "operation": "rollback", "plan_id": p.ID, "confirm": confirmation}, func(r io.Reader) error {
		var decodeErr error
		result, decodeErr = decodeRecoveryResult(r)
		return decodeErr
	})
	if result.Error != "" {
		err = errors.Join(err, fmt.Errorf("Rücksetzung: %s", result.Error))
	}
	return result, err
}

// Reject incomplete or multiple JSON values rather than retaining partially decoded evidence.
func decodeRecoveryResult(r io.Reader) (ApplyResult, error) {
	var result ApplyResult
	decoder := json.NewDecoder(io.LimitReader(r, (1<<20)+1))
	if err := decoder.Decode(&result); err != nil {
		return ApplyResult{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return ApplyResult{}, errors.New("Ungültige zusätzliche Hostantwort")
	}
	return result, nil
}
