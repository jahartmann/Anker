package main

import (
	"anker/internal/updater"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"
)

// Read just this answer: a buffered reader must not consume the next password.
func setupAnswer(reader io.Reader, fallback string) (string, error) {
	var b []byte
	for {
		one := make([]byte, 1)
		n, err := reader.Read(one)
		if n > 0 {
			if one[0] == '\n' {
				break
			}
			b = append(b, one[0])
			if len(b) > 4096 {
				return "", errors.New("Eingabe zu lang")
			}
		}
		if err != nil {
			return "", fmt.Errorf("Eingabe abgebrochen: %w", err)
		}
	}
	answer := strings.TrimSpace(string(b))
	if answer == "" {
		answer = fallback
	}
	return answer, nil
}
func setupPrompt(label, fallback string) (string, error) {
	if fallback != "" {
		fmt.Printf("%s [%s]: ", label, fallback)
	} else {
		fmt.Printf("%s: ", label)
	}
	return setupAnswer(os.Stdin, fallback)
}
func setupSecret(label string) (string, error) {
	if !term.IsTerminal(os.Stdin.Fd()) {
		return "", errors.New("Verdeckte Eingabe benötigt ein Terminal")
	}
	fmt.Print(label + ": ")
	b, err := term.ReadPassword(os.Stdin.Fd())
	fmt.Println()
	return string(b), err
}
func initialPassword() (string, error) {
	for {
		first, err := setupSecret("Administratorpasswort (mindestens 12 Zeichen)")
		if err != nil {
			return "", err
		}
		second, err := setupSecret("Passwort wiederholen")
		if err != nil {
			return "", err
		}
		if first != second {
			fmt.Println("Die Passwörter stimmen nicht überein.")
			continue
		}
		if utf8.RuneCountInString(first) < 12 || len(first) > 1024 {
			fmt.Println("Das Passwort muss mindestens 12 Zeichen und höchstens 1024 Bytes haben.")
			continue
		}
		return first, nil
	}
}
func setupListen(listen string) (bool, error) {
	if strings.ContainsAny(listen, " \t\r\n\"'\\") {
		return false, errors.New("Listen-Adresse enthält ungültige Zeichen")
	}
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return false, err
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return false, errors.New("Port muss zwischen 1 und 65535 liegen")
	}
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback()), nil
}
func setupWebEnv(listen, cert, key string) (string, error) {
	local, err := setupListen(listen)
	if err != nil {
		return "", err
	}
	if !local && (cert == "" || key == "") {
		return "", errors.New("LAN-/VPN-Zugriff benötigt TLS")
	}
	if (cert == "") != (key == "") {
		return "", errors.New("Zertifikat und Schlüssel zusammen angeben")
	}
	text := "ANKER_LISTEN=" + listen + "\n"
	if cert != "" {
		for _, p := range []string{cert, key} {
			if !filepath.IsAbs(p) || strings.ContainsAny(p, "\r\n\x00") {
				return "", errors.New("Absolute TLS-Dateipfade ohne Steuerzeichen verwenden")
			}
		}
		if _, err = tls.LoadX509KeyPair(cert, key); err != nil {
			return "", fmt.Errorf("TLS-Dateien passen nicht: %w", err)
		}
		text += "ANKER_TLS_CERT=" + strconv.Quote(cert) + "\nANKER_TLS_KEY=" + strconv.Quote(key) + "\n"
	}
	return text, nil
}
func validateSetupKey(key string) error {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(key))
	if err != nil || len(b) != ed25519.PublicKeySize {
		return errors.New("public.key muss einen öffentlichen Ed25519-Schlüssel enthalten (Base64, 32 Bytes)")
	}
	return nil
}
func validateSetupRepo(repo string) error {
	if !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(repo) || strings.Contains(repo, "..") {
		return errors.New("GitHub-Repository als owner/repo angeben")
	}
	return nil
}
func setupUpdateIdle() error {
	for _, p := range []string{updater.Maintenance, filepath.Join(updater.StateDir, "pending.json")} {
		if _, err := os.Lstat(p); err == nil {
			return errors.New("Ein Update ist in Wartung; zuerst anker update status prüfen")
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	response, err := updater.UnixClient(updater.Socket, 2*time.Second).Get("http://updater.local/status")
	if err != nil {
		return nil
	}
	defer response.Body.Close()
	var state updater.State
	if err = json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&state); err != nil {
		return err
	}
	if state.Status == "installing" {
		return errors.New("Update läuft; Einrichtung erst nach dessen Abschluss öffnen")
	}
	return nil
}
func setupEnvValue(text, name string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, name+"=") {
			value := strings.TrimPrefix(line, name+"=")
			if decoded, err := strconv.Unquote(value); err == nil {
				return decoded
			}
			return value
		}
	}
	return ""
}
func setupWrite(path string, b []byte, mode os.FileMode, uid, gid int) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".anker-setup-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err == nil {
		err = f.Chown(uid, gid)
	}
	if err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
func setupCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s fehlgeschlagen: %w", filepath.Base(name), err)
	}
	return nil
}
func setupSSHKey(path string, uid, gid int) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		// Commit a complete, synced private key; a retry derives the public half.
		staging, err := os.MkdirTemp(filepath.Dir(path), ".anker-key-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(staging)
		generated := filepath.Join(staging, "key")
		if err = exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "anker-"+filepath.Base(path), "-f", generated).Run(); err != nil {
			return err
		}
		b, err := os.ReadFile(generated)
		if err != nil {
			return err
		}
		if err = setupWrite(path, b, 0600, uid, gid); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if !info.Mode().IsRegular() {
		return errors.New("SSH-Schlüssel muss eine reguläre Datei sein: " + path)
	}
	// Repair the ownership left by an interrupted older setup without replacing it.
	if err = os.Chown(path, uid, gid); err != nil {
		return err
	}
	if err = os.Chmod(path, 0600); err != nil {
		return err
	}
	public, err := exec.Command("ssh-keygen", "-y", "-P", "", "-f", path).Output()
	if err != nil {
		return fmt.Errorf("SSH-Schlüssel %s ist ungültig oder passwortgeschützt; vor Fortsetzen prüfen", path)
	}
	return setupWrite(path+".pub", append(bytes.TrimSpace(public), '\n'), 0644, os.Geteuid(), gid)
}
func setupReady(socket, path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	client := updater.UnixClient(socket, 2*time.Second)
	for {
		req, _ := http.NewRequestWithContext(ctx, "GET", "http://anker.local"+path, nil)
		response, err := client.Do(req)
		if err == nil {
			response.Body.Close()
			if response.StatusCode == 200 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return errors.New("Dienst wird nicht bereit; journalctl -u anker -u anker-updater prüfen")
		case <-time.After(time.Second):
		}
	}
}
func runSetup(configureUpdates bool) error {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return errors.New("Servereinrichtung als root auf Linux mit systemd ausführen")
	}
	if !term.IsTerminal(os.Stdin.Fd()) {
		return errors.New("Interaktive Einrichtung: sudo anker setup in einem Terminal ausführen")
	}
	if _, err := os.Stat("/etc/anker/install-pending"); err == nil {
		return errors.New("Erstinstallation unterbrochen; scripts/install-server.sh erneut ausführen")
	}
	setupLock, err := os.OpenFile("/etc/anker/setup.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer setupLock.Close()
	if err = unix.Flock(int(setupLock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return errors.New("Eine Servereinrichtung läuft bereits")
	}
	for _, name := range []string{"runuser", "systemctl", "ssh-keygen"} {
		if _, err := exec.LookPath(name); err != nil {
			return err
		}
	}
	if err := setupUpdateIdle(); err != nil {
		return err
	}
	account, err := user.Lookup("anker")
	if err != nil {
		return errors.New("Zuerst scripts/install-server.sh ausführen")
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil {
		return err
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil {
		return err
	}
	fmt.Println("Anker · Servereinrichtung\nBestehende Benutzer und SSH-Schlüssel bleiben erhalten.")
	initialized, err := setupInitialized(filepath.Join(updater.DataDir, "catalog.db"))
	if err != nil {
		return err
	}
	admin, password := "admin", ""
	if !initialized {
		admin, err = setupPrompt("Administratorname", "admin")
		if err != nil {
			return err
		}
		password, err = initialPassword()
		if err != nil {
			return err
		}
	} else {
		fmt.Println("Administratorzugang ist bereits eingerichtet.")
	}
	priorEnv, err := os.ReadFile("/etc/anker/service.env")
	if err != nil {
		return err
	}
	web, err := setupConfigureWeb(string(priorEnv), initialized)
	if err != nil {
		return err
	}
	cfg := updater.Config{Repository: "jahartmann/Anker"}
	if b, err := os.ReadFile(updater.ConfigPath); err == nil {
		if err = json.Unmarshal(b, &cfg); err != nil {
			return err
		}
	}
	defaultKey := ""
	if _, err = os.Stat("/etc/anker/release-public.key"); err == nil {
		defaultKey = "/etc/anker/release-public.key"
	}
	keyFile := ""
	if configureUpdates {
		keyFile, err = setupPrompt("Update-Schlüssel public.key (leer = bestehende Einrichtung behalten/überspringen)", defaultKey)
		if err != nil {
			return err
		}
	} else if cfg.PublicKey == "" && defaultKey != "" {
		keyFile = defaultKey
		b, err := os.ReadFile(keyFile)
		if err != nil {
			return err
		}
		cfg.PublicKey = strings.TrimSpace(string(b))
	}
	if cfg.PublicKey != "" {
		if err = validateSetupKey(cfg.PublicKey); err != nil {
			return err
		}
		if err = validateSetupRepo(cfg.Repository); err != nil {
			return err
		}
		fmt.Println("Updatequelle:", cfg.Repository, "· signierte Updates eingerichtet")
	} else if !configureUpdates {
		fmt.Println("Updates noch nicht eingerichtet. Später: sudo anker setup --updates")
	}
	token := ""
	if keyFile != "" && configureUpdates {
		b, err := os.ReadFile(keyFile)
		if err != nil {
			return err
		}
		cfg.PublicKey = strings.TrimSpace(string(b))
		if err = validateSetupKey(cfg.PublicKey); err != nil {
			return err
		}
		cfg.Repository, err = setupPrompt("GitHub-Repository", cfg.Repository)
		if err != nil {
			return err
		}
		if err = validateSetupRepo(cfg.Repository); err != nil {
			return err
		}
		defaultAccess := "1"
		if cfg.TokenFile != "" {
			defaultAccess = "2"
		}
		access, err := setupPrompt("Repository: 1 = öffentlich, 2 = privat", defaultAccess)
		if err != nil {
			return err
		}
		if access == "2" {
			token, err = setupSecret("GitHub-Lese-Token (leer = vorhandenen behalten)")
			if err != nil {
				return err
			}
			token = strings.TrimSpace(token)
			if strings.TrimSpace(token) == "" {
				if cfg.TokenFile == "" {
					return errors.New("Token fehlt")
				}
				info, err := os.Lstat(cfg.TokenFile)
				if err != nil || !info.Mode().IsRegular() {
					return errors.New("Vorhandene Token-Datei fehlt oder ist ungültig")
				}
			} else {
				cfg.TokenFile = "/etc/anker/github-token-" + strconv.FormatInt(time.Now().UnixNano(), 10)
			}
		} else if access == "1" {
			cfg.TokenFile = ""
		} else {
			return errors.New("Repositoryzugriff 1 oder 2 wählen")
		}
	}
	fmt.Println("Webzugriff:", web.URL(), "· automatische Sicherung nach Hostanbindung")
	confirm, err := setupPrompt("Einrichtung speichern und Dienste starten? j/n", "j")
	if err != nil {
		return err
	}
	if confirm != "j" && confirm != "J" {
		return errors.New("Einrichtung abgebrochen; Konfiguration bleibt unverändert")
	}
	if err = setupUpdateIdle(); err != nil {
		return err
	}
	wasUpdaterRunning := exec.Command("systemctl", "is-active", "--quiet", "anker-updater.service").Run() == nil
	defer func() {
		if wasUpdaterRunning {
			exec.Command("systemctl", "start", "anker-updater.service").Run()
		}
	}()
	if err = setupCommand("systemctl", "stop", "anker-updater.service"); err != nil {
		return err
	}
	if err = os.MkdirAll(updater.StateDir, 0700); err != nil {
		return err
	}
	updateLock, err := os.OpenFile(filepath.Join(updater.StateDir, "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer updateLock.Close()
	if err = unix.Flock(int(updateLock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return errors.New("Updater ist noch aktiv; Einrichtung nicht verändert")
	}
	if err = setupUpdateIdle(); err != nil {
		return err
	}
	wasRunning := exec.Command("systemctl", "is-active", "--quiet", "anker.service").Run() == nil
	defer func() {
		if wasRunning {
			exec.Command("systemctl", "start", "anker.service").Run()
		}
	}()
	// Stop before offline initialization; a live instance would lock the catalog.
	if err = setupCommand("systemctl", "stop", "anker.service"); err != nil {
		return err
	}
	if err = setupInitializeAccess(admin, password); err != nil {
		return err
	}
	if web.Cert != "" {
		if err = os.MkdirAll(filepath.Dir(web.Cert), 0750); err != nil {
			return err
		}
		if err = os.Chown(filepath.Dir(web.Cert), 0, gid); err != nil {
			return err
		}
		if err = setupWrite(web.Cert, web.CertBytes, 0640, 0, gid); err != nil {
			return err
		}
		if err = setupWrite(web.Key, web.KeyBytes, 0640, 0, gid); err != nil {
			return err
		}
	}
	text, err := web.env()
	if err != nil {
		return err
	}
	if err = setupWrite("/etc/anker/service.env", []byte(text), 0640, 0, gid); err != nil {
		return err
	}
	if keyFile != "" {
		if token != "" {
			if err = setupWrite(cfg.TokenFile, []byte(token+"\n"), 0600, 0, 0); err != nil {
				return err
			}
		}
		b, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return err
		}
		if err = setupWrite(updater.ConfigPath, append(b, '\n'), 0640, 0, gid); err != nil {
			return err
		}
	}
	for _, name := range []string{"backup", "restore"} {
		path := "/etc/anker/keys/" + name
		if err = setupSSHKey(path, uid, gid); err != nil {
			return err
		}
	}
	if err = setupCommand("systemctl", "enable", "--now", "anker.service"); err != nil {
		return err
	}
	if err = setupReady(filepath.Join(updater.DataDir, "anker.sock"), "/api/update-control"); err != nil {
		return err
	}
	if err = updateLock.Close(); err != nil {
		return err
	}
	if _, err = os.Stat(updater.ConfigPath); err == nil {
		if err = setupCommand("systemctl", "enable", "--now", "anker-updater.service"); err != nil {
			return err
		}
		if err = setupCommand("systemctl", "restart", "anker-updater.service"); err != nil {
			return err
		}
		if err = setupReady(updater.Socket, "/status"); err != nil {
			return err
		}
	}
	if web.Cert == "" {
		fmt.Println("Bereit. SSH-Tunnel: ssh -N -L 8087:127.0.0.1:8087 BENUTZER@ANKER-SERVER\nDann http://127.0.0.1:8087 öffnen.")
	} else {
		fmt.Println("Bereit:", web.URL())
		leaf, err := setupTLSLeaf(web.CertBytes, web.KeyBytes)
		if err != nil {
			return err
		}
		fmt.Printf("TLS-Fingerprint SHA256: %X\nZertifikat gültig bis %s · Datei: %s\n", sha256.Sum256(leaf.Raw), leaf.NotAfter.UTC().Format("02.01.2006"), web.Cert)
		if web.SelfSigned {
			fmt.Println("Keine interne CA erforderlich. Den Fingerprint vor Bestätigen der Browserwarnung auf jedem Arbeitsplatz vergleichen. Ein bereits vertrautes Zertifikat lässt sich optional importieren. Erneuern über sudo anker setup vor dem Ablaufdatum.")
		}
	}
	fmt.Println("SSH-Schlüssel: /etc/anker/keys/backup und restore\nÖffentliche Schlüssel: gleiche Pfade mit .pub. Host-Anbindung: README → Proxmox-Hosts anbinden.\nEinrichtung erneut öffnen: sudo anker setup")
	return nil
}
