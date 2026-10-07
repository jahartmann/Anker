package hostconnect

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/sys/unix"
)

type sshFixture struct {
	t               *testing.T
	listener        net.Listener
	key             ssh.Signer
	mu              sync.Mutex
	passwords       int
	commands        []string
	uploads         [][]byte
	failProbe       string
	failInstall     bool
	block           bool
	roleKeys        map[string]ssh.PublicKey
	uid             string
	keyboardOnly    bool
	unsafePrompt    bool
	keyboardAnswers int
	flood           bool
}

func signer(t *testing.T) ssh.Signer {
	t.Helper()
	_, k, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	s, e := ssh.NewSignerFromKey(k)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func fixture(t *testing.T) *sshFixture {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	f := &sshFixture{t: t, listener: l, key: signer(t), roleKeys: map[string]ssh.PublicKey{}}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			go f.serve(c)
		}
	}()
	return f
}
func (f *sshFixture) serve(c net.Conn) {
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	f.mu.Lock()
	keyboardOnly, unsafePrompt, uid, flood, hostkey := f.keyboardOnly, f.unsafePrompt, f.uid, f.flood, f.key
	f.mu.Unlock()
	cfg := &ssh.ServerConfig{PasswordCallback: func(c ssh.ConnMetadata, p []byte) (*ssh.Permissions, error) {
		f.mu.Lock()
		f.passwords++
		f.mu.Unlock()
		if keyboardOnly || string(p) != "top-secret" {
			return nil, errors.New("malicious top-secret password error")
		}
		return nil, nil
	}, PublicKeyCallback: func(c ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
		f.mu.Lock()
		want := f.roleKeys[c.User()]
		f.mu.Unlock()
		if want == nil || !bytes.Equal(want.Marshal(), k.Marshal()) {
			return nil, errors.New("wrong key")
		}
		return nil, nil
	}}
	if keyboardOnly {
		cfg.KeyboardInteractiveCallback = func(c ssh.ConnMetadata, challenge ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
			prompt := "Password: "
			if unsafePrompt {
				prompt = "Password and token: "
			}
			answers, e := challenge("Login", "", []string{prompt}, []bool{false})
			if e != nil {
				return nil, e
			}
			f.mu.Lock()
			f.keyboardAnswers += len(answers)
			f.mu.Unlock()
			if len(answers) != 1 || answers[0] != "top-secret" {
				return nil, errors.New("bad PAM response")
			}
			return nil, nil
		}
	}
	cfg.AddHostKey(hostkey)
	conn, chans, reqs, e := ssh.NewServerConn(c, cfg)
	if e != nil {
		return
	}
	defer conn.Close()
	go ssh.DiscardRequests(reqs)
	for n := range chans {
		if n.ChannelType() != "session" {
			n.Reject(ssh.UnknownChannelType, "unsupported")
			continue
		}
		ch, reqs, e := n.Accept()
		if e != nil {
			continue
		}
		go func() {
			defer ch.Close()
			for r := range reqs {
				if r.Type != "exec" {
					r.Reply(false, nil)
					continue
				}
				var cmd struct{ Command string }
				ssh.Unmarshal(r.Payload, &cmd)
				r.Reply(true, nil)
				f.mu.Lock()
				f.commands = append(f.commands, cmd.Command)
				block, failInstall, failProbe := f.block, f.failInstall, f.failProbe
				f.mu.Unlock()
				if block {
					for range reqs {
					}
					return
				}
				status := uint32(0)
				switch {
				case cmd.Command == "id -u":
					if uid != "" {
						io.WriteString(ch, uid+"\n")
					} else {
						io.WriteString(ch, "0\n")
					}
				case strings.Contains(cmd.Command, "pveversion") && !strings.Contains(cmd.Command, "mktemp"):
					io.WriteString(ch, "pve-manager/9.0\n")
				case strings.Contains(cmd.Command, "mktemp"):
					b, _ := io.ReadAll(ch)
					f.mu.Lock()
					f.uploads = append(f.uploads, b)
					f.mu.Unlock()
					if failInstall {
						io.WriteString(ch.Stderr(), "top-secret MALICIOUS BANNER")
						status = 1
					}
				default:
					if flood {
						for {
							if _, e := io.WriteString(ch, strings.Repeat("x", 32768)); e != nil {
								return
							}
						}
					}
					b, _ := io.ReadAll(ch)
					if !bytes.Contains(b, []byte(`"operation":"probe"`)) {
						status = 1
					}
					if failProbe == "secret" {
						io.WriteString(ch, `{"pve_version":"9.0","details":{"password":"top-secret"}}`)
					} else if conn.User() == failProbe {
						io.WriteString(ch, "{invalid")
					} else {
						io.WriteString(ch, `{"hostname":"pve","pve_version":"9.0","interfaces":[],"disks":[],"details":{}}`)
					}
				}
				ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
				return
			}
		}()
	}
}
func (f *sshFixture) request() InspectRequest {
	host, port, _ := net.SplitHostPort(f.listener.Addr().String())
	p, _ := strconv.Atoi(port)
	return InspectRequest{Address: host, Port: p}
}
func managerFixture(t *testing.T, f *sshFixture) Manager {
	t.Helper()
	d := t.TempDir()
	d, _ = filepath.EvalSymlinks(d)
	os.Chmod(d, 0750)
	os.Mkdir(filepath.Join(d, "keys"), 0750)
	for role, user := range map[string]string{"backup": "anker", "restore": "anker-restore"} {
		_, k, _ := ed25519.GenerateKey(rand.Reader)
		p, e := ssh.MarshalPrivateKey(k, "")
		if e != nil {
			t.Fatal(e)
		}
		os.WriteFile(filepath.Join(d, "keys", role), pem.EncodeToMemory(p), 0640)
		s, _ := ssh.NewSignerFromKey(k)
		f.roleKeys[user] = s.PublicKey()
	}
	return Manager{Dir: d, OwnerUID: os.Getuid(), GroupGID: os.Getgid()}
}
func enrollmentRequest(f *sshFixture) EnrollRequest {
	r := f.request()
	return EnrollRequest{Address: r.Address, Port: r.Port, Username: "root", Password: "top-secret", Fingerprint: ssh.FingerprintSHA256(f.key.PublicKey()), Confirmed: true}
}
func TestInspectNeverAuthenticatesAndMarksChanged(t *testing.T) {
	f := fixture(t)
	m := managerFixture(t, f)
	id, e := m.Inspect(context.Background(), f.request())
	if e != nil || id.Fingerprint != ssh.FingerprintSHA256(f.key.PublicKey()) || id.Known || id.Changed {
		t.Fatalf("identity: %+v %v", id, e)
	}
	f.mu.Lock()
	n := f.passwords
	f.mu.Unlock()
	if n != 0 {
		t.Fatal("Inspect authenticated")
	}
	other := signer(t)
	os.WriteFile(filepath.Join(m.Dir, "known_hosts"), []byte(knownhosts.Line([]string{f.listener.Addr().String()}, other.PublicKey())+"\n"), 0640)
	id, e = m.Inspect(context.Background(), f.request())
	if e != nil || !id.Known || !id.Changed {
		t.Fatalf("changed identity: %+v %v", id, e)
	}
}
func TestEnrollRejectsUnconfirmedPinAndKnownSwapBeforePassword(t *testing.T) {
	for _, kind := range []string{"unconfirmed", "pin", "known"} {
		t.Run(kind, func(t *testing.T) {
			f := fixture(t)
			m := managerFixture(t, f)
			r := enrollmentRequest(f)
			switch kind {
			case "unconfirmed":
				r.Confirmed = false
			case "pin":
				r.Fingerprint = ssh.FingerprintSHA256(signer(t).PublicKey())
			case "known":
				os.WriteFile(filepath.Join(m.Dir, "known_hosts"), []byte(knownhosts.Line([]string{f.listener.Addr().String()}, signer(t).PublicKey())+"\n"), 0640)
			}
			_, e := m.Enroll(context.Background(), r)
			if e == nil {
				t.Fatal("untrusted enrollment accepted")
			}
			f.mu.Lock()
			n := f.passwords
			f.mu.Unlock()
			if n != 0 {
				t.Fatal("password sent before trust")
			}
		})
	}
}
func TestEnrollOnlyUploadsPublicKeysAndVerifiesBothRoles(t *testing.T) {
	f := fixture(t)
	m := managerFixture(t, f)
	r, e := m.Enroll(context.Background(), enrollmentRequest(f))
	if e != nil {
		t.Fatal(e)
	}
	if r.BackupUser != "anker" || r.RestoreUser != "anker-restore" || r.BackupKeyPath != filepath.Join(m.Dir, "keys", "backup") || !bytes.Contains(r.Inventory, []byte(`"pve_version":"9.0"`)) {
		t.Fatalf("result %+v", r)
	}
	f.mu.Lock()
	uploads := append([][]byte{}, f.uploads...)
	f.mu.Unlock()
	if len(uploads) != 1 {
		t.Fatalf("uploads %d", len(uploads))
	}
	tr := tar.NewReader(bytes.NewReader(uploads[0]))
	got := map[string]bool{}
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		b, _ := io.ReadAll(tr)
		if bytes.Contains(b, []byte("PRIVATE KEY")) {
			t.Fatal("uploaded private key")
		}
		got[h.Name] = true
	}
	for _, name := range []string{"scripts/install-host.sh", "scripts/host-authorize.py", "host/anker_host.py", "keys/backup.pub", "keys/restore.pub"} {
		if !got[name] {
			t.Fatalf("missing %s", name)
		}
	}
	if len(got) != 5 {
		t.Fatalf("unexpected files %v", got)
	}
	if _, e := m.Enroll(context.Background(), enrollmentRequest(f)); e != nil {
		t.Fatalf("repeat: %v", e)
	}
}
func TestFailedEnrollmentNeverStoresTrustAndRedactsRemoteOutput(t *testing.T) {
	for _, kind := range []string{"password", "install", "backup", "restore"} {
		t.Run(kind, func(t *testing.T) {
			f := fixture(t)
			m := managerFixture(t, f)
			r := enrollmentRequest(f)
			switch kind {
			case "password":
				r.Password = "wrong"
			case "install":
				f.failInstall = true
			case "backup":
				f.failProbe = "anker"
			case "restore":
				f.failProbe = "anker-restore"
			}
			_, e := m.Enroll(context.Background(), r)
			if e == nil {
				t.Fatal("failure accepted")
			}
			if strings.Contains(e.Error(), "top-secret") || strings.Contains(e.Error(), "MALICIOUS") {
				t.Fatalf("leaked %v", e)
			}
			if _, e := os.Stat(filepath.Join(m.Dir, "known_hosts")); !os.IsNotExist(e) {
				t.Fatal("saved trust after failure")
			}
		})
	}
}
func TestCancellationClosesSSHAndRejectsSymlinkKey(t *testing.T) {
	f := fixture(t)
	m := managerFixture(t, f)
	f.block = true
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, e := m.Enroll(ctx, enrollmentRequest(f))
	if e == nil || ctx.Err() == nil || time.Since(start) > time.Second {
		t.Fatalf("cancel: %v", e)
	}
	os.Remove(filepath.Join(m.Dir, "keys", "backup"))
	os.Symlink(filepath.Join(m.Dir, "keys", "restore"), filepath.Join(m.Dir, "keys", "backup"))
	_, e = m.Enroll(context.Background(), enrollmentRequest(f))
	if e == nil {
		t.Fatal("symlink key accepted")
	}
}

func TestPAMAcceptsOnlyHiddenPasswordPrompt(t *testing.T) {
	for _, unsafe := range []bool{false, true} {
		t.Run(strconv.FormatBool(unsafe), func(t *testing.T) {
			f := fixture(t)
			m := managerFixture(t, f)
			f.mu.Lock()
			f.keyboardOnly = true
			f.unsafePrompt = unsafe
			f.mu.Unlock()
			_, e := m.Enroll(context.Background(), enrollmentRequest(f))
			if unsafe && e == nil {
				t.Fatal("accepted arbitrary PAM challenge")
			}
			if !unsafe && e != nil {
				t.Fatal(e)
			}
			f.mu.Lock()
			answers := f.keyboardAnswers
			f.mu.Unlock()
			if unsafe && answers != 0 {
				t.Fatal("password sent to arbitrary challenge")
			}
			if !unsafe && answers != 1 {
				t.Fatalf("PAM answers %d", answers)
			}
		})
	}
}
func TestSudoUsesOnlyStdinPasswordAndChecksRoot(t *testing.T) {
	f := fixture(t)
	m := managerFixture(t, f)
	f.mu.Lock()
	f.uid = "1000"
	f.mu.Unlock()
	_, e := m.Enroll(context.Background(), enrollmentRequest(f))
	if e != nil {
		t.Fatal(e)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var install string
	for _, c := range f.commands {
		if strings.Contains(c, "top-secret") {
			t.Fatal("password in command")
		}
		if strings.Contains(c, "mktemp") {
			install = c
		}
	}
	if !strings.Contains(install, "sudo -S") || !strings.Contains(install, "sudo -n sh -c") || !strings.Contains(install, `$(id -u)`) {
		t.Fatalf("privilege checks missing: %s", install)
	}
	if len(f.uploads) != 1 || !bytes.HasPrefix(f.uploads[0], []byte("top-secret\n")) {
		t.Fatal("missing stdin password")
	}
	tr := tar.NewReader(bytes.NewReader(bytes.TrimPrefix(f.uploads[0], []byte("top-secret\n"))))
	if _, e = tr.Next(); e != nil {
		t.Fatal(e)
	}
}
func TestProtectedTrustAndKeysRejectUnsafeFiles(t *testing.T) {
	for _, kind := range []string{"shared", "trustlink", "keydirlink", "fifo", "samekey"} {
		t.Run(kind, func(t *testing.T) {
			f := fixture(t)
			m := managerFixture(t, f)
			switch kind {
			case "shared":
				os.Chmod(filepath.Join(m.Dir, "keys", "backup"), 0660)
			case "trustlink":
				os.Symlink(filepath.Join(m.Dir, "keys", "backup"), filepath.Join(m.Dir, "known_hosts"))
			case "keydirlink":
				os.Rename(filepath.Join(m.Dir, "keys"), filepath.Join(m.Dir, "original"))
				os.Symlink("original", filepath.Join(m.Dir, "keys"))
			case "fifo":
				os.Remove(filepath.Join(m.Dir, "keys", "backup"))
				if e := unix.Mkfifo(filepath.Join(m.Dir, "keys", "backup"), 0600); e != nil {
					t.Fatal(e)
				}
			case "samekey":
				b, _ := os.ReadFile(filepath.Join(m.Dir, "keys", "backup"))
				os.WriteFile(filepath.Join(m.Dir, "keys", "restore"), b, 0640)
			}
			_, e := m.Enroll(context.Background(), enrollmentRequest(f))
			if e == nil {
				t.Fatal("unsafe configuration accepted")
			}
			f.mu.Lock()
			n := f.passwords
			f.mu.Unlock()
			if n != 0 {
				t.Fatal("password sent for unsafe configuration")
			}
		})
	}
}

func TestProbeOutputCannotPersistBootstrapPassword(t *testing.T) {
	f := fixture(t)
	m := managerFixture(t, f)
	f.failProbe = "secret"
	_, e := m.Enroll(context.Background(), enrollmentRequest(f))
	if e == nil {
		t.Fatal("remote probe persisted bootstrap password")
	}
	if _, e := os.Stat(filepath.Join(m.Dir, "known_hosts")); !os.IsNotExist(e) {
		t.Fatal("trust saved for secret probe")
	}
}
func TestOutputLimitClosesSessionPromptly(t *testing.T) {
	f := fixture(t)
	m := managerFixture(t, f)
	f.mu.Lock()
	f.flood = true
	f.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	_, e := m.Enroll(ctx, enrollmentRequest(f))
	if e == nil || time.Since(start) > 500*time.Millisecond {
		t.Fatalf("oversize output did not promptly terminate: %v, %s", e, time.Since(start))
	}
}

func TestMissingSudoUsesCachedAPTBeforeRepositoryRefresh(t *testing.T) {
	d := t.TempDir()
	log := filepath.Join(d, "apt.log")
	scripts := map[string]string{"id": "#!/bin/sh\necho 0\n", "pveversion": "#!/bin/sh\nexit 0\n", "tar": "#!/bin/sh\nexit 19\n", "apt-get": "#!/bin/sh\nprintf '%s\\n' \"$*\" >>\"$ANKER_APT_LOG\"\ncase \"$*\" in *install*) /bin/cp \"$ANKER_APT_BIN/id\" \"$ANKER_APT_BIN/sudo\"; /bin/cp \"$ANKER_APT_BIN/id\" \"$ANKER_APT_BIN/visudo\";; esac\n"}
	for name, script := range scripts {
		if e := os.WriteFile(filepath.Join(d, name), []byte(script), 0700); e != nil {
			t.Fatal(e)
		}
	}
	for _, name := range []string{"mktemp", "rm"} {
		p, e := exec.LookPath(name)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.Symlink(p, filepath.Join(d, name)); e != nil {
			t.Fatal(e)
		}
	}
	c := exec.Command("/bin/sh", "-c", rootScript)
	c.Env = []string{"PATH=" + d, "ANKER_APT_BIN=" + d, "ANKER_APT_LOG=" + log}
	if e := c.Run(); e == nil {
		t.Fatal("stub tar should stop before installer")
	}
	b, e := os.ReadFile(log)
	if e != nil {
		t.Fatal(e)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 1 || !strings.Contains(lines[0], "install -y sudo") {
		t.Fatalf("APT must first install cached sudo without update: %s", b)
	}
}

func TestInspectedHostKeySwapNeverReceivesPassword(t *testing.T) {
	f := fixture(t)
	m := managerFixture(t, f)
	id, e := m.Inspect(context.Background(), f.request())
	if e != nil {
		t.Fatal(e)
	}
	f.mu.Lock()
	f.key = signer(t)
	f.mu.Unlock()
	r := enrollmentRequest(f)
	r.Fingerprint = id.Fingerprint
	if _, e = m.Enroll(context.Background(), r); e == nil {
		t.Fatal("accepted host key swapped after inspection")
	}
	f.mu.Lock()
	n := f.passwords
	f.mu.Unlock()
	if n != 0 {
		t.Fatal("swapped endpoint received password")
	}
}
func TestHashedKnownHostIsPreservedAndNotDuplicated(t *testing.T) {
	f := fixture(t)
	m := managerFixture(t, f)
	line := knownhosts.Line([]string{knownhosts.HashHostname(knownhosts.Normalize(f.listener.Addr().String()))}, f.key.PublicKey()) + "\n"
	os.WriteFile(filepath.Join(m.Dir, "known_hosts"), []byte(line), 0640)
	id, e := m.Inspect(context.Background(), f.request())
	if e != nil || !id.Known || id.Changed {
		t.Fatalf("hashed identity %+v %v", id, e)
	}
	if _, e = m.Enroll(context.Background(), enrollmentRequest(f)); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(filepath.Join(m.Dir, "known_hosts"))
	if string(b) != line {
		t.Fatal("hashed trust entry replaced or duplicated")
	}
}

func TestSudoKeepsAuthenticatingParentAliveForCachedCommands(t *testing.T) {
	d := t.TempDir()
	// sudo's non-TTY timestamp can be scoped to its invoking parent. Model that
	// behavior in an executable privilege checker, including the final install.
	executable, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Symlink(executable, filepath.Join(d, "sudo")); e != nil {
		t.Fatal(e)
	}
	shell, e := exec.LookPath("sh")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Symlink(shell, filepath.Join(d, "sh")); e != nil {
		t.Fatal(e)
	}
	c := exec.Command("/bin/sh", "-c", sudoInstallCommand)
	c.Env = []string{"PATH=" + d, "ANKER_SUDO_CACHE=" + filepath.Join(d, "cache"), "ANKER_SUDO_FIXTURE=1"}
	c.Stdin = strings.NewReader("top-secret\narchive-data")
	out, e := c.CombinedOutput()
	if e != nil || string(out) != "installed\n" {
		t.Fatalf("cached sudo lost authenticating parent: %v %q", e, out)
	}
}

// A subprocess of this test binary implements sudo's parent-scoped cache using
// the actual process parent. It never executes installation code or a real sudo.
func TestMain(m *testing.M) {
	if os.Getenv("ANKER_SUDO_FIXTURE") == "1" {
		os.Exit(sudoFixtureExit())
	}
	os.Exit(m.Run())
}
func sudoFixtureExit() int {
	if len(os.Args) < 2 {
		return 1
	}
	parent := strconv.Itoa(os.Getppid())
	path := os.Getenv("ANKER_SUDO_CACHE")
	if os.Args[1] == "-S" {
		line, e := bufio.NewReader(os.Stdin).ReadString('\n')
		if e != nil || line != "top-secret\n" {
			return 1
		}
		if os.WriteFile(path, []byte(parent), 0600) != nil {
			return 1
		}
		return 0
	}
	if os.Args[1] != "-n" || len(os.Args) < 3 {
		return 1
	}
	authorized, e := os.ReadFile(path)
	if e != nil || string(authorized) != parent {
		return 42
	}
	switch os.Args[2] {
	case "id":
		fmt.Println("0")
	case "sh":
		if len(os.Args) < 5 {
			return 1
		}
		if strings.Contains(os.Args[4], "mktemp") {
			fmt.Println("installed")
		} else if os.Args[4] != `[ "$(id -u)" -eq 0 ]` {
			return 1
		}
	default:
		return 1
	}
	return 0
}

func TestConfiguredServiceOwnerAppliesOnlyToProtectedPrivateRoleKeys(t *testing.T) {
	m := Manager{OwnerUID: 0, GroupGID: 1001, KeyOwnerUID: 1001}
	for _, tc := range []struct {
		name                  string
		uid, gid              uint32
		mode                  uint32
		dir, private, allowed bool
	}{
		{"service private key", 1001, 1001, unix.S_IFREG | 0600, false, true, true},
		{"root private key", 0, 1001, unix.S_IFREG | 0640, false, true, true},
		{"unrelated owner", 1002, 1001, unix.S_IFREG | 0600, false, true, false},
		{"wrong group", 1001, 1002, unix.S_IFREG | 0600, false, true, false},
		{"world readable key", 1001, 1001, unix.S_IFREG | 0644, false, true, false},
		{"group writable key", 1001, 1001, unix.S_IFREG | 0660, false, true, false},
		{"FIFO instead of key", 1001, 1001, unix.S_IFIFO | 0600, false, true, false},
		{"service owned trust", 1001, 1001, unix.S_IFREG | 0640, false, false, false},
		{"service owned key directory", 1001, 1001, unix.S_IFDIR | 0750, true, false, false},
		{"root owned key directory", 0, 1001, unix.S_IFDIR | 0750, true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := m.checkMetadata(tc.mode, int(tc.uid), int(tc.gid), tc.dir, tc.private)
			if (e == nil) != tc.allowed {
				t.Fatalf("owner policy allowed=%v want=%v: %v", e == nil, tc.allowed, e)
			}
		})
	}
	m.KeyOwnerUID = 0
	if m.checkMetadata(unix.S_IFREG|0600, 1001, 1001, false, true) == nil {
		t.Fatal("default configuration accepted service-owned keys")
	}
}
