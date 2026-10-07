// Package hostassets contains the trusted host helper used by enrollment.
package hostassets

import _ "embed"

//go:embed anker_host.py
var Helper []byte
