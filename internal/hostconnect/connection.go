// Package hostconnect enrolls Proxmox hosts over a confirmed SSH identity.
package hostconnect

import (
	"encoding/json"
)

type InspectRequest struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
}
type Identity struct {
	Address     string `json:"address"`
	Port        int    `json:"port"`
	Fingerprint string `json:"fingerprint"`
	KeyType     string `json:"key_type"`
	Known       bool   `json:"known"`
	Changed     bool   `json:"changed"`
}
type EnrollRequest struct {
	Address     string `json:"address"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Fingerprint string `json:"fingerprint"`
	Confirmed   bool   `json:"confirmed"`
}
type Enrollment struct {
	Address        string          `json:"address"`
	Port           int             `json:"port"`
	Fingerprint    string          `json:"fingerprint"`
	BackupUser     string          `json:"backup_user"`
	RestoreUser    string          `json:"restore_user"`
	BackupKeyPath  string          `json:"backup_key_path"`
	RestoreKeyPath string          `json:"restore_key_path"`
	KnownHostsPath string          `json:"known_hosts_path"`
	Inventory      json.RawMessage `json:"inventory"`
}
type Manager struct {
	Dir      string
	OwnerUID int
	GroupGID int
}
