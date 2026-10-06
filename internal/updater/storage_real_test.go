//go:build linux

package updater

import (
	"anker/internal/storage"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStorageRealExt4GrowthUnderSystemd(t *testing.T) {
	if os.Getenv("ANKER_SYSTEMD_TEST") != "1" || os.Geteuid() != 0 {
		t.Skip("isolated root/systemd CI only")
	}
	if _, err := os.Stat("/run/anker-isolated-ci"); err != nil {
		t.Fatal("missing isolated CI marker")
	}
	command := func(name string, args ...string) string {
		t.Helper()
		out, err := exec.Command(name, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v: %s", name, args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	image := filepath.Join(t.TempDir(), "dedicated-ext4.img")
	file, err := os.Create(image)
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Truncate(64 << 20); err != nil {
		t.Fatal(err)
	}
	file.Close()
	device := command("losetup", "--find", "--show", image)
	defer exec.Command("losetup", "--detach", device).Run()
	command("mkfs.ext4", "-F", device)
	mount := "/srv/anker/ci-storage"
	if err = os.Mkdir(mount, 0700); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(mount)
	command("mount", device, mount)
	defer exec.Command("umount", mount).Run()
	content := []byte("data must survive filesystem growth\n")
	if err = os.WriteFile(filepath.Join(mount, "marker"), content, 0600); err != nil {
		t.Fatal(err)
	}
	pid := command("systemctl", "show", "anker", "--property=MainPID", "--value")
	file, err = os.OpenFile(image, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	err = file.Truncate(128 << 20)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	command("losetup", "--set-capacity", device)
	call := func(method, path string, input any, out any) {
		t.Helper()
		var payload []byte
		if input != nil {
			payload, _ = json.Marshal(input)
		}
		req, _ := http.NewRequest(method, "http://updater.local"+path, bytes.NewReader(payload))
		req.Header.Set("X-Anker-Request", "1")
		res, e := UnixClient(Socket, 20*time.Second).Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		if res.StatusCode >= 400 {
			body, _ := io.ReadAll(res.Body)
			t.Fatalf("%s: %d %s", path, res.StatusCode, body)
		}
		if e = json.NewDecoder(res.Body).Decode(out); e != nil {
			t.Fatal(e)
		}
	}
	report, err := storage.Inspect("/srv/anker")
	if err != nil {
		t.Fatal(err)
	}
	volumeID := ""
	for _, v := range report.Volumes {
		if v.Mount == mount {
			volumeID = v.ID
		}
	}
	if volumeID == "" {
		t.Fatal("data submount missing", report)
	}
	var p storage.GrowthPlan
	call("POST", "/storage/plan", map[string]string{"volume_id": volumeID}, &p)
	if !p.CanGrow || p.Mount != mount || p.FilesystemBytes != 64<<20 || p.DeviceBytes != 128<<20 {
		t.Fatal("real growth unavailable", p)
	}
	var state storage.GrowthState
	call("POST", "/storage/grow", map[string]string{"volume_id": volumeID, "plan_id": p.ID, "confirmation": mount}, &state)
	deadline := time.Now().Add(30 * time.Second)
	for state.Status == "running" && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		call("GET", "/storage/state", nil, &state)
	}
	if state.Status != "successful" || state.AfterBytes != 128<<20 {
		t.Fatal("growth not verified", state)
	}
	got, err := os.ReadFile(filepath.Join(mount, "marker"))
	if err != nil || !bytes.Equal(got, content) {
		t.Fatal("existing data damaged", string(got), err)
	}
	if pid != command("systemctl", "show", "anker", "--property=MainPID", "--value") {
		t.Fatal("growth restarted web service")
	}
	t.Log("real ext4 grew from 64 to 128 MiB through the sandboxed systemd helper; existing data and web PID preserved")
}
