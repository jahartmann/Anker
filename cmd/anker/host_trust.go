package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

func knownHostName(host string, port int) string {
	if port == 22 {
		return host
	}
	return "[" + host + "]:" + strconv.Itoa(port)
}

func verifiedHostKey(host string, port int, fingerprint, scan string) (string, error) {
	if err := setupTLSName(host); err != nil {
		return "", err
	}
	if port < 1 || port > 65535 {
		return "", errors.New("SSH-Port muss zwischen 1 und 65535 liegen")
	}
	for _, line := range strings.Split(scan, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[1] != "ssh-ed25519" {
			continue
		}
		key, err := base64.StdEncoding.DecodeString(fields[2])
		if err != nil || len(key) != 51 || !bytes.HasPrefix(key, []byte("\x00\x00\x00\x0bssh-ed25519\x00\x00\x00\x20")) {
			continue
		}
		sum := sha256.Sum256(key)
		if fingerprint == "SHA256:"+base64.RawStdEncoding.EncodeToString(sum[:]) {
			return knownHostName(host, port) + " ssh-ed25519 " + base64.StdEncoding.EncodeToString(key) + "\n", nil
		}
	}
	return "", errors.New("SSH-Hostfingerprint stimmt nicht überein; kein Schlüssel gespeichert. Fingerprint direkt auf dem Proxmox-Host prüfen")
}

func saveTrustedHost(path, name, line string, uid, gid int) error {
	lock, err := unix.Open(path+".lock", unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer unix.Close(lock)
	if err = unix.Flock(lock, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return errors.New("SSH-Hostschlüssel werden gerade bearbeitet; erneut versuchen")
	}
	var old []byte
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err == nil {
		f := os.NewFile(uintptr(fd), path)
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > 1<<20 {
			return errors.New("known_hosts ist ungültig oder zu groß")
		}
		old, err = io.ReadAll(io.LimitReader(f, (1<<20)+1))
		if err != nil {
			return err
		}
		if len(old) > 1<<20 {
			return errors.New("known_hosts ist zu groß")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	staged, err := os.MkdirTemp(filepath.Dir(path), ".anker-host-trust-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staged)
	file := filepath.Join(staged, "known_hosts")
	if err = os.WriteFile(file, old, 0600); err != nil {
		return err
	}
	// OpenSSH finds hashed entries too. Shared aliases, wildcards and authority
	// markers require deliberate manual editing; removing a complete matching
	// line could otherwise change trust for unrelated hosts.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	matches, matchErr := exec.CommandContext(ctx, "ssh-keygen", "-F", name, "-f", file).CombinedOutput()
	if matchErr != nil {
		var exitError *exec.ExitError
		if !errors.As(matchErr, &exitError) || exitError.ExitCode() != 1 || len(matches) != 0 {
			return fmt.Errorf("Bestehende Hosteinträge können nicht geprüft werden: %w", matchErr)
		}
	}
	for _, match := range strings.Split(string(matches), "\n") {
		fields := strings.Fields(match)
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		if strings.HasPrefix(fields[0], "@") || strings.ContainsAny(fields[0], ",*?!") {
			return errors.New("Hostschlüssel gehört zu gemeinsamem Alias, Muster oder Authority-Eintrag; diesen Eintrag zuerst manuell in known_hosts trennen")
		}
	}
	// Work only on a staged copy; interruption leaves the original untouched.
	if output, err := exec.CommandContext(ctx, "ssh-keygen", "-R", name, "-f", file).CombinedOutput(); err != nil {
		return fmt.Errorf("Bestehende Hosteinträge können nicht übernommen werden: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	old, err = os.ReadFile(file)
	if err != nil {
		return err
	}
	if len(old) > 0 && old[len(old)-1] != '\n' {
		old = append(old, '\n')
	}
	return setupWrite(path, append(old, []byte(line)...), 0640, uid, gid)
}

func runHostTrust(args []string) error {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return errors.New("SSH-Hostschlüssel als root auf dem Anker-Server einrichten")
	}
	if len(args) == 0 {
		return errors.New("anker host trust ADRESSE --fingerprint SHA256:... [--port 22]")
	}
	host := args[0]
	f := flag.NewFlagSet("host trust", flag.ContinueOnError)
	fingerprint := f.String("fingerprint", "", "Unabhängig geprüfter SHA256-Fingerprint des Ed25519-Hostschlüssels")
	port := f.Int("port", 22, "SSH-Port")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return errors.New("Nur eine Hostadresse angeben")
	}
	if err := setupTLSName(host); err != nil {
		return err
	}
	if *port < 1 || *port > 65535 {
		return errors.New("SSH-Port muss zwischen 1 und 65535 liegen")
	}
	decoded, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(*fingerprint, "SHA256:"))
	if err != nil || len(decoded) != 32 || !strings.HasPrefix(*fingerprint, "SHA256:") {
		return errors.New("SHA256-Fingerprint von der Proxmox-Konsole vollständig angeben")
	}
	account, err := user.Lookup("anker")
	if err != nil {
		return errors.New("Zuerst Anker auf diesem Server einrichten")
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "ssh-keyscan", "-T", "5", "-t", "ed25519", "-p", strconv.Itoa(*port), host).Output()
	if err != nil {
		return fmt.Errorf("SSH-Hostschlüssel nicht erreichbar: %w", err)
	}
	line, err := verifiedHostKey(host, *port, *fingerprint, string(output))
	if err != nil {
		return err
	}
	if err = saveTrustedHost("/etc/anker/known_hosts", knownHostName(host, *port), line, 0, gid); err != nil {
		return err
	}
	fmt.Println("SSH-Hostschlüssel geprüft und gespeichert. Host in Anker hinzufügen oder Verbindung erneut prüfen.")
	return nil
}
