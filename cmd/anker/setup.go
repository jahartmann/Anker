package main

import (
	"anker/internal/anker"
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
	fmt.Print(newSetupDisplay().Prompt(label, fallback))
	return setupAnswer(os.Stdin, fallback)
}
func setupSecret(label string) (string, error) {
	if !term.IsTerminal(os.Stdin.Fd()) {
		return "", errors.New("Verdeckte Eingabe benötigt ein Terminal")
	}
	fmt.Print(newSetupDisplay().Prompt(label, ""))
	b, err := term.ReadPassword(os.Stdin.Fd())
	fmt.Println()
	return string(b), err
}
func initialPassword(minimum int) (string, error) {
	for {
		first, err := setupSecret(fmt.Sprintf("Administratorpasswort (mindestens %d Zeichen)", minimum))
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
		if err := anker.ValidatePassword(first, minimum); err != nil {
			fmt.Println(err)
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
	if state.Status == "installing" || state.Busy {
		return errors.New("Eine Systemänderung läuft; Einrichtung erst nach deren Abschluss öffnen")
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
func runSetup(configureUpdates bool) (result error) {
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
	display := newSetupDisplay()
	display.Header()
	if _, err = setupRecoverPending("/etc/anker"); err != nil {
		return err
	}
	display.Section("01", "Administratorzugang")
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
		minimum, policyErr := setupPasswordMinimum(filepath.Join(updater.DataDir, "catalog.db"))
		if policyErr != nil {
			return policyErr
		}
		password, err = initialPassword(minimum)
		if err != nil {
			return err
		}
	} else {
		display.Note("Administratorzugang ist bereits eingerichtet. Bestehende Benutzer bleiben erhalten.")
	}
	display.Section("02", "Verbindung")
	priorEnv, err := os.ReadFile("/etc/anker/service.env")
	if err != nil {
		return err
	}
	policy, err := updater.LoadTLSPolicy("/etc/anker")
	if err != nil {
		return err
	}
	var web setupWebConfig
	reuse := false
	if initialized {
		display.Note("Benutzer, Sicherungen und SSH-Schlüssel bleiben bei beiden Optionen erhalten.")
		display.Fact("1 · Fortsetzen", "Vorhandene Einstellungen übernehmen")
		display.Fact("2 · Neu konfigurieren", "Verbindung erneut einrichten")
		choice, choiceErr := setupPrompt("Einrichtung", "1")
		if choiceErr != nil {
			return choiceErr
		}
		if choice != "1" && choice != "2" {
			return errors.New("Einrichtung 1 oder 2 wählen")
		}
		if choice == "1" {
			_, completeErr := os.Stat("/etc/anker/setup-complete")
			_, backupErr := os.Stat("/etc/anker/keys/backup")
			_, restoreErr := os.Stat("/etc/anker/keys/restore")
			_, startedErr := os.Stat("/etc/anker/setup-started")
			configured := completeErr == nil || setupEnvValue(string(priorEnv), "ANKER_TLS_CERT") != "" || os.IsNotExist(startedErr) && backupErr == nil && restoreErr == nil
			if configured {
				web, err = setupExistingWeb(string(priorEnv), policy)
				reuse = err == nil
			}
			if !reuse {
				display.Note("Verbindung fehlt oder ist nicht mehr gültig. Jetzt vervollständigen.")
			}
		}
	}
	if !reuse {
		web, err = setupConfigureWeb(string(priorEnv), initialized)
		if err != nil {
			return err
		}
	}
	cfg := updater.Config{Repository: "jahartmann/Anker"}
	if b, readErr := os.ReadFile(updater.ConfigPath); readErr == nil {
		if err = json.Unmarshal(b, &cfg); err != nil {
			return err
		}
	} else if !os.IsNotExist(readErr) {
		return readErr
	}
	defaultKey := ""
	if _, err = os.Stat("/etc/anker/release-public.key"); err == nil {
		defaultKey = "/etc/anker/release-public.key"
	}
	keyFile := ""
	if configureUpdates {
		display.Note("Signierte Updates · Repository und öffentlicher Schlüssel")
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
	display.Section("03", "Prüfen & speichern")
	if initialized {
		display.Fact("Benutzer", "Vorhandene Zugänge")
	} else {
		display.Fact("Administrator", admin)
	}
	display.Fact("Webzugriff", web.URL())
	if web.Cert == "" {
		display.Fact("Verbindung", "SSH-Tunnel · nur lokal erreichbar")
	} else if web.Managed {
		display.Fact("Zertifikat", "Selbst erzeugt · von Anker verwaltet")
	} else {
		display.Fact("Zertifikat", "Eigene Zertifikatsdateien")
	}
	display.Fact("Datenordner", updater.DataDir)
	if cfg.PublicKey != "" {
		display.Fact("Updates", cfg.Repository+" · signierte Releases")
	} else {
		display.Fact("Updates", "Noch nicht eingerichtet · später: anker setup --updates")
	}
	fmt.Println()
	confirm, err := setupPrompt("Einrichtung speichern und Dienste starten? j/n", "j")
	if err != nil {
		return err
	}
	if confirm != "j" && confirm != "J" {
		return errors.New("Einrichtung abgebrochen; Konfiguration bleibt unverändert")
	}
	fmt.Println()
	display.Note("Einrichtung wird gespeichert. Die Dienste werden kurz angehalten.")
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
	updateLockReleased := false
	defer func() { updateLock.Close() }()
	if err = unix.Flock(int(updateLock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return errors.New("Updater ist noch aktiv; Einrichtung nicht verändert")
	}
	if err = setupUpdateIdle(); err != nil {
		return err
	}
	wasRunning := exec.Command("systemctl", "is-active", "--quiet", "anker.service").Run() == nil
	if err = setupMigrateUpdaterUnit(); err != nil {
		return err
	}
	defer func() {
		if wasRunning {
			exec.Command("systemctl", "start", "anker.service").Run()
		}
	}()
	// Stop before offline initialization; a live instance would lock the catalog.
	if err = setupCommand("systemctl", "stop", "anker.service"); err != nil {
		return err
	}
	var created []string
	if web.Cert != "" && web.Cert != setupEnvValue(string(priorEnv), "ANKER_TLS_CERT") {
		created = append(created, web.Cert, web.Key)
	}
	if token != "" {
		created = append(created, cfg.TokenFile)
	}
	if _, completeErr := os.Stat("/etc/anker/setup-complete"); os.IsNotExist(completeErr) {
		if err = setupWrite("/etc/anker/setup-started", []byte("1\n"), 0600, 0, 0); err != nil {
			return err
		}
	}
	change, err := setupBeginChange("/etc/anker", created, wasRunning, wasUpdaterRunning)
	if err != nil {
		return err
	}
	defer func() {
		if change == nil {
			return
		}
		if updateLockReleased {
			if stopErr := setupCommand("systemctl", "stop", "anker-updater.service"); stopErr != nil {
				wasRunning, wasUpdaterRunning = false, false
				result = errors.Join(result, stopErr)
				return
			}
			lock, lockErr := os.OpenFile(filepath.Join(updater.StateDir, "lock"), os.O_RDWR, 0600)
			if lockErr == nil {
				lockErr = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB)
				defer lock.Close()
			}
			if lockErr != nil {
				wasRunning, wasUpdaterRunning = false, false
				result = errors.Join(result, fmt.Errorf("Wiederherstellung gesperrt; anker setup erneut ausführen: %w", lockErr))
				return
			}
		}
		if stopErr := setupCommand("systemctl", "stop", "anker.service"); stopErr != nil {
			wasRunning, wasUpdaterRunning = false, false
			result = errors.Join(result, stopErr)
			return
		}
		if rollbackErr := change.rollback("/etc/anker"); rollbackErr != nil {
			wasRunning, wasUpdaterRunning = false, false
			result = errors.Join(result, fmt.Errorf("Konfiguration nicht wiederhergestellt; anker setup erneut ausführen: %w", rollbackErr))
		} else {
			display.Note("Letzter Konfigurationsstand wiederhergestellt. Einrichtung erneut mit anker setup starten.")
		}
	}()
	if err = setupInitializeAccess(admin, password); err != nil {
		return err
	}
	for _, name := range []string{"backup", "restore"} {
		path := "/etc/anker/keys/" + name
		if err = setupSSHKey(path, uid, gid); err != nil {
			return err
		}
	}
	if web.Cert != "" && web.Cert != setupEnvValue(string(priorEnv), "ANKER_TLS_CERT") {
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
	text := string(priorEnv)
	if !reuse {
		newEnv, envErr := web.env()
		if envErr != nil {
			return envErr
		}
		text = setupMergeWebEnv(string(priorEnv), newEnv)
	}
	if web.Managed {
		if policy.ManagedCert == "" {
			policy.Automatic = true
		}
		if policy.ManagedCert != web.Cert {
			policy.LastRenewedAt = ""
		}
		leaf, err := setupTLSLeaf(web.CertBytes, web.KeyBytes)
		if err != nil {
			return err
		}
		hash := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
		policy.ManagedCert, policy.ManagedPublicKey = web.Cert, fmt.Sprintf("%X", hash)
	} else {
		policy.Automatic = false
		policy.ManagedCert, policy.ManagedPublicKey, policy.LastRenewedAt = "", "", ""
	}
	policyBytes, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	if err = setupWrite(updater.TLSConfigPath, append(policyBytes, '\n'), 0600, 0, 0); err != nil {
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
	if err = setupWrite("/etc/anker/service.env", []byte(text), 0640, 0, gid); err != nil {
		return err
	}
	if err = setupWrite("/etc/anker/setup-complete", []byte("1\n"), 0600, 0, 0); err != nil {
		return err
	}
	if err = setupCommand("systemctl", "enable", "--now", "anker.service"); err != nil {
		return err
	}
	if err = display.Wait("Webdienst", func() error {
		return setupReady(filepath.Join(updater.DataDir, "anker.sock"), "/api/update-control")
	}); err != nil {
		return err
	}
	if err = updateLock.Close(); err != nil {
		return err
	}
	updateLockReleased = true
	{
		if err = setupCommand("systemctl", "enable", "--now", "anker-updater.service"); err != nil {
			return err
		}
		if err = setupCommand("systemctl", "restart", "anker-updater.service"); err != nil {
			return err
		}
		if err = display.Wait("Systemdienst", func() error { return setupReady(updater.Socket, "/status") }); err != nil {
			return err
		}
	}
	if err = change.commit("/etc/anker"); err != nil {
		return err
	}
	change = nil
	fmt.Println("\n" + display.style("1;32", "✓ Einrichtung abgeschlossen"))
	fmt.Println(display.style("1", web.URL()))
	if web.Cert == "" {
		display.Note("SSH-Tunnel vom Arbeitsplatz öffnen:")
		fmt.Println(setupTunnelCommand(web))
	} else {
		leaf, err := setupTLSLeaf(web.CertBytes, web.KeyBytes)
		if err != nil {
			return err
		}
		fmt.Println()
		display.Note("TLS-Fingerprint · SHA256")
		fmt.Printf("%X\n", sha256.Sum256(leaf.Raw))
		display.Note("Zertifikat gültig bis " + leaf.NotAfter.UTC().Format("02.01.2006"))
		if web.SelfSigned {
			display.Note("Vor Bestätigen der Browserwarnung den Fingerprint vergleichen.")
			display.Note("Erneuerung: Einstellungen → System → Webzertifikat.")
			display.Note("Nach Erneuerung kann eine neue Browserfreigabe nötig sein.")
		}
	}
	display.Section("→", "Hosts anbinden")
	display.Fact("Backup-Schlüssel", "/etc/anker/keys/backup.pub")
	display.Fact("Restore-Schlüssel", "/etc/anker/keys/restore.pub")
	display.Note("Anleitung: README → Proxmox-Hosts anbinden.")
	display.Note("Einrichtung erneut öffnen: anker setup")
	return nil
}
