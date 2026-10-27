package hostconnect

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type CheckResult struct {
	Authenticated bool `json:"authenticated"`
}

// Check authenticates against an explicitly confirmed identity. It does not run
// commands, install helpers, create keys or modify the trust store.
func (m Manager) Check(ctx context.Context, request EnrollRequest) (CheckResult, error) {
	r, err := validate(InspectRequest{Address: request.Address, Port: request.Port})
	if err != nil {
		return CheckResult{}, err
	}
	decoded, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(request.Fingerprint, "SHA256:"))
	if !request.Confirmed || err != nil || len(decoded) != 32 || !strings.HasPrefix(request.Fingerprint, "SHA256:") {
		return CheckResult{}, errors.New("Gültigen SSH-Fingerprint ausdrücklich bestätigen")
	}
	if !usernamePattern.MatchString(request.Username) || len(request.Password) == 0 || len(request.Password) > 4096 || strings.ContainsAny(request.Password, "\x00\r\n") {
		return CheckResult{}, errors.New("Ungültige SSH-Zugangsdaten")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	dir, err := m.openDir()
	if err != nil {
		return CheckResult{}, phaseError("SSH-Konfiguration", ctx)
	}
	defer dir.Close()
	trust, err := m.readTrust(dir)
	if err != nil {
		return CheckResult{}, phaseError("SSH-Vertrauen", ctx)
	}
	verified := false
	client, err := connect(ctx, r, clientConfig(request.Username, passwordMethods(request.Password), func(_ string, _ net.Addr, key ssh.PublicKey) error {
		if ssh.FingerprintSHA256(key) != request.Fingerprint {
			return errors.New("SSH-Hostschlüssel stimmt nicht mit Bestätigung überein")
		}
		_, changed, err := identityState(trust, endpoint(r), key)
		if err != nil {
			return err
		}
		if changed {
			return errors.New("Bekannter SSH-Hostschlüssel wurde geändert")
		}
		verified = true
		return nil
	}))
	if err != nil {
		if verified && strings.Contains(err.Error(), "unable to authenticate") {
			return CheckResult{Authenticated: false}, nil
		}
		return CheckResult{}, phaseError("Zugangsprüfung", ctx)
	}
	client.Close()
	return CheckResult{Authenticated: true}, nil
}
