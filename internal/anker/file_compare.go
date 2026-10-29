package anker

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"
)

const comparisonBytes = 1 << 20
const comparisonLines = 10000
const comparisonCells = 1000000

type MetadataChange struct {
	Field  string `json:"field"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}
type DiffLine struct {
	Kind       string `json:"kind"`
	Text       string `json:"text"`
	BeforeLine int    `json:"before_line,omitempty"`
	AfterLine  int    `json:"after_line,omitempty"`
}
type DiffHunk struct {
	BeforeStart int        `json:"before_start"`
	AfterStart  int        `json:"after_start"`
	Lines       []DiffLine `json:"lines"`
}
type FileComparison struct {
	Path            string           `json:"path"`
	BeforeLabel     string           `json:"before_label"`
	AfterLabel      string           `json:"after_label"`
	State           string           `json:"state"`
	ContentChanged  bool             `json:"content_changed"`
	MetadataChanges []MetadataChange `json:"metadata_changes"`
	Hunks           []DiffHunk       `json:"hunks"`
	Error           string           `json:"error,omitempty"`
	Masked          bool             `json:"masked"`
}

func compareFiles(before, after []byte, beforeEntry, afterEntry *Entry, beforeLabel, afterLabel string) FileComparison {
	return compareFilesVisible(before, after, beforeEntry, afterEntry, beforeLabel, afterLabel, false)
}
func comparisonSecret(data []byte, e *Entry) bool {
	if publicSecretText(string(data)) {
		return true
	}
	if e == nil {
		return false
	}
	return e.Secret || isSecret(e.Path) || publicSecretText(e.Link) || comparisonXAttrsSecret(e.XAttrs)
}

// Xattrs are stored as base64, so inspecting their serialized representation
// cannot establish that metadata is safe. Invalid encodings stay restricted.
func comparisonXAttrsSecret(attrs map[string]string) bool {
	for name, encoded := range attrs {
		if publicSecretText(name+"=value") || base64.StdEncoding.DecodedLen(len(encoded)) > comparisonBytes {
			return true
		}
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || publicSecretText(string(decoded)) {
			return true
		}
	}
	return false
}
func compareFilesVisible(before, after []byte, a, b *Entry, al, bl string, reveal bool) FileComparison {
	c := FileComparison{BeforeLabel: al, AfterLabel: bl, State: "identical", MetadataChanges: []MetadataChange{}, Hunks: []DiffHunk{}}
	if a != nil {
		c.Path = a.Path
	}
	if b != nil {
		c.Path = b.Path
	}
	if !reveal && (comparisonSecret(before, a) || comparisonSecret(after, b)) {
		c.State = "masked"
		c.Masked = true
		return c
	}
	if len(before) > comparisonBytes || len(after) > comparisonBytes {
		c.State = "too_large"
		c.Error = "Vorschau zu groß; vollständigen Download verwenden"
		return c
	}
	if a != nil && a.Type != "file" {
		before = nil
	}
	if b != nil && b.Type != "file" {
		after = nil
	}
	c.ContentChanged = !bytes.Equal(before, after)
	c.MetadataChanges = comparisonMetadata(a, b)
	if c.ContentChanged || len(c.MetadataChanges) > 0 {
		c.State = "different"
	}
	if a == nil && b != nil {
		c.State = "missing_before"
	}
	if b == nil && a != nil {
		c.State = "missing_after"
	}

	if bytes.IndexByte(before, 0) >= 0 || bytes.IndexByte(after, 0) >= 0 || !utf8.Valid(before) || !utf8.Valid(after) {
		c.State = "binary"
		return c
	}
	aa, bb := comparisonTextLines(before), comparisonTextLines(after)
	if len(aa) > comparisonLines || len(bb) > comparisonLines {
		c.State = "too_large"
		c.Error = "Zu viele Zeilen; vollständigen Download verwenden"
		return c
	}
	if !c.ContentChanged {
		return c
	}
	lines, ok := boundedDiff(aa, bb)
	if !ok {
		c.State = "too_large"
		c.Error = "Vergleichsbudget ausgeschöpft; vollständigen Download verwenden"
		return c
	}
	for i := 0; i < len(lines); {
		if lines[i].Kind == "context" {
			i++
			continue
		}
		start := max(0, i-3)
		end := min(len(lines), i+4)
		for j := i + 1; j < min(len(lines), end+3); j++ {
			if lines[j].Kind != "context" {
				end = min(len(lines), j+4)
			}
		}
		h := DiffHunk{Lines: append([]DiffLine(nil), lines[start:end]...)}
		for _, l := range h.Lines {
			if h.BeforeStart == 0 && l.BeforeLine != 0 {
				h.BeforeStart = l.BeforeLine
			}
			if h.AfterStart == 0 && l.AfterLine != 0 {
				h.AfterStart = l.AfterLine
			}
		}
		c.Hunks = append(c.Hunks, h)
		i = end
	}
	return c
}

// Keep line terminators to distinguish empty files and missing final newlines.
func comparisonTextLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	v := strings.SplitAfter(string(data), "\n")
	if v[len(v)-1] == "" {
		v = v[:len(v)-1]
	}
	return v
}

// Trim equal edges first, then use a strictly capped matrix (<= 2 MB). Total
// input, work and output are bounded independently of the size of stored files.
func boundedDiff(a, b []string) ([]DiffLine, bool) {
	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(a)-prefix && suffix < len(b)-prefix && a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		suffix++
	}
	n, m := len(a)-prefix-suffix, len(b)-prefix-suffix
	if (n+1)*(m+1) > comparisonCells {
		return nil, false
	}
	dp := make([]uint16, (n+1)*(m+1))
	width := m + 1
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[prefix+i] == b[prefix+j] {
				dp[i*width+j] = dp[(i+1)*width+j+1] + 1
			} else {
				dp[i*width+j] = max(dp[(i+1)*width+j], dp[i*width+j+1])
			}
		}
	}
	out := []DiffLine{}
	i, j := 0, 0
	add := func(kind, text string) {
		l := DiffLine{Kind: kind, Text: text}
		if kind != "add" {
			i++
			l.BeforeLine = i
		}
		if kind != "remove" {
			j++
			l.AfterLine = j
		}
		out = append(out, l)
	}
	for i < prefix {
		add("context", a[i])
	}
	for i < prefix+n || j < prefix+m {
		if i < prefix+n && j < prefix+m && a[i] == b[j] {
			add("context", a[i])
			continue
		}
		if i < prefix+n && (j == prefix+m || dp[(i-prefix+1)*width+j-prefix] >= dp[(i-prefix)*width+j-prefix+1]) {
			add("remove", a[i])
		} else {
			add("add", b[j])
		}
	}
	for i < len(a) {
		add("context", a[i])
	}
	return out, true
}
func legacyComparisonDiff(c FileComparison) string {
	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n+++ %s\n", c.BeforeLabel, c.AfterLabel)
	for _, h := range c.Hunks {
		fmt.Fprintf(&out, "@@ -%d +%d @@\n", h.BeforeStart, h.AfterStart)
		for _, l := range h.Lines {
			prefix := " "
			if l.Kind == "add" {
				prefix = "+"
			}
			if l.Kind == "remove" {
				prefix = "-"
			}
			out.WriteString(prefix + l.Text)
			if !strings.HasSuffix(l.Text, "\n") {
				out.WriteString("\n\\ No newline at end of file\n")
			}
		}
	}
	return out.String()
}

func comparisonMetadata(a, b *Entry) []MetadataChange {
	changes := []MetadataChange{}
	if a != nil && b != nil {
		fields := []struct {
			name string
			a, b any
		}{{"type", a.Type, b.Type}, {"mode", a.Mode, b.Mode}, {"uid", a.UID, b.UID}, {"gid", a.GID, b.GID}, {"mtime", a.MTime, b.MTime}, {"link", a.Link, b.Link}}
		for _, f := range fields {
			if f.name == "mtime" && (a.MTimeUnknown || b.MTimeUnknown) {
				continue
			}
			if !reflect.DeepEqual(f.a, f.b) {
				changes = append(changes, MetadataChange{f.name, f.a, f.b})
			}
		}
		if !equalXAttrs(a.XAttrs, b.XAttrs) {
			changes = append(changes, MetadataChange{"xattrs", a.XAttrs, b.XAttrs})
		}
	}
	return changes
}
