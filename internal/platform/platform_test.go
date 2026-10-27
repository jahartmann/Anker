package platform

import "testing"

func TestDetectCurrentAndLegacyHelpers(t *testing.T) {
	for _, tc := range []struct{ kind, pve, pbs, want string }{
		{"", "8.4", "", "pve"}, {"pve", "9.0", "", "pve"}, {"pbs", "", "4.0.1-1", "pbs"},
		{"", "", "4.0", ""}, {"pbs", "9.0", "4.0", ""}, {"pve", "", "4.0", ""}, {"unknown", "9.0", "", ""}, {"pbs", "", " ", ""},
	} {
		if got := Detect(tc.kind, tc.pve, tc.pbs); got != tc.want {
			t.Errorf("Detect(%q,%q,%q)=%q, want %q", tc.kind, tc.pve, tc.pbs, got, tc.want)
		}
	}
}
