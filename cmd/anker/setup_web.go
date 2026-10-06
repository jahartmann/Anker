package main

import (
	"bytes"
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
}

func setupConfigureWeb(prior string, initialized bool) (setupWebConfig, error) {
	currentCert, currentKey := setupEnvValue(prior, "ANKER_TLS_CERT"), setupEnvValue(prior, "ANKER_TLS_KEY")
	defaultMode := "2"
	if initialized && currentCert == "" {
		defaultMode = "1"
	}
	mode, err := setupPrompt("Webzugriff: 1 = SSH-Tunnel, 2 = LAN/VPN mit HTTPS", defaultMode)
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
	c.Host, err = setupPrompt("Adresse im Browser (DNS-Name oder IP, ohne https://)", defaultHost)
	if err != nil {
		return c, err
	}
	if err = setupTLSName(c.Host); err != nil {
		return c, err
	}
	defaultTLS := "1"
	label := "TLS: 1 = automatisch erzeugen, 2 = eigene Zertifikatsdateien"
	if currentCert != "" && currentKey != "" {
		defaultTLS = "3"
		label += ", 3 = aktuelles behalten"
	}
	choice, err := setupPrompt(label, defaultTLS)
	if err != nil {
		return c, err
	}
	preserve := false
	if choice == "1" {
		c.CertBytes, c.KeyBytes, preserve, err = setupLocalTLS(c.Host, currentCert, currentKey)
		if err != nil {
			return c, err
		}
	} else if choice == "2" || (choice == "3" && currentCert != "" && currentKey != "") {
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
	if c.SelfSigned {
		fmt.Println("Eigenes TLS-Zertifikat: Verbindung verschlüsselt; Fingerprint vor dem ersten Browserzugriff prüfen. Für verwaltete Clients eure interne CA verwenden.")
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
		return "http://127.0.0.1:8087"
	}
	_, port, _ := net.SplitHostPort(c.Listen)
	return "https://" + net.JoinHostPort(c.Host, port)
}
