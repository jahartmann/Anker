package anker

import (
	"strings"
	"testing"
)

func TestComparisonMetadataOnly(t *testing.T) {
	for _, tc := range []struct {
		field  string
		change func(*Entry)
	}{
		{"mode", func(e *Entry) { e.Mode = 0644 }},
		{"uid", func(e *Entry) { e.UID = 12 }},
		{"gid", func(e *Entry) { e.GID = 15 }},
		{"mtime", func(e *Entry) { e.MTime = 123 }},
		{"link", func(e *Entry) { e.Link = "target2" }},
		{"xattrs", func(e *Entry) { e.XAttrs = map[string]string{"user.a": "b"} }},
	} {
		t.Run(tc.field, func(t *testing.T) {
			a := Entry{Path: "etc/hosts", Type: "file", Mode: 0600}
			b := a
			tc.change(&b)
			v := compareFiles([]byte("same\n"), []byte("same\n"), &a, &b, "A", "B")
			if v.State != "different" || v.ContentChanged || len(v.Hunks) != 0 || len(v.MetadataChanges) != 1 || v.MetadataChanges[0].Field != tc.field {
				t.Fatalf("%+v", v)
			}
		})
	}
}
func TestComparisonAddedEmptyRemovedAndFinalNewline(t *testing.T) {
	e := Entry{Path: "etc/hosts", Type: "file"}
	for _, tc := range []struct {
		before, after string
		a, b          *Entry
		state         string
		changed       bool
	}{
		{"", "", nil, &e, "missing_before", false}, {"", "", &e, nil, "missing_after", false}, {"", "new", nil, &e, "missing_before", true}, {"old", "", &e, nil, "missing_after", true}, {"a", "a\n", &e, &e, "different", true}, {"a", "a", &e, &e, "identical", false},
	} {
		v := compareFiles([]byte(tc.before), []byte(tc.after), tc.a, tc.b, "A", "B")
		if v.State != tc.state || v.ContentChanged != tc.changed {
			t.Fatalf("%+v", v)
		}
	}
}
func TestComparisonBudgetAndDistantHunks(t *testing.T) {
	e := Entry{Path: "etc/hosts", Type: "file"}
	v := compareFiles([]byte(strings.Repeat("a\n", 1500)), []byte(strings.Repeat("b\n", 1500)), &e, &e, "A", "B")
	if v.State != "too_large" || !strings.Contains(v.Error, "Download") {
		t.Fatal(v)
	}
	a := "old\n" + strings.Repeat("same\n", 20) + "old\n"
	b := "new\n" + strings.Repeat("same\n", 20) + "new\n"
	v = compareFiles([]byte(a), []byte(b), &e, &e, "A", "B")
	if len(v.Hunks) != 2 || v.Hunks[1].BeforeStart != 19 {
		t.Fatal(v)
	}
}
func TestComparisonMergesOverlappingContext(t *testing.T) {
	e := Entry{Path: "etc/hosts", Type: "file"}
	a := "old\n" + strings.Repeat("same\n", 5) + "old\n"
	b := "new\n" + strings.Repeat("same\n", 5) + "new\n"
	v := compareFiles([]byte(a), []byte(b), &e, &e, "A", "B")
	if len(v.Hunks) != 1 || len(v.Hunks[0].Lines) != 9 {
		t.Fatal(v)
	}
}
func TestComparisonSymlinkTargetIsMetadata(t *testing.T) {
	a := Entry{Path: "etc/hosts", Type: "symlink", Link: "old"}
	b := a
	b.Link = "new"
	v := compareFiles([]byte("old"), []byte("new"), &a, &b, "A", "B")
	if v.ContentChanged || len(v.Hunks) != 0 || len(v.MetadataChanges) != 1 || v.MetadataChanges[0].Field != "link" {
		t.Fatal(v)
	}
}
