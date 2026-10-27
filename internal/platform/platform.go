// Package platform interprets version metadata from current and legacy host helpers.
package platform

import "strings"

func Detect(kind, pve, pbs string) string {
	pve, pbs = strings.TrimSpace(pve), strings.TrimSpace(pbs)
	if (kind == "" || kind == "pve") && pve != "" && pbs == "" {
		return "pve"
	}
	if kind == "pbs" && pbs != "" && pve == "" {
		return "pbs"
	}
	return ""
}
