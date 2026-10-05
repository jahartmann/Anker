package anker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type DemoCollector struct{ Root string }

func (c DemoCollector) root(h Host) (string, error) {
	if !validID(h.ID) {
		return "", errors.New("ungültige Demo-ID")
	}
	p := filepath.Join(c.Root, "demo-hosts", h.ID)
	if _, err := os.Stat(filepath.Join(p, "inventory.json")); err != nil {
		return "", errors.New("Lokale Demo: Dieser Host ist nicht an einen echten Server angeschlossen")
	}
	return p, nil
}
func (c DemoCollector) Probe(_ context.Context, h Host) (Inventory, error) {
	root, err := c.root(h)
	if err != nil {
		return Inventory{}, err
	}
	data, err := os.ReadFile(filepath.Join(root, "inventory.json"))
	if err != nil {
		return Inventory{}, err
	}
	var inv Inventory
	if err = json.Unmarshal(data, &inv); err != nil {
		return inv, err
	}
	hashes := map[string]string{}
	err = filepath.WalkDir(filepath.Join(root, "files"), func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(filepath.Join(root, "files"), p)
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		hashes[filepath.ToSlash(rel)] = Hash(data)
		return nil
	})
	if err != nil {
		return inv, err
	}
	if inv.Details == nil {
		inv.Details = map[string]json.RawMessage{}
	}
	inv.Details["file_hashes"], _ = json.Marshal(hashes)
	inv.CapturedAt = now()
	inv.Fingerprint = Fingerprint(inv)
	return inv, nil
}
func (c DemoCollector) Collect(ctx context.Context, h Host, dest string) (Collection, error) {
	root, err := c.root(h)
	if err != nil {
		return Collection{}, err
	}
	inv, err := c.Probe(ctx, h)
	if err != nil {
		return Collection{}, err
	}
	entries := []Entry{}
	err = filepath.WalkDir(filepath.Join(root, "files"), func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == filepath.Join(root, "files") {
			return nil
		}
		rel, _ := filepath.Rel(filepath.Join(root, "files"), p)
		rel = filepath.ToSlash(rel)
		st, err := d.Info()
		if err != nil {
			return err
		}
		e := Entry{Path: rel, UID: 0, GID: 0, Mode: 0644, MTime: st.ModTime().Unix(), Type: "file"}
		target, err := safeJoin(filepath.Join(dest, "files"), rel)
		if err != nil {
			return err
		}
		if d.IsDir() {
			e.Type = "directory"
			e.Mode = 0755
			err = os.MkdirAll(target, 0700)
		} else {
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			err = atomicWrite(target, data, 0600)
			e.Size = int64(len(data))
			if isSecret(rel) {
				e.Mode = 0600
			}
		}
		entries = append(entries, e)
		return err
	})
	if err != nil {
		return Collection{}, err
	}
	data, err := os.ReadFile(filepath.Join(root, "config.db"))
	if err != nil {
		return Collection{}, err
	}
	if err = atomicWrite(filepath.Join(dest, "recovery/config.db"), data, 0600); err != nil {
		return Collection{}, err
	}
	return Collection{Inventory: inv, Entries: entries, Warnings: []string{}}, nil
}
func (c DemoCollector) Apply(ctx context.Context, h Host, p Plan, root string) (ApplyResult, error) {
	target, err := c.root(h)
	if err != nil {
		return ApplyResult{}, err
	}
	mdata, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		return ApplyResult{}, err
	}
	var m Manifest
	if err = json.Unmarshal(mdata, &m); err != nil {
		return ApplyResult{}, err
	}
	for _, step := range p.Steps {
		if step.Action != "apply" {
			continue
		}
		dest, err := safeJoin(filepath.Join(target, "files"), step.Path)
		if err != nil {
			return ApplyResult{}, err
		}
		data, err := os.ReadFile(dest)
		hash := "missing"
		if err == nil {
			hash = Hash(data)
		} else if !os.IsNotExist(err) {
			return ApplyResult{}, err
		}
		if hash != step.BeforeSHA {
			return ApplyResult{}, errors.New("Demo-Ziel wurde verändert")
		}
	}
	result := ApplyResult{Applied: []string{}, RollbackPath: filepath.Join(target, "rollback", p.ID), Checks: []string{"Lokale Testdateien übertragen. Kein realer Proxmox-Host geändert."}}
	for _, step := range p.Steps {
		if step.Action != "apply" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return result, err
		}
		source, _ := safeJoin(filepath.Join(root, "prepared-files"), step.Path)
		dest, _ := safeJoin(filepath.Join(target, "files"), step.Path)
		data, err := os.ReadFile(source)
		if err != nil {
			return result, err
		}
		if old, err := os.ReadFile(dest); err == nil {
			back, _ := safeJoin(result.RollbackPath, step.Path)
			if err = atomicWrite(back, old, 0600); err != nil {
				return result, err
			}
		}
		if err = atomicWrite(dest, data, 0600); err != nil {
			return result, err
		}
		result.Applied = append(result.Applied, step.Path)
	}
	return result, nil
}
func (s *Service) SeedDemo(a *Auth) error {
	s.Demo = true
	users, _ := records[User](s.Store, "users")
	if len(users) == 0 {
		if err := a.CreateUser("demo", "anker-demo-2026", "admin", true); err != nil {
			return err
		}
	}
	names := []string{"pve-berlin-01", "pve-berlin-02", "pve-berlin-03", "pve-hamburg-01", "pve-hamburg-02", "pve-lab-01"}
	for i, name := range names {
		id := fmt.Sprintf("demo%d", i+1)
		if _, err := s.Host(id); err == nil {
			continue
		}
		group := "Standalone"
		cluster := ""
		if i < 3 {
			group = "Cluster Berlin"
			cluster = "berlin"
		}
		h := Host{ID: id, Name: name, Address: fmt.Sprintf("192.0.2.%d", 11+i), Group: group, ClusterID: cluster, SSHUser: "anker", SSHPort: 22, Enabled: true}
		if err := s.SaveHost(h); err != nil {
			return err
		}
		root := filepath.Join(s.Root, "demo-hosts", id)
		version := "8.4"
		nic := "eno1"
		if i == 5 {
			version = "9.1"
			nic = "ens3"
		}
		inv := Inventory{Hostname: name, PVEVersion: version, Debian: "12", Kernel: "6.8.12-pve", BootMode: "UEFI", ClusterID: cluster, Quorate: cluster != "", Interfaces: []Interface{{Name: nic, MAC: fmt.Sprintf("02:00:00:00:00:%02d", i+1)}}, Disks: []Disk{{Name: "nvme0n1", ID: "demo-disk-" + id, Size: 1000204886016}}, Details: map[string]json.RawMessage{}}
		if err := writeJSON(filepath.Join(root, "inventory.json"), inv); err != nil {
			return err
		}
		files := map[string]string{"etc/network/interfaces": fmt.Sprintf("auto lo\niface lo inet loopback\n\nauto %s\niface %s inet manual\n\nauto vmbr0\niface vmbr0 inet static\n    address 192.0.2.%d/24\n    gateway 192.0.2.1\n    bridge-ports %s\n    bridge-stp off\n    bridge-fd 0\n", nic, nic, 11+i, nic), "etc/hostname": name + "\n", "etc/hosts": "127.0.0.1 localhost\n", "etc/pve/storage.cfg": "dir: local\n    path /var/lib/vz\n    content iso,vztmpl,backup\n\nlvmthin: local-lvm\n    thinpool data\n    vgname pve\n    content images,rootdir\n", "etc/pve/priv/storage/demo.pw": "demo-secret-not-a-production-password\n", "etc/sysctl.d/99-anker.conf": "# Lokale Testkonfiguration\nnet.ipv4.ip_forward=1\n", "etc/ssh/sshd_config": "PermitRootLogin prohibit-password\nPasswordAuthentication no\n"}
		for p, data := range files {
			full, _ := safeJoin(filepath.Join(root, "files"), p)
			if err := atomicWrite(full, []byte(data), 0600); err != nil {
				return err
			}
		}
		db, err := sql.Open("sqlite", filepath.Join(root, "config.db"))
		if err != nil {
			return err
		}
		_, err = db.Exec(`CREATE TABLE tree(inode INTEGER PRIMARY KEY,parent INTEGER,version INTEGER,writer INTEGER,mtime INTEGER,type INTEGER,name TEXT,data BLOB); INSERT INTO tree VALUES(0,0,1,0,0,4,'',NULL)`)
		db.Close()
		if err != nil {
			return err
		}
		if i < 5 {
			if _, err = s.CreateBackup(context.Background(), id); err != nil {
				return err
			}
			if i == 0 {
				p := filepath.Join(root, "files/etc/sysctl.d/99-anker.conf")
				atomicWrite(p, []byte("# Lokale Testkonfiguration\nnet.ipv4.ip_forward=0\n"), 0600)
				if _, err = s.CreateBackup(context.Background(), id); err != nil {
					return err
				}
			}
		}
	}
	_ = strings.Builder{}
	return nil
}
