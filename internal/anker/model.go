package anker

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const FormatVersion = 1

type Host struct {
	RestoreSSHUser string     `json:"restore_ssh_user,omitempty"`
	RestoreKeyPath string     `json:"restore_key_path,omitempty"`
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Address        string     `json:"address"`
	Group          string     `json:"group"`
	ClusterID      string     `json:"cluster_id"`
	SSHUser        string     `json:"ssh_user"`
	SSHPort        int        `json:"ssh_port"`
	KeyPath        string     `json:"key_path"`
	KnownHostsPath string     `json:"known_hosts_path"`
	Enabled        bool       `json:"enabled"`
	Schedule       string     `json:"schedule"`
	ExtraPaths     []string   `json:"extra_paths"`
	DemoRoot       string     `json:"-"`
	Inventory      *Inventory `json:"inventory,omitempty"`
	LastProbe      string     `json:"last_probe,omitempty"`
	ProbeError     string     `json:"probe_error,omitempty"`
}
type Interface struct {
	Name string `json:"name"`
	MAC  string `json:"mac"`
	PCI  string `json:"pci,omitempty"`
	Role string `json:"role,omitempty"`
}
type Disk struct {
	Name  string `json:"name"`
	ID    string `json:"id"`
	Size  int64  `json:"size"`
	UUID  string `json:"uuid,omitempty"`
	Mount string `json:"mount,omitempty"`
}
type Inventory struct {
	Hostname    string                     `json:"hostname"`
	PVEVersion  string                     `json:"pve_version"`
	Debian      string                     `json:"debian"`
	Kernel      string                     `json:"kernel"`
	BootMode    string                     `json:"boot_mode"`
	ClusterID   string                     `json:"cluster_id"`
	Quorate     bool                       `json:"quorate"`
	Interfaces  []Interface                `json:"interfaces"`
	Disks       []Disk                     `json:"disks"`
	Details     map[string]json.RawMessage `json:"details,omitempty"`
	CapturedAt  string                     `json:"captured_at"`
	Fingerprint string                     `json:"fingerprint"`
}
type Entry struct {
	Path   string            `json:"path"`
	Type   string            `json:"type"`
	Mode   uint32            `json:"mode"`
	UID    int               `json:"uid"`
	GID    int               `json:"gid"`
	Size   int64             `json:"size"`
	MTime  int64             `json:"mtime"`
	Link   string            `json:"link,omitempty"`
	SHA256 string            `json:"sha256,omitempty"`
	XAttrs map[string]string `json:"xattrs,omitempty"`
	Secret bool              `json:"secret"`
}
type Collection struct {
	Inventory Inventory `json:"inventory"`
	Entries   []Entry   `json:"entries"`
	Warnings  []string  `json:"warnings"`
}
type Manifest struct {
	Version     int               `json:"version"`
	ID          string            `json:"id"`
	HostID      string            `json:"host_id"`
	CreatedAt   string            `json:"created_at"`
	CompletedAt string            `json:"completed_at"`
	Status      string            `json:"status"`
	Consistency string            `json:"consistency"`
	Entries     []Entry           `json:"entries"`
	Warnings    []string          `json:"warnings"`
	Artifacts   map[string]string `json:"artifacts"`
	Inventory   Inventory         `json:"inventory"`
}
type Backup struct {
	VerifiedAt        string   `json:"verified_at,omitempty"`
	VerificationError string   `json:"verification_error,omitempty"`
	ID                string   `json:"id"`
	HostID            string   `json:"host_id"`
	HostName          string   `json:"host_name"`
	CreatedAt         string   `json:"created_at"`
	Status            string   `json:"status"`
	Size              int64    `json:"size"`
	Files             int      `json:"files"`
	Pinned            bool     `json:"pinned"`
	Archived          bool     `json:"archived"`
	ManifestSHA       string   `json:"manifest_sha"`
	Warnings          []string `json:"warnings"`
}
type Job struct {
	ID         string `json:"id"`
	HostID     string `json:"host_id"`
	Kind       string `json:"kind"`
	State      string `json:"state"`
	CreatedAt  string `json:"created_at"`
	FinishedAt string `json:"finished_at,omitempty"`
	ResultID   string `json:"result_id,omitempty"`
	Error      string `json:"error,omitempty"`
	Attempts   int    `json:"attempts"`
}
type Mapping struct {
	Interfaces map[string]string `json:"interfaces"`
	Storage    map[string]string `json:"storage"`
	Hostname   string            `json:"hostname"`
	Address    string            `json:"address"`
}
type PlanRequest struct {
	BackupID         string   `json:"backup_id"`
	TargetID         string   `json:"target_id"`
	Scenario         string   `json:"scenario"`
	Files            []string `json:"files"`
	Mapping          Mapping  `json:"mapping"`
	ConsoleConfirmed bool     `json:"console_confirmed"`
	SourceOffline    bool     `json:"source_offline"`
}
type Step struct {
	Secret      bool   `json:"secret"`
	Path        string `json:"path"`
	Action      string `json:"action"`
	Reason      string `json:"reason"`
	BeforeSHA   string `json:"before_sha,omitempty"`
	PreparedSHA string `json:"prepared_sha,omitempty"`
	Diff        string `json:"diff,omitempty"`
}
type Plan struct {
	ID        string       `json:"id"`
	BackupID  string       `json:"backup_id"`
	TargetID  string       `json:"target_id"`
	Scenario  string       `json:"scenario"`
	CreatedAt string       `json:"created_at"`
	State     string       `json:"state"`
	Source    Inventory    `json:"source"`
	Target    Inventory    `json:"target"`
	Mapping   Mapping      `json:"mapping"`
	Steps     []Step       `json:"steps"`
	Blockers  []string     `json:"blockers"`
	Manual    []string     `json:"manual"`
	Result    *ApplyResult `json:"result,omitempty"`
}
type ApplyResult struct {
	Applied        []string `json:"applied"`
	RollbackPath   string   `json:"rollback_path"`
	Checks         []string `json:"checks"`
	RebootVerified bool     `json:"reboot_verified"`
}
type Settings struct {
	SessionDays  int    `json:"session_days"`
	Timezone     string `json:"timezone"`
	Schedule     string `json:"schedule"`
	Parallel     int    `json:"parallel"`
	Retries      int    `json:"retries"`
	Daily        int    `json:"daily"`
	Weekly       int    `json:"weekly"`
	Monthly      int    `json:"monthly"`
	ArchiveDays  int    `json:"archive_days"`
	StaleHours   int    `json:"stale_hours"`
	Webhook      string `json:"webhook"`
	SMTPServer   string `json:"smtp_server"`
	SMTPUser     string `json:"smtp_user"`
	SMTPPassword string `json:"smtp_password,omitempty"`
	MailFrom     string `json:"mail_from"`
	MailTo       string `json:"mail_to"`
}
type User struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Role         string `json:"role"`
	Secrets      bool   `json:"secrets"`
	PasswordHash string `json:"-"`
	Disabled     bool   `json:"disabled"`
}
type Audit struct {
	ID     string `json:"id"`
	At     string `json:"at"`
	User   string `json:"user"`
	Action string `json:"action"`
	Object string `json:"object"`
}
type Collector interface {
	Probe(context.Context, Host) (Inventory, error)
	Collect(context.Context, Host, string) (Collection, error)
	Apply(context.Context, Host, Plan, string) (ApplyResult, error)
}
type Service struct {
	backupLocks   map[string]*sync.RWMutex
	planLocks     map[string]*sync.RWMutex
	hostMu        sync.Mutex
	diskUsage     func() (DiskUsage, error)
	Root          string
	Store         *Store
	Collector     Collector
	mu            sync.Mutex
	locks         map[string]bool
	cancels       map[string]context.CancelFunc
	Demo          bool
	schedulerBusy bool
	maintenance   bool
	jobMu         sync.Mutex
	notifyMu      sync.Mutex
	storageMu     sync.Mutex
}

func ID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func now() string          { return time.Now().UTC().Format(time.RFC3339Nano) }
func Hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func ValidPath(p string) error {
	if p == "" || filepath.IsAbs(p) || strings.Contains(p, "\\") || strings.ContainsRune(p, 0) || filepath.ToSlash(filepath.Clean(p)) != p || p == "." || p == ".." || strings.HasPrefix(p, "../") {
		return fmt.Errorf("ungültiger relativer Pfad %q", p)
	}
	return nil
}
func validID(p string) bool {
	if len(p) == 0 || len(p) > 100 {
		return false
	}
	for _, c := range p {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
func safeJoin(root, p string) (string, error) {
	if err := ValidPath(p); err != nil {
		return "", err
	}
	cur := root
	for _, part := range strings.Split(p, "/") {
		cur = filepath.Join(cur, part)
		st, err := os.Lstat(cur)
		if err == nil && st.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("symbolischer Link in Ablagepfad")
		}
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
	}
	return cur, nil
}
func writeJSON(p string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(p, append(b, '\n'), 0600)
}
func atomicWrite(p string, b []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".write-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	cerr := f.Close()
	if err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(name, p)
	}
	if err == nil {
		err = syncDir(filepath.Dir(p))
	}
	return err
}
func Fingerprint(i Inventory) string {
	i.CapturedAt = ""
	i.Fingerprint = ""
	stable := map[string]json.RawMessage{}
	for _, key := range []string{"file_hashes", "boot_id", "addresses", "routes", "packages", "manual_packages", "pci", "boot"} {
		if v, ok := i.Details[key]; ok {
			if key == "addresses" {
				var value any
				if json.Unmarshal(v, &value) == nil {
					value = stableNetwork(value)
					v, _ = json.Marshal(value)
				}
			}
			stable[key] = v
		}
	}
	i.Details = stable
	b, _ := json.Marshal(i)
	return Hash(b)
}
func stableNetwork(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, item := range x {
			if k == "valid_life_time" || k == "preferred_life_time" {
				delete(x, k)
			} else {
				x[k] = stableNetwork(item)
			}
		}
	case []any:
		for i, item := range x {
			x[i] = stableNetwork(item)
		}
	}
	return v
}
func isSecret(p string) bool {
	p = strings.ToLower(p)
	if strings.Contains(p, "priv/") || strings.Contains(p, ".ssh/") || strings.Contains(p, "wireguard/") || strings.Contains(p, "secret") || strings.Contains(p, "password") || strings.Contains(p, "shadow") || strings.Contains(p, "credential") || strings.Contains(p, "token") || strings.HasSuffix(p, ".key") || strings.HasSuffix(p, ".keyring") || strings.HasSuffix(p, ".pem") {
		return true
	}
	// Unknown configuration is restricted until explicitly known to be suitable for readers.
	switch p {
	case "etc/hostname", "etc/hosts", "etc/fstab", "etc/resolv.conf", "etc/debian_version", "etc/network/interfaces", "etc/ssh/sshd_config", "etc/pve/storage.cfg":
		return false
	}
	if strings.HasPrefix(p, "etc/sysctl.d/") || strings.HasPrefix(p, "etc/network/interfaces.d/") {
		return false
	}
	return true
}

var secretAssignment = regexp.MustCompile(`(?im)["']?\b(?:password|passwd|passphrase|psk|secret|token|apikey|api_key|privatekey|private_key|key)["']?\s*[:=]`)

func secretContent(data []byte) bool {
	return strings.Contains(strings.ToLower(string(data)), "private key") || secretAssignment.Match(data)
}
