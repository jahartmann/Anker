package updater

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// This test runs real fixed-path services only in the explicitly isolated CI VM.
func TestSystemdInstallationAndRecovery(t *testing.T) {
	if os.Getenv("ANKER_SYSTEMD_TEST") != "1" {
		t.Skip("requires isolated Linux/systemd CI runner")
	}
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || os.Getenv("GITHUB_ACTIONS") != "true" {
		t.Fatal("unsafe integration environment")
	}
	if _, err := os.Stat("/run/anker-isolated-ci"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	ctl := SystemControl{Socket: filepath.Join(DataDir, "anker.sock")}
	i := &Installer{Binary: BinaryPath, Data: DataDir, StateDir: StateDir, Control: ctl}
	command := func(action string, units ...string) {
		t.Helper()
		args := append([]string{action}, units...)
		if out, err := exec.CommandContext(ctx, "systemctl", args...).CombinedOutput(); err != nil {
			t.Fatal(string(out), err)
		}
	}
	health := func(want string) {
		t.Helper()
		if err := ctl.Health(ctx, want); err != nil {
			out, _ := exec.Command("journalctl", "-u", "anker", "-u", "anker-updater", "-n", "80", "--no-pager").CombinedOutput()
			t.Fatal(err, string(out))
		}
	}
	old, err := ctl.call(ctx, "GET", "")
	if err != nil {
		t.Fatal(err)
	}
	health(old)
	jar, _ := cookiejar.New(nil)
	web := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	login := func() {
		t.Helper()
		request, err := http.NewRequest("POST", "http://127.0.0.1:8087/api/login", bytes.NewBufferString(`{"name":"admin","password":"systemd-test-password"}`))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Anker-Request", "1")
		response, err := web.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			b, _ := io.ReadAll(response.Body)
			t.Fatal(response.StatusCode, string(b))
		}
	}
	session := func() {
		t.Helper()
		response, err := web.Get("http://127.0.0.1:8087/api/hosts")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatal("session lost", response.StatusCode)
		}
	}
	login()
	response, err := web.Get("http://127.0.0.1:8087/api/status")
	if err != nil {
		t.Fatal(err)
	}
	var initial struct {
		Demo                        bool `json:"demo"`
		Hosts, Backups, Jobs, Plans []json.RawMessage
	}
	err = json.NewDecoder(response.Body).Decode(&initial)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || initial.Demo || len(initial.Hosts)+len(initial.Backups)+len(initial.Jobs)+len(initial.Plans) != 0 {
		t.Fatal("fresh production service is not empty", initial, err)
	}
	if _, err := os.Stat(filepath.Join(DataDir, "demo-hosts")); !os.IsNotExist(err) {
		t.Fatal("demo files installed", err)
	}
	if out, err := exec.CommandContext(ctx, BinaryPath, "--data", DataDir, "demo").CombinedOutput(); err == nil {
		t.Fatal("production path accepted for demo", string(out))
	}
	health(old)
	t.Log("fresh production installation contains no demo hosts, backups, plans or jobs")
	candidate, err := os.ReadFile(os.Getenv("ANKER_SYSTEMD_CANDIDATE"))
	if err != nil {
		t.Fatal(err)
	}
	release := func(version string, b []byte) Release {
		sum := sha256.Sum256(b)
		return Release{Version: version, Artifact: Artifact{Size: int64(len(b)), SHA256: hex.EncodeToString(sum[:])}}
	}
	if err = i.Install(ctx, release("v0.2.1", candidate), candidate); err != nil {
		t.Fatal(err)
	}
	health("v0.2.1")
	session()
	t.Log("real service update and persisted login passed")
	bad := []byte("broken candidate executable\n")
	if err = i.Install(ctx, release("v0.2.2", bad), bad); err == nil {
		t.Fatal("broken candidate accepted")
	}
	health("v0.2.1")
	session()
	t.Log("failed executable automatically rolled back under systemd")
	for _, snapshot := range []bool{false, true} {
		command("stop", "anker-updater")
		previous, err := ctl.Prepare(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err = copyFile(BinaryPath, filepath.Join(StateDir, "previous"), 0755, -1, -1); err != nil {
			t.Fatal(err)
		}
		j := journal{Version: "v0.2.2", Previous: previous, UID: -1, GID: -1}
		if err = i.writeJournal(j); err != nil {
			t.Fatal(err)
		}
		command("stop", "anker")
		if snapshot {
			if err = copyFile(filepath.Join(DataDir, "catalog.db"), filepath.Join(StateDir, "catalog.db"), 0600, -1, -1); err != nil {
				t.Fatal(err)
			}
			// The real service UID/GID comes from the installed directory owner.
			out, err := exec.Command("id", "-u", "anker").Output()
			if err != nil {
				t.Fatal(err)
			}
			var uid int
			if _, err = fmt.Sscan(string(out), &uid); err != nil {
				t.Fatal(err)
			}
			out, err = exec.Command("id", "-g", "anker").Output()
			if err != nil {
				t.Fatal(err)
			}
			var gid int
			if _, err = fmt.Sscan(string(out), &gid); err != nil {
				t.Fatal(err)
			}
			j.Snapshot, j.UID, j.GID = true, uid, gid
			if err = i.writeJournal(j); err != nil {
				t.Fatal(err)
			}
			if err = atomic(filepath.Join(DataDir, "catalog.db"), []byte("corrupted candidate catalog"), 0600, uid, gid); err != nil {
				t.Fatal(err)
			}
		}
		if err = atomic(BinaryPath, bad, 0755, -1, -1); err != nil {
			t.Fatal(err)
		}
		// Restart both real units as they are enabled at boot. The known helper repairs.
		exec.CommandContext(ctx, "systemctl", "start", "anker").Run() // Candidate failure is expected.
		command("start", "anker-updater")
		deadline := time.Now().Add(60 * time.Second)
		for {
			_, pending := os.Stat(filepath.Join(StateDir, "pending.json"))
			_, guard := os.Stat(Maintenance)
			if os.IsNotExist(pending) && os.IsNotExist(guard) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("helper did not recover interrupted journal")
			}
			time.Sleep(250 * time.Millisecond)
		}
		health("v0.2.1")
		login()
		session()
		if _, err = os.Stat(Maintenance); !os.IsNotExist(err) {
			t.Fatal("maintenance marker not released", err)
		}
		response, err := UnixClient(Socket, 5*time.Second).Get("http://updater.local/status")
		if err != nil {
			t.Fatal(err)
		}
		var state State
		err = json.NewDecoder(response.Body).Decode(&state)
		response.Body.Close()
		if err != nil || state.Status != "rolled_back" {
			t.Fatal(state, err)
		}
		t.Log("interrupted update recovered; catalog snapshot:", snapshot)
	}
	command("restart", "anker", "anker-updater")
	health("v0.2.1")
	session()
	for _, unit := range []string{"anker", "anker-updater"} {
		command("is-enabled", unit)
	}
}
