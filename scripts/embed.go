// Package hostscripts contains the trusted host installer and key authorizer.
package hostscripts

import _ "embed"

//go:embed install-host.sh
var Installer []byte

//go:embed host-authorize.py
var Authorizer []byte
