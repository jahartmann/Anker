package anker

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"path"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode"
)

const packageIndexName = "package-index.json"
const packageChecksumsName = "package-checksums.sha256"
const packageMaxEntries = 50000
const packageMaxBytes int64 = 256 << 20
const packageMaxFile int64 = 64 << 20

type tarEntryWriter interface {
	io.Writer
	WriteHeader(*tar.Header) error
}
type packageEntry struct {
	Path   string            `json:"path"`
	Type   byte              `json:"type"`
	Size   int64             `json:"size"`
	Mode   int64             `json:"mode"`
	UID    int               `json:"uid"`
	GID    int               `json:"gid"`
	MTime  int64             `json:"mtime"`
	Link   string            `json:"link,omitempty"`
	XAttrs map[string]string `json:"xattrs_base64,omitempty"`
	SHA256 string            `json:"sha256,omitempty"`
}
type packageIndex struct {
	Version int            `json:"version"`
	Entries []packageEntry `json:"entries"`
}

func packageMetadata(h *tar.Header) packageEntry {
	typ := h.Typeflag
	if typ == tar.TypeRegA {
		typ = tar.TypeReg
	}
	// Xattr values are arbitrary bytes. Standard padded base64 preserves them
	// across JSON serialization, both when indexing and when verifying raw tar.
	// Never mutate the header: original tar attributes retain their exact bytes.
	xattrs := make(map[string]string, len(h.Xattrs))
	// Go's tar reader leaves empty PAX xattrs out of Header.Xattrs but keeps
	// their explicit records. Include that byte evidence, including empty values.
	for key, value := range h.PAXRecords {
		if name, ok := strings.CutPrefix(key, "SCHILY.xattr."); ok {
			xattrs[name] = base64.StdEncoding.EncodeToString([]byte(value))
		}
	}
	// The tar writer also gives Header.Xattrs precedence over PAXRecords.
	for name, value := range h.Xattrs {
		xattrs[name] = base64.StdEncoding.EncodeToString([]byte(value))
	}
	if len(xattrs) == 0 {
		xattrs = nil
	}

	return packageEntry{Path: h.Name, Type: typ, Size: h.Size, Mode: h.Mode, UID: h.Uid, GID: h.Gid, MTime: h.ModTime.Unix(), Link: h.Linkname, XAttrs: xattrs}
}

type indexedPlanTar struct {
	*tar.Writer
	entries []packageEntry
	digest  hash.Hash
	total   int64
	seen    map[string]byte
	parents map[string]bool
}

func (w *indexedPlanTar) finishEntry() {
	if w.digest != nil {
		w.entries[len(w.entries)-1].SHA256 = hex.EncodeToString(w.digest.Sum(nil))
		w.digest = nil
	}
}
func (w *indexedPlanTar) WriteHeader(h *tar.Header) error {
	w.finishEntry()
	if h.Name == packageIndexName || h.Name == packageChecksumsName {
		return errors.New("reservierter Paketpfad")
	}
	if len(w.entries) >= packageMaxEntries-2 || h.Size > packageMaxFile || h.Size > packageMaxBytes-w.total {
		return errors.New("Planpaket überschreitet Exportgrenzen")
	}
	if w.seen == nil {
		w.seen = map[string]byte{}
		w.parents = map[string]bool{}
	}
	if err := validatePackageHeader(h, w.seen, w.parents); err != nil {
		return err
	}
	if err := w.Writer.WriteHeader(h); err != nil {
		return err
	}
	entry := packageMetadata(h)
	// archive/tar rounds implicit-format timestamps; index the serialized value.
	if h.Format == tar.FormatUnknown {
		entry.MTime = h.ModTime.Round(time.Second).Unix()
	}
	w.entries = append(w.entries, entry)
	w.total += h.Size
	if h.Typeflag == tar.TypeReg || h.Typeflag == tar.TypeRegA {
		w.digest = sha256.New()
	}
	return nil
}
func (w *indexedPlanTar) Write(b []byte) (int, error) {
	n, err := w.Writer.Write(b)
	if w.digest != nil {
		w.digest.Write(b[:n])
	}
	return n, err
}
func (w *indexedPlanTar) Close() error {
	w.finishEntry()
	index, err := json.MarshalIndent(packageIndex{Version: 1, Entries: w.entries}, "", "  ")
	if err != nil {
		return err
	}
	if int64(len(index)) > packageMaxFile || w.total+int64(len(index)) > packageMaxBytes {
		return errors.New("Paketindex überschreitet Exportgrenze")
	}
	if err = writePackageFile(w.Writer, packageIndexName, index); err != nil {
		return err
	}
	sums := map[string]string{packageIndexName: Hash(index)}
	for _, e := range w.entries {
		if e.Type == tar.TypeReg {
			sums[e.Path] = e.SHA256
		}
	}
	names := make([]string, 0, len(sums))
	for n := range sums {
		names = append(names, n)
	}
	sort.Strings(names)
	var checks strings.Builder
	for _, n := range names {
		fmt.Fprintf(&checks, "%s  %s\n", sums[n], n)
	}
	if int64(checks.Len()) > packageMaxFile || w.total+int64(len(index)+checks.Len()) > packageMaxBytes {
		return errors.New("Paketprüfsummen überschreiten Exportgrenze")
	}
	if err = writePackageFile(w.Writer, packageChecksumsName, []byte(checks.String())); err != nil {
		return err
	}
	return w.Writer.Close()
}
func writePackageFile(w tarEntryWriter, name string, b []byte) error {
	if err := w.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0600, Size: int64(len(b))}); err != nil {
		return err
	}
	_, err := w.Write(b)
	return err
}
func validPackagePath(n string) bool {
	return n != "" && len(n) <= 4096 && strings.Count(n, "/") <= 100 && path.Clean(n) == n && !strings.HasPrefix(n, "/") && n != "." && n != ".." && !strings.HasPrefix(n, "../") && !strings.ContainsAny(n, "\\:") && !strings.ContainsFunc(n, unicode.IsControl)
}
func validatePackageHeader(h *tar.Header, seen map[string]byte, parents map[string]bool) error {
	if !validPackagePath(h.Name) || h.Size < 0 {
		return errors.New("unsicherer Paketpfad oder Größe")
	}
	if _, ok := seen[h.Name]; ok {
		return errors.New("doppelter Paketpfad")
	}
	typ := h.Typeflag
	if typ == tar.TypeRegA {
		typ = tar.TypeReg
	}
	if typ != tar.TypeReg && typ != tar.TypeDir && typ != tar.TypeSymlink {
		return errors.New("unzulässiger Archiveintrag")
	}
	if typ != tar.TypeReg && h.Size != 0 {
		return errors.New("unerwarteter Eintragsinhalt")
	}
	if typ == tar.TypeSymlink && !strings.HasPrefix(h.Name, "original/original-files/") {
		return errors.New("Link außerhalb Originalmetadaten")
	}
	for parent := path.Dir(h.Name); parent != "."; parent = path.Dir(parent) {
		if t, ok := seen[parent]; ok && t != tar.TypeDir {
			return errors.New("Paketpfad unter Datei oder Link")
		}
	}
	if typ != tar.TypeDir && parents[h.Name] {
		return errors.New("Paketpfad kollidiert mit Unterpfad")
	}
	for parent := path.Dir(h.Name); parent != "."; parent = path.Dir(parent) {
		parents[parent] = true
	}

	seen[h.Name] = typ
	return nil
}

// PackageVerification describes internal coverage only. No signature or runtime
// recovery result is inferred from a self-contained checksum list.
type PackageVerification struct {
	Coverage         string `json:"coverage"`
	Entries          int    `json:"entries"`
	PlanID           string `json:"plan_id"`
	BackupID         string `json:"backup_id"`
	TargetID         string `json:"target_id"`
	TrustedOrigin    bool   `json:"trusted_origin"`
	RecoveryVerified bool   `json:"recovery_verified"`
	Note             string `json:"note"`
}

// VerifyPlanPackage streams a tar package without extracting or following links.
func VerifyPlanPackage(r io.Reader) (PackageVerification, error) {
	return verifyPlanPackage(r, packageMaxEntries, packageMaxBytes)
}
func verifyPlanPackage(r io.Reader, maxEntries int, maxBytes int64) (PackageVerification, error) {
	result := PackageVerification{}
	wire := &io.LimitedReader{R: r, N: 512<<20 + 1}
	tr := tar.NewReader(wire)
	seen := map[string]byte{}
	parents := map[string]bool{}
	actual := map[string]packageEntry{}
	saved := map[string][]byte{}
	var total int64
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return result, err
		}
		if len(seen) >= maxEntries || h.Size > packageMaxFile || h.Size > maxBytes-total {
			return result, errors.New("Paketgrenze überschritten (Einträge/Bytes)")
		}
		if err = validatePackageHeader(h, seen, parents); err != nil {
			return result, err
		}
		total += h.Size
		e := packageMetadata(h)
		if e.Type == tar.TypeReg {
			digest := sha256.New()
			var buf bytes.Buffer
			var dest io.Writer = digest
			switch h.Name {
			case packageIndexName, packageChecksumsName, "original/checksums.sha256", "plan.json":
				dest = io.MultiWriter(digest, &buf)
			}
			if _, err = io.Copy(dest, tr); err != nil {
				return result, err
			}
			e.SHA256 = hex.EncodeToString(digest.Sum(nil))
			if buf.Len() > 0 {
				saved[h.Name] = buf.Bytes()
			}
		}
		actual[h.Name] = e
	}
	// Reject appended archives or non-padding data instead of silently ignoring it.
	tail := make([]byte, 32768)
	for {
		n, err := wire.Read(tail)
		for _, b := range tail[:n] {
			if b != 0 {
				return result, errors.New("Daten hinter Archivende")
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return result, err
		}
	}
	if wire.N <= 0 {
		return result, errors.New("Paket überschreitet Transportgrenze")
	}
	for _, n := range []string{"plan.json", "mapping.json", "manifest.json", "WIEDERHERSTELLUNG.md", "original/manifest.json", "original/checksums.sha256", "original/inventory/host.json"} {
		if e, ok := actual[n]; !ok || e.Type != tar.TypeReg {
			return result, fmt.Errorf("Paketbestandteil fehlt: %s", n)
		}
	}
	var p Plan
	if err := json.Unmarshal(saved["plan.json"], &p); err != nil {
		return result, err
	}
	if p.ID == "" || p.BackupID == "" || p.TargetID == "" {
		return result, errors.New("Planbindung fehlt")
	}
	if p.TargetSnapshotID != "" {
		if e, ok := actual["target-snapshot.json"]; !ok || e.Type != tar.TypeReg {
			return result, errors.New("Zielaufnahme fehlt")
		}
	}
	result.PlanID = p.ID
	result.BackupID = p.BackupID
	result.TargetID = p.TargetID
	result.Entries = len(actual)
	if err := checkPackageSums(saved["original/checksums.sha256"], "original/", actual, false); err != nil {
		return result, err
	}
	_, hasIndex := actual[packageIndexName]
	_, hasSums := actual[packageChecksumsName]
	if !hasIndex && !hasSums {
		result.Coverage = "limited"
		result.Note = "Altexport: nur gelistete Quellprüfsummen geprüft; vollständige Paketabdeckung, Herkunft und Wiederherstellung unbewiesen."
		return result, nil
	}
	if !hasIndex || !hasSums {
		return result, errors.New("Paketindex oder Paketprüfsummen fehlen")
	}
	var index packageIndex
	dec := json.NewDecoder(bytes.NewReader(saved[packageIndexName]))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&index); err != nil {
		return result, err
	}
	if dec.Decode(new(any)) != io.EOF {
		return result, errors.New("ungültiges Indexende")
	}
	if index.Version != 1 || len(index.Entries) != len(actual)-2 {
		return result, errors.New("Paketindex unvollständig oder unbekannte Version")
	}
	indexed := map[string]bool{}
	for _, e := range index.Entries {
		a, ok := actual[e.Path]
		if !ok || indexed[e.Path] || e.Path == packageIndexName || e.Path == packageChecksumsName || !reflect.DeepEqual(a, e) {
			return result, fmt.Errorf("Paketinhalt oder Metadaten weichen vom Index ab: %q", e.Path)
		}
		indexed[e.Path] = true
	}
	if err := checkPackageSums(saved[packageChecksumsName], "", actual, true); err != nil {
		return result, err
	}
	result.Coverage = "complete"
	result.Note = "Interne Paketprüfsummen vollständig; Herkunft/Authentizität und erfolgreiche Wiederherstellung nicht nachgewiesen. Originalsymlinks nur als Metadaten geprüft, nie verfolgt."
	return result, nil
}
func checkPackageSums(raw []byte, prefix string, actual map[string]packageEntry, complete bool) error {
	checked := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		if len(line) < 67 || line[64:66] != "  " {
			return errors.New("ungültige Prüfsummenliste")
		}
		sum, n := line[:64], line[66:]
		if _, err := hex.DecodeString(sum); err != nil || !validPackagePath(n) {
			return errors.New("ungültige Prüfsumme oder Pfad")
		}
		name := prefix + n
		e, ok := actual[name]
		if !ok || e.Type != tar.TypeReg || checked[name] || e.SHA256 != sum {
			return errors.New("fehlender oder veränderter Prüfsummenbestandteil")
		}
		checked[name] = true
	}
	if complete {
		for n, e := range actual {
			if e.Type == tar.TypeReg && n != packageChecksumsName && !checked[n] {
				return errors.New("unvollständige Paketprüfsummen")
			}
		}
	}
	return nil
}
