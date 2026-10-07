package tui

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type trustedHost struct {
	address, fingerprint string
	port                 int
}

var dnsLabel = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

func validateTrust(h trustedHost) error {
	if h.port < 1 || h.port > 65535 {
		return errors.New("SSH-Port muss zwischen 1 und 65535 liegen")
	}
	if ip := net.ParseIP(h.address); ip != nil {
		if ip.IsUnspecified() || ip.IsMulticast() {
			return errors.New("Erreichbare Host-IP angeben")
		}
	} else {
		if len(h.address) == 0 || len(h.address) > 253 {
			return errors.New("DNS-Name oder IP ohne Protokoll und Port angeben")
		}
		for _, part := range strings.Split(h.address, ".") {
			if !dnsLabel.MatchString(part) {
				return errors.New("DNS-Name oder IP ohne Protokoll und Port angeben")
			}
		}
	}
	decoded, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(h.fingerprint, "SHA256:"))
	if err != nil || len(decoded) != 32 || !strings.HasPrefix(h.fingerprint, "SHA256:") {
		return errors.New("Unabhängig geprüften SHA256-Fingerprint vollständig angeben")
	}
	return nil
}
func trustArgs(h trustedHost) []string {
	return []string{"host", "trust", h.address, "--port", strconv.Itoa(h.port), "--fingerprint", h.fingerprint}
}

type boundedOutput struct{ data []byte }

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := min(len(p), max(0, (64<<10)-len(b.data)))
	b.data = append(b.data, p[:n]...)
	return len(p), nil
}
func executeTrust(parent context.Context, h trustedHost) (any, error) {
	if err := validateTrust(h); err != nil {
		return nil, err
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(parent, 25*time.Second)
	defer cancel()
	// Invoke only Anker's existing fixed, root-checked trust entry point. No shell,
	// editable command, or additional executable is accepted from form input.
	cmd := exec.CommandContext(ctx, executable, trustArgs(h)...)
	var output boundedOutput
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err = cmd.Run(); err != nil {
		return nil, fmt.Errorf("SSH-Vertrauen: %s (%w)", single(string(output.data)), err)
	}
	return map[string]any{"message": single(string(output.data))}, nil
}
