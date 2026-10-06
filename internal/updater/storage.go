package updater

import (
	"anker/internal/storage"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type StorageManager struct {
	ConfigDir, StateDir string
	mu                  sync.Mutex
	state               storage.GrowthState
	ownerUID            uint32
	inspect             func() (storage.Report, error)
	run                 func(context.Context, string, ...string) ([]byte, error)
	verifyDevice        func(string, string, int64) error
}

func (m *StorageManager) command(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if m.run != nil {
		return m.run(ctx, name, args...)
	}
	return storage.Run(ctx, name, args...)
}
func (m *StorageManager) report() (storage.Report, error) {
	if m.inspect != nil {
		return m.inspect()
	}
	return storage.Inspect("/srv/anker")
}
func (m *StorageManager) statePath() string {
	return filepath.Join(m.StateDir, "storage-operation.json")
}
func (m *StorageManager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, _, err := (&TLSManager{Dir: m.ConfigDir, ownerUID: m.ownerUID}).read(m.statePath(), 64<<10)
	if os.IsNotExist(err) {
		m.state = storage.GrowthState{Status: "idle"}
		return nil
	}
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, &m.state); err != nil {
		return err
	}
	if m.state.Status == "running" {
		m.state.Status = "interrupted"
		m.state.Message = "Erweiterung wurde unterbrochen. Aktuellen Speicher prüfen und den Plan erneut laden."
		return m.save()
	}
	return nil
}
func (m *StorageManager) save() error {
	data, err := json.Marshal(m.state)
	if err != nil {
		return err
	}
	return atomic(m.statePath(), data, 0600, -1, -1)
}
func (m *StorageManager) Status() storage.GrowthState {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.state
	if s.Status == "" {
		s.Status = "idle"
	}
	return s
}

var extGeometry = regexp.MustCompile(`(?m)^Block (count|size):\s+([0-9]+)\s*$`)
var xfsGeometry = regexp.MustCompile(`(?m)^data\s+=\s+bsize=([0-9]+)\s+blocks=([0-9]+)`)

func (m *StorageManager) geometry(p storage.GrowthPlan) (int64, error) {
	if p.FSType == "ext4" {
		data, err := m.command("dumpe2fs", "-h", p.Source)
		if err != nil {
			return 0, errors.New("Dateisystemgröße nicht lesbar; e2fsprogs und Gerätezugriff prüfen")
		}
		var count, size int64
		for _, match := range extGeometry.FindAllStringSubmatch(string(data), -1) {
			n, _ := strconv.ParseInt(match[2], 10, 64)
			if match[1] == "count" {
				count = n
			} else {
				size = n
			}
		}
		if count > 0 && size > 0 && count <= 1<<62/size {
			return count * size, nil
		}
	} else if p.FSType == "xfs" {
		data, err := m.command("xfs_info", p.Mount)
		if err != nil {
			return 0, errors.New("Dateisystemgröße nicht lesbar; xfsprogs und Mount prüfen")
		}
		matches := xfsGeometry.FindStringSubmatch(string(data))
		if len(matches) == 3 {
			size, _ := strconv.ParseInt(matches[1], 10, 64)
			count, _ := strconv.ParseInt(matches[2], 10, 64)
			if count > 0 && size > 0 && count <= 1<<62/size {
				return count * size, nil
			}
		}
	}
	return 0, errors.New("Dateisystemgeometrie ist ungültig")
}
func (m *StorageManager) Plan(volumeID string) (storage.GrowthPlan, error) {
	r, err := m.report()
	if err != nil {
		return storage.GrowthPlan{}, err
	}
	var v storage.Volume
	found := false
	for _, candidate := range r.Volumes {
		if candidate.ID == volumeID {
			v = candidate
			found = true
			break
		}
	}
	if !found || (!v.IsData && !v.IsSystem) {
		return storage.GrowthPlan{}, errors.New("Nur System und Anker-Ablage können erweitert werden")
	}
	target := v.Mount
	if target != "/" && target != "/srv/anker" && !strings.HasPrefix(target, "/srv/anker/") {
		return storage.GrowthPlan{}, errors.New("Kein erlaubter Anker-Mountpoint")
	}
	if strings.ContainsAny(target, "\x00\n\r") {
		return storage.GrowthPlan{}, errors.New("Mountpoint enthält nicht unterstützte Steuerzeichen")
	}
	p := storage.GrowthPlan{VolumeID: v.ID, Mount: target, Source: v.Source, FSType: v.FSType, Environment: r.Environment, Steps: []storage.GrowthStep{}}
	if strings.HasPrefix(r.Environment, "container:") {
		p.Message = "Container-Speicher wird außerhalb des Containers verwaltet. Zugehöriges Dateisystem oder Volume auf dem Host erweitern und anschließend die Anzeige aktualisieren."
		if r.Environment == "container:lxc" {
			p.Message = "Der Container bekommt seinen Platz vom Proxmox-Host. Dort rootfs oder den passenden Mountpoint vergrößern und anschließend die Anzeige aktualisieren."
			p.Steps = append(p.Steps, storage.GrowthStep{Title: "Container auf Proxmox erweitern", Explanation: "CT-ID und rootfs/mp-Nummer im Assistenten angeben. Bindmounts brauchen eine Erweiterung ihres Dateisystems auf dem Host."})
		}
		return p, nil
	}
	if !(strings.HasPrefix(r.Environment, "vm:") || r.Environment == "physical") {
		p.Message = "Umgebung nicht eindeutig erkannt. Erweiterung direkt am System prüfen."
		return p, nil
	}
	if v.Error != "" {
		p.Message = "Dateisystemwerte nicht lesbar: " + v.Error
		return p, nil
	}
	if v.ReadOnly {
		p.Message = "Das Dateisystem ist schreibgeschützt."
		return p, nil
	}
	if p.FSType != "ext4" && p.FSType != "xfs" {
		p.Message = "Dieses Dateisystem wird über seine eigene Speicherverwaltung erweitert."
		return p, nil
	}
	if !strings.HasPrefix(p.Source, "/dev/") || len(p.Source) > 256 || strings.ContainsAny(p.Source, "\x00\n\r") {
		p.Message = "Kein eindeutig zugeordnetes lokales Blockgerät."
		return p, nil
	}
	d, ok := storage.FindDevice(r.Devices, p.Source)
	if !ok {
		if resolved, e := filepath.EvalSymlinks(p.Source); e == nil {
			d, ok = storage.FindDevice(r.Devices, resolved)
		}
	}
	if !ok || d.Size <= 0 {
		p.Message = "Die zugehörige Gerätegröße konnte nicht gelesen werden."
		return p, nil
	}
	if v.UUID == "" || d.UUID != v.UUID {
		p.Message = "Dateisystemidentität nicht eindeutig lesbar. Erweiterung direkt am System prüfen."
		return p, nil
	}
	p.DeviceBytes = d.Size
	verify := m.verifyDevice
	if verify == nil {
		verify = verifyStorageDevice
	}
	if err = verify(p.Source, p.Mount, p.DeviceBytes); err != nil {
		p.Message = err.Error()
		return p, nil
	}
	p.FilesystemBytes, err = m.geometry(p)
	if err != nil {
		p.Message = err.Error()
		return p, nil
	}
	p.Steps = append(p.Steps, storage.GrowthStep{Title: "Aufbau prüfen", Command: "lsblk -o NAME,TYPE,SIZE,FSTYPE,MOUNTPOINTS", Explanation: "Virtuelle Disk und gegebenenfalls Partition oder LVM-Volume müssen zuerst vergrößert sein."})
	if d.Type == "lvm" {
		p.Steps = append(p.Steps, storage.GrowthStep{Title: "LVM prüfen", Command: "sudo pvs; sudo vgs; sudo lvs -o lv_name,vg_name,lv_size,devices", Explanation: "Nach Erweiterung der virtuellen Disk gegebenenfalls Partition und PV vergrößern und dem LV gezielt zusätzliche Größe zuweisen. Anker verändert diese Zuordnung nicht."})
	}
	command := "sudo resize2fs " + storage.ShellQuote(p.Source)
	if p.FSType == "xfs" {
		command = "sudo xfs_growfs -d " + storage.ShellQuote(target)
	}
	p.Steps = append(p.Steps, storage.GrowthStep{Title: "Dateisystem übernimmt den verfügbaren Platz", Command: command, Explanation: "Verwendet den bereits zugewiesenen Platz. Keine Formatierung, Partitionstabellenänderung oder Verkleinerung."})
	if p.DeviceBytes-p.FilesystemBytes < 1<<20 {
		p.Message = "Das Dateisystem nutzt den zugewiesenen Platz bereits. Zuerst Disk, Partition oder LVM-Volume erweitern."
		return p, nil
	}
	p.CanGrow = true
	p.Message = "Zusätzlicher Platz ist dem Blockgerät bereits zugewiesen und kann vom Dateisystem übernommen werden."
	// Only geometry and device identity enter the plan; routine writes do not stale it.
	identity := struct {
		Plan storage.GrowthPlan
		UUID string
	}{p, v.UUID}
	data, _ := json.Marshal(identity)
	p.ID = storage.Identity(string(data))
	return p, nil
}
func (m *StorageManager) Begin(volumeID, planID, confirmation string) (storage.GrowthState, func(), error) {
	unlock, err := (&TLSManager{Dir: m.ConfigDir, ownerUID: m.ownerUID}).lock()
	if err != nil {
		return storage.GrowthState{}, nil, err
	}
	failed := true
	defer func() {
		if failed {
			unlock()
		}
	}()
	p, err := m.Plan(volumeID)
	if err != nil {
		return storage.GrowthState{}, nil, err
	}
	if !p.CanGrow || p.ID == "" || p.ID != planID {
		return storage.GrowthState{}, nil, errors.New("Erweiterungsplan ist nicht mehr gültig; erneut prüfen")
	}
	if confirmation != p.Mount {
		return storage.GrowthState{}, nil, errors.New("Mountpoint zur Bestätigung exakt eingeben")
	}
	m.mu.Lock()
	m.state = storage.GrowthState{Status: "running", StartedAt: time.Now().UTC().Format(time.RFC3339), VolumeID: p.VolumeID, Mount: p.Mount, BeforeBytes: p.FilesystemBytes, Message: "Dateisystem übernimmt zusätzlichen Platz."}
	err = m.save()
	state := m.state
	if err != nil {
		m.state.Status = "failed"
		m.state.Message = "Operationsjournal konnte nicht gespeichert werden; Erweiterung wurde nicht gestartet."
	}
	m.mu.Unlock()
	if err != nil {
		return storage.GrowthState{}, nil, err
	}
	var once sync.Once
	complete := func() {
		once.Do(func() {
			defer unlock()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			run := m.run
			if run == nil {
				run = storage.Run
			}
			name, args := "resize2fs", []string{p.Source}
			if p.FSType == "xfs" {
				name, args = "xfs_growfs", []string{"-d", p.Mount}
			}
			var output []byte
			current, runErr := m.Plan(volumeID)
			if runErr == nil && (current.ID != p.ID || !current.CanGrow) {
				runErr = errors.New("Gerät oder Dateisystem hat sich verändert; Plan erneut prüfen")
			}
			if runErr == nil {
				output, runErr = run(ctx, name, args...)
			}
			after, verifyErr := m.Plan(volumeID)
			if runErr == nil && (verifyErr != nil || after.FilesystemBytes <= p.FilesystemBytes || after.FilesystemBytes > after.DeviceBytes) {
				runErr = errors.New("Erweiterung konnte nicht anhand der neuen Dateisystemgröße bestätigt werden")
			}
			m.mu.Lock()
			defer m.mu.Unlock()
			m.state.FinishedAt = time.Now().UTC().Format(time.RFC3339)
			m.state.AfterBytes = after.FilesystemBytes
			if runErr != nil {
				m.state.Status = "failed"
				detail := strings.TrimSpace(string(output))
				if len(detail) > 2000 {
					detail = detail[:2000]
				}
				m.state.Message = runErr.Error()
				if detail != "" {
					m.state.Message += " · " + detail
				}
			} else {
				m.state.Status = "successful"
				m.state.Message = "Dateisystem erweitert und neue Größe geprüft."
			}
			if err := m.save(); err != nil {
				m.state.Status = "failed"
				m.state.Message = "Ergebnis konnte nicht dauerhaft gespeichert werden; Dateisystemgröße direkt prüfen."
				fmt.Fprintln(os.Stderr, m.state.Message)
			}
		})
	}
	failed = false
	return state, complete, nil
}
