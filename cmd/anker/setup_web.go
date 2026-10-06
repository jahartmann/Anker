package main

import (
	"anker/internal/updater"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type setupWebConfig struct {
	Listen, Host, Cert, Key string
	CertBytes, KeyBytes     []byte
	SelfSigned              bool
	Managed                 bool
}

func setupConfigureWeb(prior string, initialized bool) (setupWebConfig, error) {
	display := newSetupDisplay()
	currentCert, currentKey := setupEnvValue(prior, "ANKER_TLS_CERT"), setupEnvValue(prior, "ANKER_TLS_KEY")
	defaultMode := "2"
	if initialized && currentCert == "" {
		defaultMode = "1"
	}
	display.Fact("1 · SSH-Tunnel", "Nur lokal erreichbar")
	display.Fact("2 · LAN/VPN", "HTTPS für das interne Netz")
	mode, err := setupPrompt("Webzugriff", defaultMode)
	if err != nil {
		return setupWebConfig{}, err
	}
	if mode == "1" {
		return setupWebConfig{Listen: "127.0.0.1:8087"}, nil
	}
	if mode != "2" {
		return setupWebConfig{}, errors.New("Webzugriff 1 oder 2 wählen")
	}
	c := setupWebConfig{Listen: "0.0.0.0:8087"}
	if currentCert != "" {
		c.Listen = setupEnvValue(prior, "ANKER_LISTEN")
		if c.Listen == "" {
			c.Listen = "0.0.0.0:8087"
		}
	}
	if _, err := setupListen(c.Listen); err != nil {
		return c, err
	}
	defaultHost := setupEnvValue(prior, "ANKER_PUBLIC_HOST")
	if defaultHost == "" && currentCert != "" {
		_, _, leaf, err := setupReadTLS(currentCert, currentKey)
		if err == nil && len(leaf.DNSNames) > 0 && !strings.HasPrefix(leaf.DNSNames[0], "*.") {
			defaultHost = leaf.DNSNames[0]
		} else if err == nil && len(leaf.IPAddresses) > 0 {
			defaultHost = leaf.IPAddresses[0].String()
		}
	}
	if defaultHost == "" {
		defaultHost, _ = os.Hostname()
	}
	fmt.Println()
	display.Note("Interner DNS-Name oder IP-Adresse, ohne https://.")
	c.Host, err = setupPrompt("Adresse im Browser", defaultHost)
	if err != nil {
		return c, err
	}
	if err = setupTLSName(c.Host); err != nil {
		return c, err
	}
	defaultTLS := "1"
	fmt.Println()
	display.Fact("1 · Erzeugen", "Eigenes Zertifikat durch Anker verwalten")
	display.Fact("2 · Importieren", "Vorhandene Zertifikatsdateien verwenden")
	canKeep := false
	if currentCert != "" && currentKey != "" {
		_, _, leaf, readErr := setupReadTLS(currentCert, currentKey)
		canKeep = readErr == nil && leaf.VerifyHostname(c.Host) == nil && !time.Now().Before(leaf.NotBefore) && time.Now().Before(leaf.NotAfter)
		if canKeep {
			defaultTLS = "3"
			display.Fact("3 · Behalten", "Aktuelles Zertifikat weiterverwenden")
		} else {
			display.Note("Vorhandenes Zertifikat für diese Adresse nicht nutzbar. Erzeugen oder importieren.")
		}
	}
	choice, err := setupPrompt("TLS", defaultTLS)
	if err != nil {
		return c, err
	}
	preserve := false
	if choice == "1" {
		c.CertBytes, c.KeyBytes, preserve, err = setupLocalTLS(c.Host, currentCert, currentKey)
		if err != nil {
			return c, err
		}
	} else if choice == "2" || (choice == "3" && canKeep) {
		preserve = choice == "3"
		if choice == "2" {
			currentCert, err = setupPrompt("Zertifikat mit Zertifikatskette (PEM)", currentCert)
			if err != nil {
				return c, err
			}
			currentKey, err = setupPrompt("Zugehöriger privater TLS-Schlüssel (PEM)", currentKey)
			if err != nil {
				return c, err
			}
		}
		if _, err = setupWebEnv(c.Listen, currentCert, currentKey); err != nil {
			return c, err
		}
		c.CertBytes, c.KeyBytes, _, err = setupReadTLS(currentCert, currentKey)
		if err != nil {
			return c, err
		}
	} else {
		return c, errors.New("Eine angebotene TLS-Option wählen")
	}
	leaf, err := setupTLSLeaf(c.CertBytes, c.KeyBytes)
	if err != nil {
		return c, err
	}
	if err = leaf.VerifyHostname(c.Host); err != nil {
		return c, fmt.Errorf("Zertifikat passt nicht zur Webadresse: %w", err)
	}
	if time.Now().Before(leaf.NotBefore) || !time.Now().Before(leaf.NotAfter) {
		return c, errors.New("TLS-Zertifikat ist noch nicht gültig oder abgelaufen")
	}
	c.SelfSigned = bytes.Equal(leaf.RawIssuer, leaf.RawSubject) && leaf.CheckSignature(leaf.SignatureAlgorithm, leaf.RawTBSCertificate, leaf.Signature) == nil
	c.Managed = choice == "1" && c.SelfSigned
	if choice == "3" {
		policy, err := updater.LoadTLSPolicy("/etc/anker")
		if err != nil {
			return c, err
		}
		hash := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
		c.Managed = c.SelfSigned && policy.ManagedCert == currentCert && policy.ManagedPublicKey == fmt.Sprintf("%X", hash)
	}
	if c.SelfSigned {
		display.Note("Eigenes Zertifikat · keine CA oder öffentliche Domain erforderlich.")
		display.Note("Den Fingerprint vor dem ersten Browserzugriff am Server abgleichen.")
	}
	if preserve && strings.HasPrefix(currentCert, "/etc/anker/tls/") && strings.HasPrefix(currentKey, "/etc/anker/tls/") {
		certInfo, certErr := os.Lstat(currentCert)
		keyInfo, keyErr := os.Lstat(currentKey)
		if certErr == nil && keyErr == nil && certInfo.Mode().IsRegular() && keyInfo.Mode().IsRegular() {
			c.Cert, c.Key = currentCert, currentKey
			return c, nil
		}
	}
	generation := strconv.FormatInt(time.Now().UnixNano(), 10)
	c.Cert, c.Key = "/etc/anker/tls/server-"+generation+".crt", "/etc/anker/tls/server-"+generation+".key"
	return c, nil
}

func (c setupWebConfig) env() (string, error) {
	text, err := setupWebEnv(c.Listen, c.Cert, c.Key)
	if err != nil {
		return "", err
	}
	if c.Cert != "" {
		text += "ANKER_PUBLIC_HOST=" + strconv.Quote(c.Host) + "\n"
	}
	return text, nil
}

func (c setupWebConfig) URL() string {
	if c.Cert == "" {
		return "http://" + c.Listen
	}
	_, port, _ := net.SplitHostPort(c.Listen)
	return "https://" + net.JoinHostPort(c.Host, port)
}

// Keep local service customizations when changing only the web connection.
func setupMergeWebEnv(prior, web string) string {
	var kept []string
	for _, line := range strings.Split(prior, "\n") {
		replace := false
		for _, name := range []string{"ANKER_LISTEN", "ANKER_TLS_CERT", "ANKER_TLS_KEY", "ANKER_PUBLIC_HOST"} {
			if strings.HasPrefix(strings.TrimSpace(line), name+"=") {
				replace = true
				break
			}
		}
		if !replace && line != "" {
			kept = append(kept, line)
		}
	}
	if len(kept) == 0 {
		return web
	}
	return strings.Join(kept, "\n") + "\n" + web
}
