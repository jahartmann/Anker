package main

import (
	"anker/internal/updater"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"time"
)

// Reuse preserves the selected address, custom port, TLS identity and policy.
// An invalid configuration must go through the connection wizard again.
func setupExistingWeb(prior string, policy updater.TLSPolicy) (setupWebConfig, error) {
	c := setupWebConfig{Listen: setupEnvValue(prior, "ANKER_LISTEN"), Host: setupEnvValue(prior, "ANKER_PUBLIC_HOST"), Cert: setupEnvValue(prior, "ANKER_TLS_CERT"), Key: setupEnvValue(prior, "ANKER_TLS_KEY")}
	if _, err := setupWebEnv(c.Listen, c.Cert, c.Key); err != nil {
		return c, err
	}
	if c.Cert == "" {
		return c, nil
	}
	var err error
	c.CertBytes, c.KeyBytes, _, err = setupReadTLS(c.Cert, c.Key)
	if err != nil {
		return c, err
	}
	leaf, err := setupTLSLeaf(c.CertBytes, c.KeyBytes)
	if err != nil {
		return c, err
	}
	if c.Host == "" {
		if len(leaf.IPAddresses) > 0 {
			c.Host = leaf.IPAddresses[0].String()
		} else if len(leaf.DNSNames) > 0 {
			c.Host = leaf.DNSNames[0]
		}
	}
	if err = setupTLSName(c.Host); err != nil {
		return c, err
	}
	if err = leaf.VerifyHostname(c.Host); err != nil {
		return c, err
	}
	if time.Now().Before(leaf.NotBefore) || !time.Now().Before(leaf.NotAfter) {
		return c, errors.New("TLS-Zertifikat abgelaufen oder noch nicht gültig")
	}
	c.SelfSigned = bytes.Equal(leaf.RawIssuer, leaf.RawSubject) && leaf.CheckSignature(leaf.SignatureAlgorithm, leaf.RawTBSCertificate, leaf.Signature) == nil
	hash := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	c.Managed = c.SelfSigned && policy.ManagedCert == c.Cert && policy.ManagedPublicKey == fmt.Sprintf("%X", hash)
	return c, nil
}

func setupTunnelCommand(c setupWebConfig) string {
	host, port, _ := net.SplitHostPort(c.Listen)
	return "ssh -N -L " + port + ":" + net.JoinHostPort(host, port) + " BENUTZER@ANKER-SERVER"
}
