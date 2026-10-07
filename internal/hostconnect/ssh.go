package hostconnect

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	hostassets "anker/host"
	hostscripts "anker/scripts"
	"golang.org/x/crypto/ssh"
)

var addressPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]*$`)
var passwordPromptPattern = regexp.MustCompile(`(?i)^(?:(?:[a-z0-9_.@-]+(?:'s)? )?(?:password|passwort)|(?:password|passwort) for [a-z0-9_.@-]+):$`)
var usernamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.-]{0,63}\$?$`)

func validate(r InspectRequest) (InspectRequest, error) {
	if len(r.Address) == 0 || len(r.Address) > 253 || (net.ParseIP(r.Address) == nil && (!addressPattern.MatchString(r.Address) || strings.Contains(r.Address, ".."))) {
		return r, errors.New("Ungültige SSH-Hostadresse")
	}
	if r.Port == 0 {
		r.Port = 22
	}
	if r.Port < 1 || r.Port > 65535 {
		return r, errors.New("SSH-Port muss zwischen 1 und 65535 liegen")
	}
	return r, nil
}
func endpoint(r InspectRequest) string { return net.JoinHostPort(r.Address, strconv.Itoa(r.Port)) }
func phaseError(phase string, ctx context.Context) error {
	if ctx.Err() != nil {
		return errors.New("Hostanbindung: " + phase + " abgebrochen oder Zeitlimit erreicht")
	}
	return errors.New("Hostanbindung: " + phase + " fehlgeschlagen")
}

// Retain the raw connection and close it on cancellation, including during the
// handshake. ClientConfig.Timeout alone does not bound active SSH sessions.
func connect(ctx context.Context, r InspectRequest, cfg *ssh.ClientConfig) (*ssh.Client, error) {
	c, e := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", endpoint(r))
	if e != nil {
		return nil, e
	}
	stop := context.AfterFunc(ctx, func() { c.Close() })
	ok := false
	defer func() {
		if !ok {
			stop()
			c.Close()
		}
	}()
	deadline := time.Now().Add(15 * time.Second)
	if d, has := ctx.Deadline(); has && d.Before(deadline) {
		deadline = d
	}
	c.SetDeadline(deadline)
	conn, chans, reqs, e := ssh.NewClientConn(c, endpoint(r), cfg)
	if e != nil {
		return nil, e
	}
	c.SetDeadline(time.Time{})
	ok = true
	client := ssh.NewClient(conn, chans, reqs)
	go func() { <-ctx.Done(); client.Close(); stop() }()
	return client, nil
}
func clientConfig(user string, auth []ssh.AuthMethod, callback ssh.HostKeyCallback) *ssh.ClientConfig {
	return &ssh.ClientConfig{User: user, Auth: auth, HostKeyCallback: callback, HostKeyAlgorithms: []string{ssh.KeyAlgoED25519}, Timeout: 10 * time.Second}
}
func (m Manager) Inspect(ctx context.Context, request InspectRequest) (Identity, error) {
	r, e := validate(request)
	if e != nil {
		return Identity{}, e
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	dir, e := m.openDir()
	if e != nil {
		return Identity{}, phaseError("SSH-Konfiguration", ctx)
	}
	defer dir.Close()
	trust, e := m.readTrust(dir)
	if e != nil {
		return Identity{}, phaseError("SSH-Vertrauen", ctx)
	}
	var key ssh.PublicKey
	seen := errors.New("identity captured")
	_, dialError := connect(ctx, r, clientConfig("anker-inspect", nil, func(_ string, _ net.Addr, k ssh.PublicKey) error { key = k; return seen }))
	if key == nil || !errors.Is(dialError, seen) {
		return Identity{}, phaseError("Identitätsprüfung", ctx)
	}
	known, changed, e := identityState(trust, endpoint(r), key)
	if e != nil {
		return Identity{}, phaseError("SSH-Vertrauen", ctx)
	}
	return Identity{Address: r.Address, Port: r.Port, Fingerprint: ssh.FingerprintSHA256(key), KeyType: key.Type(), Known: known, Changed: changed}, nil
}
func passwordMethods(password string) []ssh.AuthMethod {
	rounds := 0
	return []ssh.AuthMethod{ssh.Password(password), ssh.KeyboardInteractive(func(user, instruction string, questions []string, echo []bool) ([]string, error) {
		rounds++
		if rounds > 3 || len(questions) != 1 || len(echo) != 1 || echo[0] || len(questions[0]) > 256 {
			return nil, errors.New("unsupported authentication")
		}
		prompt := strings.ToLower(strings.TrimSpace(questions[0]))
		if !passwordPromptPattern.MatchString(prompt) {
			return nil, errors.New("unsupported authentication")
		}
		return []string{password}, nil
	})}
}
func (m Manager) Enroll(ctx context.Context, request EnrollRequest) (Enrollment, error) {
	r, e := validate(InspectRequest{Address: request.Address, Port: request.Port})
	if e != nil {
		return Enrollment{}, e
	}
	if !request.Confirmed {
		return Enrollment{}, errors.New("SSH-Fingerprint ausdrücklich bestätigen")
	}
	decoded, e := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(request.Fingerprint, "SHA256:"))
	if e != nil || len(decoded) != 32 || !strings.HasPrefix(request.Fingerprint, "SHA256:") {
		return Enrollment{}, errors.New("Ungültiger SSH-Fingerprint")
	}
	if request.Username == "" {
		request.Username = "root"
	}
	if !usernamePattern.MatchString(request.Username) || len(request.Password) == 0 || len(request.Password) > 4096 || strings.ContainsAny(request.Password, "\x00\r\n") {
		return Enrollment{}, errors.New("Ungültige SSH-Zugangsdaten")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	dir, e := m.openDir()
	if e != nil {
		return Enrollment{}, phaseError("SSH-Konfiguration", ctx)
	}
	defer dir.Close()
	backup, restore, e := m.keys(dir)
	if e != nil {
		return Enrollment{}, errors.New("Hostanbindung: geschützte getrennte Ed25519-Rollenschlüssel fehlen oder sind unsicher")
	}
	trust, e := m.readTrust(dir)
	if e != nil {
		return Enrollment{}, phaseError("SSH-Vertrauen", ctx)
	}
	var key ssh.PublicKey
	callback := func(_ string, _ net.Addr, k ssh.PublicKey) error {
		if ssh.FingerprintSHA256(k) != request.Fingerprint {
			return errors.New("SSH-Hostschlüssel stimmt nicht mit Bestätigung überein")
		}
		_, changed, e := identityState(trust, endpoint(r), k)
		if e != nil {
			return e
		}
		if changed {
			return errors.New("Bekannter SSH-Hostschlüssel wurde geändert")
		}
		key = k
		return nil
	}
	client, e := connect(ctx, r, clientConfig(request.Username, passwordMethods(request.Password), callback))
	if e != nil {
		if key == nil {
			return Enrollment{}, phaseError("SSH-Identität", ctx)
		}
		return Enrollment{}, phaseError("SSH-Anmeldung", ctx)
	}
	defer client.Close()
	if _, e = run(client, preflightCommand, nil, 8192); e != nil {
		return Enrollment{}, phaseError("Proxmox-Prüfung", ctx)
	}
	uid, e := run(client, "id -u", nil, 64)
	if e != nil {
		return Enrollment{}, phaseError("Administratorrechte", ctx)
	}
	payload, e := bundle(backup.PublicKey(), restore.PublicKey())
	if e != nil {
		return Enrollment{}, phaseError("Installationspaket", ctx)
	}
	command := rootInstallCommand
	input := payload
	if strings.TrimSpace(string(uid)) != "0" {
		command = sudoInstallCommand
		input = append(append([]byte{}, []byte(request.Password+"\n")...), payload...)
	}
	if _, e = run(client, command, input, 32768); e != nil {
		return Enrollment{}, phaseError("Hostinstallation oder sudo-Rechte", ctx)
	}
	pinned := func(_ string, _ net.Addr, k ssh.PublicKey) error {
		if !bytes.Equal(k.Marshal(), key.Marshal()) {
			return errors.New("SSH-Hostschlüssel wurde geändert")
		}
		return nil
	}
	var inventory json.RawMessage
	for _, role := range []struct {
		user string
		key  ssh.Signer
	}{{"anker", backup}, {"anker-restore", restore}} {
		c, e := connect(ctx, r, clientConfig(role.user, []ssh.AuthMethod{ssh.PublicKeys(role.key)}, pinned))
		if e != nil {
			return Enrollment{}, phaseError("Schlüsselprüfung "+role.user, ctx)
		}
		out, e := run(c, "anker-host", []byte("{\"version\":1,\"operation\":\"probe\"}\n"), 16<<20)
		c.Close()
		if e != nil {
			return Enrollment{}, phaseError("Schlüsselprüfung "+role.user, ctx)
		}
		var decodedInventory any
		if json.Unmarshal(out, &decodedInventory) != nil || containsSecret(decodedInventory, request.Password) {
			return Enrollment{}, phaseError("Inventarprüfung "+role.user, ctx)
		}
		var object map[string]json.RawMessage
		if json.Unmarshal(out, &object) != nil || object == nil {
			return Enrollment{}, phaseError("Inventarprüfung "+role.user, ctx)
		}
		var version string
		if json.Unmarshal(object["pve_version"], &version) != nil || strings.TrimSpace(version) == "" {
			return Enrollment{}, phaseError("Proxmox-Inventar "+role.user, ctx)
		}
		if role.user == "anker" {
			inventory = append(json.RawMessage(nil), bytes.TrimSpace(out)...)
		}
	}
	if e = m.saveTrust(dir, endpoint(r), key); e != nil {
		return Enrollment{}, phaseError("SSH-Vertrauen speichern", ctx)
	}
	return Enrollment{Address: r.Address, Port: r.Port, Fingerprint: request.Fingerprint, BackupUser: "anker", RestoreUser: "anker-restore", BackupKeyPath: m.directory() + "/keys/backup", RestoreKeyPath: m.directory() + "/keys/restore", KnownHostsPath: m.directory() + "/known_hosts", Inventory: inventory}, nil
}

// A compromised authenticated endpoint may echo the one-time credential into
// an otherwise valid inventory. Do not hand that credential to catalog storage.
func containsSecret(value any, secret string) bool {
	switch v := value.(type) {
	case string:
		return strings.Contains(v, secret)
	case []any:
		for _, item := range v {
			if containsSecret(item, secret) {
				return true
			}
		}
	case map[string]any:
		for key, item := range v {
			if strings.Contains(key, secret) || containsSecret(item, secret) {
				return true
			}
		}
	}
	return false
}

// boundedOutput rejects untrusted output above a phase-specific ceiling. No raw
// stdout, stderr, banners, or SSH errors are used in messages returned to callers.
type boundedOutput struct {
	buffer    bytes.Buffer
	remaining int
	overflow  bool
	abort     func()
}

func (w *boundedOutput) Write(b []byte) (int, error) {
	if len(b) > w.remaining {
		w.overflow = true
		if w.abort != nil {
			w.abort()
		}
		return 0, errors.New("output limit")
	}
	w.remaining -= len(b)
	return w.buffer.Write(b)
}
func run(c *ssh.Client, command string, input []byte, limit int) ([]byte, error) {
	s, e := c.NewSession()
	if e != nil {
		return nil, e
	}
	defer s.Close()
	out := &boundedOutput{remaining: limit, abort: func() { c.Close() }}
	stderr := &boundedOutput{remaining: 32768, abort: func() { c.Close() }}
	s.Stdout = out
	s.Stderr = stderr
	s.Stdin = bytes.NewReader(input)
	e = s.Run(command)
	if e != nil || out.overflow || stderr.overflow {
		return nil, errors.New("SSH command failed")
	}
	return out.buffer.Bytes(), nil
}
func bundle(backup, restore ssh.PublicKey) ([]byte, error) {
	var b bytes.Buffer
	t := tar.NewWriter(&b)
	for _, file := range []struct {
		name string
		data []byte
		mode int64
	}{{"scripts/install-host.sh", hostscripts.Installer, 0700}, {"scripts/host-authorize.py", hostscripts.Authorizer, 0600}, {"host/anker_host.py", hostassets.Helper, 0600}, {"keys/backup.pub", ssh.MarshalAuthorizedKey(backup), 0600}, {"keys/restore.pub", ssh.MarshalAuthorizedKey(restore), 0600}} {
		if e := t.WriteHeader(&tar.Header{Name: file.name, Mode: file.mode, Size: int64(len(file.data)), Typeflag: tar.TypeReg}); e != nil {
			return nil, e
		}
		if _, e := io.Copy(t, bytes.NewReader(file.data)); e != nil {
			return nil, e
		}
	}
	if e := t.Close(); e != nil {
		return nil, e
	}
	return b.Bytes(), nil
}

const preflightCommand = "command -v pveversion >/dev/null 2>&1 && pveversion >/dev/null 2>&1"
const rootScript = `set -eu
[ "$(id -u)" -eq 0 ]
command -v pveversion >/dev/null 2>&1
pveversion >/dev/null 2>&1
if ! command -v sudo >/dev/null 2>&1 || ! command -v visudo >/dev/null 2>&1; then
 command -v apt-get >/dev/null 2>&1
 if ! DEBIAN_FRONTEND=noninteractive apt-get -o DPkg::Lock::Timeout=60 -o Acquire::Retries=2 install -y sudo </dev/null; then
  apt-get -o DPkg::Lock::Timeout=60 -o Acquire::Retries=2 update </dev/null
  DEBIAN_FRONTEND=noninteractive apt-get -o DPkg::Lock::Timeout=60 -o Acquire::Retries=2 install -y sudo </dev/null
 fi
 command -v sudo >/dev/null 2>&1
 command -v visudo >/dev/null 2>&1
fi
umask 077
anker_tmp=$(mktemp -d /tmp/anker-enroll.XXXXXXXXXX)
trap 'rm -rf -- "$anker_tmp"' EXIT
trap 'exit 1' HUP INT TERM
tar -xf - -C "$anker_tmp"
sh "$anker_tmp/scripts/install-host.sh" --backup-key "$anker_tmp/keys/backup.pub" --restore-key "$anker_tmp/keys/restore.pub"
`

var rootInstallCommand = "sh -c " + shellQuote(rootScript)
var sudoInstallCommand = "sh -c " + shellQuote(`set -eu
IFS= read -r anker_password
printf '%s\n' "$anker_password" | sudo -S -p '' -v
unset anker_password
sudo -n sh -c '[ "$(id -u)" -eq 0 ]'
sudo -n sh -c `+shellQuote(rootScript)+`
exit "$?"
`)

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
