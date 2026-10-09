package anker

// Cold disaster bundles deliberately carry raw catalog rows, not public models.
// They are not normal backup exports and never grant recovery authority.
import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"golang.org/x/crypto/scrypt"
	"golang.org/x/sys/unix"
)

const disasterMagic = "ANKER-DISASTER\x00\x01"
const disasterMaxFile = 64 << 20
const disasterMaxData = 256 << 20
const disasterMaxWire = 384 << 20
const disasterMaxEntries = 50000
const disasterIsolation = ".disaster-isolated"

type DisasterInfo struct {
	Version            int      `json:"version"`
	Mode               string   `json:"mode"`
	Files              int      `json:"files"`
	Records            int      `json:"records"`
	Hosts              int      `json:"hosts"`
	Backups            int      `json:"backups"`
	Plans              int      `json:"plans"`
	ExternalReferences int      `json:"external_references"`
	Prerequisites      []string `json:"prerequisites"`
}
type disasterRecord struct {
	Bucket string `json:"bucket"`
	ID     string `json:"id"`
	Value  []byte `json:"value"`
	SHA256 string `json:"sha256"`
}
type disasterFile struct {
	Path   string `json:"path"`
	Data   []byte `json:"data"`
	SHA256 string `json:"sha256"`
}
type disasterReference struct {
	Kind        string `json:"kind"`
	Path        string `json:"path"`
	Instruction string `json:"instruction"`
}
type disasterPackage struct {
	Version    int                 `json:"version"`
	CreatedAt  string              `json:"created_at"`
	SourceRoot string              `json:"source_root"`
	ConfigRoot string              `json:"config_root,omitempty"`
	Mode       string              `json:"mode"`
	Records    []disasterRecord    `json:"records"`
	Files      []disasterFile      `json:"files"`
	References []disasterReference `json:"references"`
}

// Pin the directory identity; every child is subsequently opened relative to an
// already-open descriptor, with O_NOFOLLOW on every component. EvalSymlinks here
// permits platform aliases (/var and /tmp on macOS), never links in inventory.
type disasterDir struct {
	f        *os.File
	path     string
	supplied string
}

func openDisasterDir(path string) (*disasterDir, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	st, err := os.Lstat(abs)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("Ablage muss ein echtes Verzeichnis sein")
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	for _, part := range strings.Split(strings.TrimPrefix(canonical, "/"), "/") {
		if part == "" {
			continue
		}
		next, e := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(fd)
		if e != nil {
			return nil, e
		}
		fd = next
	}
	d := &disasterDir{os.NewFile(uintptr(fd), canonical), canonical, abs}
	opened, e := d.f.Stat()
	if e != nil || !os.SameFile(st, opened) {
		d.f.Close()
		return nil, errors.New("Ablage vor Öffnung ersetzt")
	}
	if err = d.check(); err != nil {
		d.f.Close()
		return nil, err
	}
	return d, nil
}
func (d *disasterDir) check() error {
	canonical, err := filepath.EvalSymlinks(d.supplied)
	if err != nil || canonical != d.path {
		return errors.New("Ablagepfad während Vorgang verändert")
	}
	old, err := d.f.Stat()
	if err != nil {
		return err
	}
	current, err := os.Lstat(d.path)
	if err != nil {
		return err
	}
	if !os.SameFile(old, current) || !current.IsDir() {
		return errors.New("Ablageidentität während Vorgang verändert")
	}
	return nil
}
func disasterOpenAt(dir *os.File, name string, directory bool) (*os.File, error) {
	flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_CLOEXEC
	if directory {
		flags |= unix.O_DIRECTORY
	}
	fd, err := unix.Openat(int(dir.Fd()), name, flags, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), filepath.Join(dir.Name(), name)), nil
}
func disasterReadFile(f *os.File, limit int64) ([]byte, error) {
	before, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if stat, ok := before.Sys().(*syscall.Stat_t); !ok || stat.Nlink != 1 {
		return nil, errors.New("Mehrfach verknüpfte Quelldatei")
	}
	if !before.Mode().IsRegular() || before.Size() > limit {
		return nil, errors.New("Dateityp oder Größenlimit ungültig")
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || int64(len(data)) != after.Size() {
		return nil, errors.New("Datei verändert oder Größenlimit überschritten")
	}
	return data, nil
}
func disasterAllowed(name string, config bool) bool {
	top := strings.Split(name, "/")[0]
	if config {
		switch top {
		case "keys", "tls":
			return true
		case "known_hosts", "service.env", "tls-renewal.json", "update.json", "setup-complete", "release.pub", "release-key.pub", "release-public.key":
			return name == top
		}
		return strings.HasPrefix(top, "github-token-") && !strings.Contains(name, "/")
	}
	switch top {
	case "config":
		if name == "config" {
			return true
		}
		return disasterAllowed(strings.TrimPrefix(name, "config/"), true)
	case "hosts", "plans", "recovery-inspections", "clusters", "keys", "ssh", "tls":
		return true
	case "known_hosts", "service.env", ".anker-mode":
		return name == top
	}
	return false
}
func disasterWalk(dir *os.File, prefix string, p *disasterPackage, config bool, total *int64, nodes *int) error {
	entries, err := dir.ReadDir(disasterMaxEntries + 1)
	if err == io.EOF {
		err = nil
	}
	*nodes += len(entries)
	if *nodes > disasterMaxEntries || strings.Count(prefix, "/") > 100 {
		return errors.New("Disaster-Verzeichnislimit überschritten")
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		rel := entry.Name()
		if prefix != "" {
			rel = prefix + "/" + rel
		}
		if !disasterAllowed(rel, config) {
			continue
		}
		st, err := entry.Info()
		if err != nil {
			return err
		}
		if st.Mode()&os.ModeSymlink != 0 || (!st.IsDir() && !st.Mode().IsRegular()) {
			return fmt.Errorf("Unsicherer Dateityp in Disaster-Inventar: %s", rel)
		}
		if rel == "plans" || rel == "recovery-inspections" || strings.HasPrefix(rel, "plans/") || strings.HasPrefix(rel, "recovery-inspections/") {
			want := os.FileMode(0600)
			if st.IsDir() {
				want = 0700
			}
			if st.Mode().Perm() != want {
				return fmt.Errorf("Ungeschützter privater Nachweis: %s", rel)
			}
		}
		if stat, ok := st.Sys().(*syscall.Stat_t); !st.IsDir() && (!ok || stat.Nlink != 1) {
			return errors.New("Mehrfach verknüpfte Datei im Disaster-Inventar")
		}
		child, err := disasterOpenAt(dir, entry.Name(), st.IsDir())
		if err != nil {
			return err
		}
		opened, e := child.Stat()
		if e != nil || !os.SameFile(st, opened) {
			child.Close()
			return errors.New("Inventaridentität verändert")
		}
		if st.IsDir() {
			err = disasterWalk(child, rel, p, config, total, nodes)
		} else {
			var data []byte
			data, err = disasterReadFile(child, disasterMaxFile)
			if err == nil {
				*total += int64(len(data))
				if *total > disasterMaxData || len(p.Files) >= disasterMaxEntries {
					err = errors.New("Disaster-Inventarlimit überschritten")
				} else {
					target := rel
					if config {
						target = "disaster-config/" + rel
					}
					p.Files = append(p.Files, disasterFile{target, data, Hash(data)})
				}
			}
		}
		child.Close()
		if err != nil {
			return err
		}
		// Reopen through the same pinned parent to detect replacement after reading.
		check, err := disasterOpenAt(dir, entry.Name(), st.IsDir())
		if err != nil {
			return err
		}
		current, e := check.Stat()
		check.Close()
		if e != nil || !os.SameFile(st, current) {
			return errors.New("Inventar während Lesen ersetzt")
		}
	}
	return nil
}
func disasterCatalog(root *disasterDir) ([]disasterRecord, error) {
	temp, err := os.MkdirTemp("", "anker-cold-catalog-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temp)
	for _, name := range []string{"catalog.db", "catalog.db-wal"} {
		f, e := disasterOpenAt(root.f, name, false)
		if os.IsNotExist(e) && name != "catalog.db" {
			continue
		}
		if e != nil {
			return nil, e
		}
		data, e := disasterReadFile(f, disasterMaxFile)
		f.Close()
		if e != nil {
			return nil, e
		}
		if e = os.WriteFile(filepath.Join(temp, name), data, 0600); e != nil {
			return nil, e
		}
	}
	dsn := url.URL{Scheme: "file", Path: filepath.Join(temp, "catalog.db"), RawQuery: "mode=ro"}
	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	var integrity string
	if err = db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil {
		return nil, err
	}
	if integrity != "ok" {
		return nil, errors.New("Katalogintegrität fehlgeschlagen")
	}
	var extra int
	if err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name NOT IN ('records') AND name NOT LIKE 'sqlite_%'").Scan(&extra); err != nil || extra != 0 {
		return nil, errors.New("Unbekanntes Katalogschema")
	}
	rows, err := db.Query("SELECT bucket,id,value FROM records ORDER BY bucket,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []disasterRecord{}
	total := 0
	for rows.Next() {
		var r disasterRecord
		if err = rows.Scan(&r.Bucket, &r.ID, &r.Value); err != nil {
			return nil, err
		}
		total += len(r.Value)
		if total > disasterMaxFile || len(records) >= disasterMaxEntries || !json.Valid(r.Value) {
			return nil, errors.New("Kataloglimit oder Datensatz ungültig")
		}
		r.SHA256 = Hash(r.Value)
		records = append(records, r)
	}
	return records, rows.Err()
}
func disasterOwned(path, root string) (string, bool) {
	if root == "" {
		return "", false
	}
	rel, err := filepath.Rel(root, path)
	return filepath.ToSlash(rel), err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
func disasterReferences(p *disasterPackage) error {
	files := map[string]bool{}
	for _, f := range p.Files {
		files[f.Path] = true
	}
	seen := map[string]bool{}
	add := func(kind, path string) error {
		if path == "" {
			return nil
		}
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, "\x00\r\n") {
			return errors.New("Zugangspfad muss absolut und normalisiert sein")
		}
		rel, local := disasterOwned(path, p.SourceRoot)
		if !local {
			if c, ok := disasterOwned(path, p.ConfigRoot); ok {
				rel = "disaster-config/" + c
				local = true
			}
		}
		if local {
			if !files[rel] {
				return fmt.Errorf("Erforderliches lokales Material fehlt im Inventar (%s)", kind)
			}
			return nil
		}
		if !seen[kind+path] {
			p.References = append(p.References, disasterReference{kind, path, "Separat sicher bereitstellen; Pfad und SSH-Vertrauen/TLS vor Aktivierung ausdrücklich prüfen. Nicht automatisch enthalten."})
			seen[kind+path] = true
		}
		return nil
	}
	for _, r := range p.Records {
		if r.Bucket == "hosts" {
			var h Host
			if err := json.Unmarshal(r.Value, &h); err != nil {
				return err
			}
			for kind, path := range map[string]string{"backup_key": h.KeyPath, "restore_key": h.RestoreKeyPath, "ssh_trust": h.KnownHostsPath} {
				if err := add(kind, path); err != nil {
					return err
				}
			}
		}
	}
	// Parse data only; never source env files or execute substitutions.
	for _, f := range p.Files {
		check := func(value string) error {
			if strings.HasPrefix(value, "/") && value != p.SourceRoot && value != p.ConfigRoot {
				return add("configuration", value)
			}
			return nil
		}
		configName := strings.TrimPrefix(strings.TrimPrefix(f.Path, "disaster-config/"), "config/")
		if configName == "service.env" {
			for _, line := range strings.Split(string(f.Data), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				_, value, ok := strings.Cut(line, "=")
				if !ok {
					continue
				}
				value = strings.TrimSpace(value)
				if len(value) >= 2 && (value[0] == '"' && value[len(value)-1] == '"' || value[0] == '\'' && value[len(value)-1] == '\'') {
					value = value[1 : len(value)-1]
				}
				if err := check(value); err != nil {
					return err
				}
			}
		} else if configName == "update.json" || configName == "tls-renewal.json" {
			var value any
			if err := json.Unmarshal(f.Data, &value); err != nil {
				return errors.New("Lokale Anker-Konfiguration enthält ungültiges JSON")
			}
			var walk func(any) error
			walk = func(v any) error {
				switch x := v.(type) {
				case string:
					return check(x)
				case map[string]any:
					for _, item := range x {
						if err := walk(item); err != nil {
							return err
						}
					}
				case []any:
					for _, item := range x {
						if err := walk(item); err != nil {
							return err
						}
					}
				}
				return nil
			}
			if err := walk(value); err != nil {
				return err
			}
		}
	}
	sort.Slice(p.References, func(i, j int) bool { a, b := p.References[i], p.References[j]; return a.Kind+a.Path < b.Kind+b.Path })
	return nil
}
func disasterSummary(p *disasterPackage) DisasterInfo {
	v := DisasterInfo{Version: p.Version, Mode: p.Mode, Files: len(p.Files), Records: len(p.Records), ExternalReferences: len(p.References), Prerequisites: []string{"Dienst und Updater stoppen; keine fremden Schreiber", "Frisches isoliertes Datenverzeichnis; ausreichender lokaler Speicher", "Externe Quellen und Konfiguration separat migrieren; SSH-Vertrauen prüfen", "Isolationsmarker erst nach lokaler Prüfung entfernen; Hosts einzeln aktivieren; neue Recovery-Pläne erstellen"}}
	for _, r := range p.Records {
		switch r.Bucket {
		case "hosts":
			v.Hosts++
		case "backups":
			v.Backups++
		case "plans":
			v.Plans++
		}
	}
	return v
}
func disasterCipher(pass, salt []byte) (cipher.AEAD, error) {
	if len(pass) == 0 || len(pass) > 4096 {
		return nil, errors.New("Passphrase muss 1 bis 4096 Bytes enthalten")
	}
	key, err := scrypt.Key(pass, salt, 32768, 8, 1, 32)
	if err != nil {
		return nil, err
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func disasterSeal(p *disasterPackage, pass []byte, w io.Writer) error {
	plain, err := json.Marshal(p)
	if err != nil {
		return err
	}
	defer clear(plain)
	if len(plain) > disasterMaxWire-128 {
		return errors.New("Disaster-Paket zu groß")
	}
	header := make([]byte, len(disasterMagic)+16+12)
	copy(header, disasterMagic)
	if _, err = rand.Read(header[len(disasterMagic):]); err != nil {
		return err
	}
	aead, err := disasterCipher(pass, header[len(disasterMagic):len(disasterMagic)+16])
	if err != nil {
		return err
	}
	ciphertext := aead.Seal(nil, header[len(disasterMagic)+16:], plain, header)
	if _, err = w.Write(header); err != nil {
		return err
	}
	_, err = w.Write(ciphertext)
	return err
}
func disasterUnseal(r io.Reader, pass []byte) (*disasterPackage, error) {
	wire, err := io.ReadAll(io.LimitReader(r, disasterMaxWire+1))
	if err != nil {
		return nil, err
	}
	hs := len(disasterMagic) + 28
	if len(wire) > disasterMaxWire || len(wire) < hs+16 || string(wire[:len(disasterMagic)]) != disasterMagic {
		return nil, errors.New("Unbekanntes oder zu großes Disaster-Format")
	}
	aead, err := disasterCipher(pass, wire[len(disasterMagic):len(disasterMagic)+16])
	if err != nil {
		return nil, err
	}
	plain, err := aead.Open(nil, wire[len(disasterMagic)+16:hs], wire[hs:], wire[:hs])
	if err != nil {
		return nil, errors.New("Disaster-Authentifizierung fehlgeschlagen (Passphrase oder Paket)")
	}
	defer clear(plain)
	var p disasterPackage
	dec := json.NewDecoder(bytes.NewReader(plain))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&p); err != nil {
		return nil, err
	}
	if err = dec.Decode(new(any)); err != io.EOF {
		return nil, errors.New("Zusätzliche Paketdaten")
	}
	if err = disasterValidate(&p); err != nil {
		return nil, err
	}
	return &p, nil
}
func disasterValidate(p *disasterPackage) error {
	if p.Version != 1 || (p.Mode != "production" && p.Mode != "demo") || !filepath.IsAbs(p.SourceRoot) || filepath.Clean(p.SourceRoot) != p.SourceRoot || len(p.Files) > disasterMaxEntries || len(p.Records) > disasterMaxEntries {
		return errors.New("Ungültiges Disaster-Schema")
	}
	if p.ConfigRoot != "" && (!filepath.IsAbs(p.ConfigRoot) || filepath.Clean(p.ConfigRoot) != p.ConfigRoot) {
		return errors.New("Ungültige Konfigurationswurzel")
	}
	paths := map[string]bool{}
	total := int64(0)
	for _, f := range p.Files {
		if ValidPath(f.Path) != nil || paths[f.Path] || len(f.Data) > disasterMaxFile || Hash(f.Data) != f.SHA256 {
			return errors.New("Disaster-Dateiinventar oder Prüfsumme ungültig")
		}
		allowed := disasterAllowed(f.Path, false)
		if strings.HasPrefix(f.Path, "disaster-config/") {
			allowed = disasterAllowed(strings.TrimPrefix(f.Path, "disaster-config/"), true)
		}
		if !allowed {
			return errors.New("Datei außerhalb Disaster-Umfang")
		}
		paths[f.Path] = true
		total += int64(len(f.Data))
		if total > disasterMaxData {
			return errors.New("Disaster-Datenlimit überschritten")
		}
	}
	for path := range paths {
		for parent := filepath.ToSlash(filepath.Dir(path)); parent != "."; parent = filepath.ToSlash(filepath.Dir(parent)) {
			if paths[parent] {
				return errors.New("Datei-/Verzeichniskollision")
			}
		}
	}
	if !paths[".anker-mode"] {
		return errors.New("Modusmarker fehlt")
	}
	for _, f := range p.Files {
		if f.Path == ".anker-mode" && strings.TrimSpace(string(f.Data)) != p.Mode {
			return errors.New("Modusmarker widerspricht Paket")
		}
	}
	rows := map[string]bool{}
	size := 0
	for _, r := range p.Records {
		key := r.Bucket + "\x00" + r.ID
		size += len(r.Value)
		if r.Bucket == "" || r.ID == "" || strings.ContainsAny(r.Bucket+r.ID, "\x00") || rows[key] || !json.Valid(r.Value) || Hash(r.Value) != r.SHA256 || size > disasterMaxFile {
			return errors.New("Kataloginventar oder Prüfsumme ungültig")
		}
		rows[key] = true
		switch r.Bucket {
		case "hosts", "jobs", "backups", "plans", "recovery-inspections", "users", "credentials", "settings":
			var object map[string]json.RawMessage
			if err := json.Unmarshal(r.Value, &object); err != nil || object == nil {
				return errors.New("Katalogobjekt fehlt oder ist ungültig")
			}
			if r.Bucket == "hosts" || r.Bucket == "jobs" {
				var id string
				if json.Unmarshal(object["id"], &id) != nil || id != r.ID {
					return errors.New("Katalogobjekt-ID widerspricht Inventar")
				}
			}
		}
		if r.Bucket == "hosts" || r.Bucket == "backups" || r.Bucket == "plans" || r.Bucket == "recovery-inspections" {
			if !validID(r.ID) {
				return errors.New("Ungültige Katalog-ID")
			}
		}
	}
	original := p.References
	p.References = nil
	if err := disasterReferences(p); err != nil {
		return err
	}
	want, _ := json.Marshal(p.References)
	got, _ := json.Marshal(original)
	if !bytes.Equal(want, got) {
		return errors.New("Externe Referenzen widersprechen Katalog")
	}
	return nil
}

func ExportDisaster(root, configRoot string, pass []byte, w io.Writer) error {
	d, err := openDisasterDir(root)
	if err != nil {
		return err
	}
	defer d.f.Close()
	unlock, err := InstanceLock(d.path)
	if err != nil {
		return err
	}
	defer unlock()
	if err = d.check(); err != nil {
		return err
	}
	p := &disasterPackage{Version: 1, CreatedAt: now(), SourceRoot: d.supplied}
	p.Records, err = disasterCatalog(d)
	if err != nil {
		return err
	}
	total := int64(0)
	nodes := 0
	if err = disasterWalk(d.f, "", p, false, &total, &nodes); err != nil {
		return err
	}
	for _, f := range p.Files {
		if f.Path == ".anker-mode" {
			p.Mode = strings.TrimSpace(string(f.Data))
		}
	}
	if configRoot != "" {
		c, e := openDisasterDir(configRoot)
		if e != nil {
			return e
		}
		defer c.f.Close()
		p.ConfigRoot = c.supplied
		if err = disasterWalk(c.f, "", p, true, &total, &nodes); err != nil {
			return err
		}
		if err = c.check(); err != nil {
			return err
		}
	}
	if err = disasterReferences(p); err != nil {
		return err
	}
	if err = disasterValidate(p); err != nil {
		return err
	}
	// Validate exactly the captured bytes, never chase catalog paths in the source.
	stage, err := os.MkdirTemp("", "anker-disaster-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err = disasterMaterialize(p, stage); err != nil {
		return err
	}
	if err = disasterCheckMaterial(p, stage); err != nil {
		return err
	}
	if err = d.check(); err != nil {
		return err
	}
	return disasterSeal(p, pass, w)
}
func InspectDisaster(r io.Reader, pass []byte) (DisasterInfo, error) {
	p, err := disasterUnseal(r, pass)
	if err != nil {
		return DisasterInfo{}, err
	}
	return disasterSummary(p), nil
}
func disasterMaterialize(p *disasterPackage, root string) error {
	for _, f := range p.Files {
		if err := atomicWrite(filepath.Join(root, filepath.FromSlash(f.Path)), f.Data, 0600); err != nil {
			return err
		}
	}
	store, err := OpenStore(filepath.Join(root, "catalog.db"))
	if err != nil {
		return err
	}
	defer store.Close()
	tx, err := store.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range p.Records {
		if _, err = tx.Exec("INSERT INTO records(bucket,id,value) VALUES(?,?,?)", r.Bucket, r.ID, []byte(r.Value)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Bound cumulative decompression before the established backup verifier runs.
func disasterCheckArchives(p *disasterPackage) error {
	var total int64
	entries := 0
	for _, file := range p.Files {
		if !strings.HasSuffix(file.Path, "/archive.tar.gz") {
			continue
		}
		gz, err := gzip.NewReader(bytes.NewReader(file.Data))
		if err != nil {
			return err
		}
		tr := tar.NewReader(io.LimitReader(gz, disasterMaxData+1024*disasterMaxEntries+1))
		seen := map[string]bool{}
		for {
			h, e := tr.Next()
			if e == io.EOF {
				break
			}
			if e != nil {
				gz.Close()
				return e
			}
			entries++
			name := strings.TrimSuffix(h.Name, "/")
			total += h.Size
			if entries > disasterMaxEntries || h.Size < 0 || h.Size > disasterMaxFile || total > disasterMaxData || ValidPath(name) != nil || seen[name] || (h.Typeflag != tar.TypeDir && h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA) {
				gz.Close()
				return errors.New("Sicherungsarchiv überschreitet Disaster-Grenzen oder enthält unsichere Einträge")
			}
			seen[name] = true
		}
		gz.Close()
	}
	return nil
}
func disasterCheckMaterial(p *disasterPackage, root string) error {
	if err := disasterCheckArchives(p); err != nil {
		return err
	}
	store, err := OpenStore(filepath.Join(root, "catalog.db"))
	if err != nil {
		return err
	}
	defer store.Close()
	s := &Service{Root: root, Store: store}
	if err = os.MkdirAll(filepath.Join(root, "staging"), 0700); err != nil {
		return err
	}
	for _, r := range p.Records {
		switch r.Bucket {
		case "backups":
			var b Backup
			if err = json.Unmarshal(r.Value, &b); err != nil {
				return err
			}
			if !validID(b.ID) || !validID(b.HostID) || b.ID != r.ID {
				return errors.New("Ungültiger Sicherungseintrag")
			}
			if err = s.checkBackupUnlocked(b.ID); err != nil {
				return fmt.Errorf("Sicherungsmaterial fehlt oder ist ungültig (%s): %w", b.ID, err)
			}
		case "plans":
			var plan Plan
			if err = json.Unmarshal(r.Value, &plan); err != nil {
				return err
			}
			if plan.ID != r.ID || !validID(plan.BackupID) || !validID(plan.TargetID) {
				return errors.New("Ungültiger Planeintrag")
			}
			if err = s.verifyPlanFiles(plan); err != nil {
				return fmt.Errorf("Privates Planmaterial fehlt oder ist ungültig (%s): %w", plan.ID, err)
			}
		case "recovery-inspections":
			var record recoverySnapshotRecord
			if err = json.Unmarshal(r.Value, &record); err != nil {
				return err
			}
			data, e := os.ReadFile(filepath.Join(root, "recovery-inspections", r.ID, "snapshot.json"))
			if e != nil || Hash(data) != record.SHA256 {
				return errors.New("Privater Zielnachweis fehlt oder ist verändert")
			}
			var snapshot RecoverySnapshot
			if err = json.Unmarshal(data, &snapshot); err != nil {
				return err
			}
			if snapshot.ID != r.ID || record.ID != r.ID {
				return errors.New("Ungültiger Zielnachweis")
			}
			for _, file := range snapshot.Files {
				if file.State != "read" {
					continue
				}
				if ValidPath(file.Path) != nil {
					return errors.New("Ungültiger Zielnachweispfad")
				}
				content, e := readSnapshotBytes(filepath.Join(root, "recovery-inspections", r.ID, "files", file.Path), 1<<20)
				if e != nil || file.Metadata == nil || Hash(content) != file.SHA256 || int64(len(content)) != file.Metadata.Size {
					return errors.New("Privater Zielinhalt fehlt oder ist verändert")
				}
			}
		}
	}
	return nil
}

// A fresh root must not be nested inside another (possibly live) instance.
func disasterIsolatedParent(parent *os.File) error {
	fd, err := unix.Dup(int(parent.Fd()))
	if err != nil {
		return err
	}
	for {
		for _, name := range []string{"catalog.db", ".anker-mode", "service.lock"} {
			var st unix.Stat_t
			err = unix.Fstatat(fd, name, &st, unix.AT_SYMLINK_NOFOLLOW)
			if err == nil {
				unix.Close(fd)
				return errors.New("Importziel darf nicht innerhalb einer bestehenden Anker-Instanz liegen")
			}
			if !errors.Is(err, unix.ENOENT) {
				unix.Close(fd)
				return err
			}
		}
		var current unix.Stat_t
		if err = unix.Fstat(fd, &current); err != nil {
			unix.Close(fd)
			return err
		}
		next, e := unix.Openat(fd, "..", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(fd)
		if e != nil {
			return e
		}
		fd = next
		var ancestor unix.Stat_t
		if err = unix.Fstat(fd, &ancestor); err != nil {
			unix.Close(fd)
			return err
		}
		if current.Dev == ancestor.Dev && current.Ino == ancestor.Ino {
			unix.Close(fd)
			return nil
		}
	}
}
func ImportDisaster(r io.Reader, dest string, pass []byte) (DisasterInfo, error) {
	p, err := disasterUnseal(r, pass)
	if err != nil {
		return DisasterInfo{}, err
	}
	abs, err := filepath.Abs(dest)
	if err != nil {
		return DisasterInfo{}, err
	}
	if filepath.Clean(abs) == "/" {
		return DisasterInfo{}, errors.New("Ungültiges Importziel")
	}
	parent, err := openDisasterDir(filepath.Dir(abs))
	if err != nil {
		return DisasterInfo{}, err
	}
	defer parent.f.Close()
	if err = disasterIsolatedParent(parent.f); err != nil {
		return DisasterInfo{}, err
	}
	name := filepath.Base(abs)
	target := filepath.Join(parent.path, name)
	if _, err = os.Lstat(target); !os.IsNotExist(err) {
		return DisasterInfo{}, errors.New("Importziel muss neu sein und darf noch nicht existieren")
	}
	// Preflight and SQLite work take place in a private temporary directory.
	// The final destination is touched only through descriptor-confined handles.
	stage, err := os.MkdirTemp("", "anker-disaster-preflight-")
	if err != nil {
		return DisasterInfo{}, err
	}
	defer os.RemoveAll(stage)
	unlock, err := InstanceLock(stage)
	if err != nil {
		return DisasterInfo{}, err
	}
	defer unlock()
	if err = disasterMaterialize(p, stage); err != nil {
		return DisasterInfo{}, err
	}
	if err = disasterCheckMaterial(p, stage); err != nil {
		return DisasterInfo{}, err
	}
	if err = disasterIsolate(p, stage, target); err != nil {
		return DisasterInfo{}, err
	}
	handle, err := os.OpenRoot(parent.path)
	if err != nil {
		return DisasterInfo{}, err
	}
	defer handle.Close()
	approved, err := parent.f.Stat()
	if err != nil {
		return DisasterInfo{}, err
	}
	actual, err := handle.Stat(".")
	if err != nil || !os.SameFile(approved, actual) {
		return DisasterInfo{}, errors.New("Import-Elternverzeichnis ersetzt")
	}
	stageName := ".anker-disaster-" + ID()
	if err = handle.Mkdir(stageName, 0700); err != nil {
		return DisasterInfo{}, err
	}
	defer handle.RemoveAll(stageName)
	confined, err := handle.OpenRoot(stageName)
	if err != nil {
		return DisasterInfo{}, err
	}
	defer confined.Close()
	if err = disasterCopyStage(stage, confined); err != nil {
		return DisasterInfo{}, err
	}
	lock, err := confined.OpenFile("service.lock", os.O_RDWR|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return DisasterInfo{}, err
	}
	defer lock.Close()
	if err = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return DisasterInfo{}, err
	}
	defer unix.Flock(int(lock.Fd()), unix.LOCK_UN)
	if err = parent.check(); err != nil {
		return DisasterInfo{}, err
	}
	if err = disasterIsolatedParent(parent.f); err != nil {
		return DisasterInfo{}, err
	}
	stageInfo, err := confined.Stat(".")
	if err != nil {
		return DisasterInfo{}, err
	}
	current, err := handle.Lstat(stageName)
	if err != nil || !os.SameFile(stageInfo, current) {
		return DisasterInfo{}, errors.New("Import-Staging ersetzt")
	}
	if err = disasterPublish(int(parent.f.Fd()), stageName, name); err != nil {
		return DisasterInfo{}, err
	}
	if err = parent.f.Sync(); err != nil {
		return DisasterInfo{}, err
	}
	if err = parent.check(); err != nil {
		return DisasterInfo{}, err
	}
	published, err := handle.Lstat(name)
	if err != nil || !os.SameFile(stageInfo, published) {
		return DisasterInfo{}, errors.New("Veröffentlichte Importablage verändert")
	}
	return disasterSummary(p), nil
}

// Stage is private and never sourced from an archive pathname. Destination
// operations remain rooted even if an ancestor is concurrently renamed.
func disasterCopyStage(source string, dest *os.Root) error {
	dirs := []string{"."}
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil || path == source {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			dirs = append(dirs, rel)
			return dest.Mkdir(rel, 0700)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("Link im privaten Staging")
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := dest.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_EXCL|unix.O_NOFOLLOW, 0600)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, in)
		if err == nil {
			err = out.Sync()
		}
		return errors.Join(err, out.Close())
	})
	if err != nil {
		return err
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		f, e := dest.Open(dirs[i])
		if e != nil {
			return e
		}
		e = f.Sync()
		f.Close()
		if e != nil {
			return e
		}
	}
	return nil
}
func disasterIsolate(p *disasterPackage, root, target string) error {
	store, err := OpenStore(filepath.Join(root, "catalog.db"))
	if err != nil {
		return err
	}
	defer store.Close()
	tx, err := store.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM records WHERE bucket='sessions'"); err != nil {
		return err
	}
	for _, r := range p.Records {
		switch r.Bucket {
		case "hosts", "jobs":
			var object map[string]json.RawMessage
			if err = json.Unmarshal(r.Value, &object); err != nil {
				return err
			}
			if object == nil {
				return errors.New("Leeres Importobjekt")
			}
			if r.Bucket == "hosts" {
				object["enabled"] = json.RawMessage("false")
				for _, field := range []string{"key_path", "restore_key_path", "known_hosts_path"} {
					var path string
					if err = json.Unmarshal(object[field], &path); err != nil && object[field] != nil {
						return err
					}
					if rel, ok := disasterOwned(path, p.SourceRoot); ok {
						object[field], _ = json.Marshal(filepath.Join(target, rel))
					} else if rel, ok := disasterOwned(path, p.ConfigRoot); ok {
						object[field], _ = json.Marshal(filepath.Join(target, "disaster-config", rel))
					}
				}
			} else {
				var state string
				json.Unmarshal(object["state"], &state)
				if state == "queued" || state == "running" {
					object["state"] = json.RawMessage(`"interrupted"`)
					object["error"] = json.RawMessage(`"Disaster-Import: Auftrag wird nicht fortgesetzt"`)
					object["finished_at"], _ = json.Marshal(now())
				}
			}
			data, err := json.Marshal(object)
			if err != nil {
				return err
			}
			if _, err = tx.Exec("UPDATE records SET value=? WHERE bucket=? AND id=?", data, r.Bucket, r.ID); err != nil {
				return err
			}
		case "recovery-inspections":
			if err = txPut(tx, "disaster-revoked-inspections", r.ID, map[string]string{"imported_at": now()}); err != nil {
				return err
			}
		case "plans":
			if err = txPut(tx, "disaster-revoked-plans", r.ID, map[string]string{"imported_at": now(), "reason": "Alter Plan bleibt Nachweis; neue Zielprüfung und neuer Plan erforderlich"}); err != nil {
				return err
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	report := struct {
		Version    int                 `json:"version"`
		ImportedAt string              `json:"imported_at"`
		References []disasterReference `json:"external_references"`
		SourceRoot string              `json:"source_root"`
		ConfigRoot string              `json:"config_root"`
	}{1, now(), p.References, p.SourceRoot, p.ConfigRoot}
	if err = writeJSON(filepath.Join(root, "disaster-migration.json"), report); err != nil {
		return err
	}
	return atomicWrite(filepath.Join(root, disasterIsolation), []byte("Isolierter Disaster-Import. OPERATIONS.md: Material und externe Referenzen prüfen; alte Pläne bleiben gesperrt.\n"), 0600)
}

// Revocation lives separately so private plan bytes and their evidence hashes
// remain unchanged. It survives activation and later exports.
func (s *Service) disasterPlanAllowed(id string) error {
	var value any
	err := s.Store.Get("disaster-revoked-plans", id, &value)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("Importierter Plan bleibt Nachweis; vor Hostzugriff neue Zielprüfung und neuen Plan erstellen")
}
