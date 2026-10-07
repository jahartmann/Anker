package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// Apply directory permissions explicitly: setup can inherit the installer's
// private umask, and an existing directory can retain a previous restrictive mode.
func setupTLSDirectory(path string, gid int) error {
	if err := os.MkdirAll(path, 0750); err != nil {
		return err
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	dir := os.NewFile(uintptr(fd), path)
	defer dir.Close()
	if err = dir.Chown(os.Geteuid(), gid); err != nil {
		return err
	}
	return dir.Chmod(0750)
}

func setupTLSName(host string) error {
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsUnspecified() || ip.IsMulticast() {
			return errors.New("Eine erreichbare Server-IP verwenden, keine Listen-Adresse")
		}
		return nil
	}
	label := regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)
	if len(host) == 0 || len(host) > 253 {
		return errors.New("DNS-Name oder IP ohne Protokoll und Port angeben")
	}
	for _, part := range strings.Split(host, ".") {
		if !label.MatchString(part) {
			return errors.New("DNS-Name oder IP ohne Protokoll und Port angeben")
		}
	}
	return nil
}

func setupReadTLS(cert, key string) ([]byte, []byte, *x509.Certificate, error) {
	read := func(path string) ([]byte, error) {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() > 1<<20 {
			return nil, errors.New("TLS-Datei ungültig oder zu groß")
		}
		return os.ReadFile(path)
	}
	certBytes, err := read(cert)
	if err != nil {
		return nil, nil, nil, err
	}
	keyBytes, err := read(key)
	if err != nil {
		return nil, nil, nil, err
	}
	leaf, err := setupTLSLeaf(certBytes, keyBytes)
	return certBytes, keyBytes, leaf, err
}

func setupTLSLeaf(certBytes, keyBytes []byte) (*x509.Certificate, error) {
	pair, err := tls.X509KeyPair(certBytes, keyBytes)
	if err != nil {
		return nil, fmt.Errorf("TLS-Dateien passen nicht: %w", err)
	}
	return x509.ParseCertificate(pair.Certificate[0])
}

// Self-signed certificates encrypt LAN access. Trust still requires the operator
// to check the fingerprint or install a certificate from their existing CA.
func setupLocalTLS(host, previousCert, previousKey string) ([]byte, []byte, bool, error) {
	if err := setupTLSName(host); err != nil {
		return nil, nil, false, err
	}
	if previousCert != "" && previousKey != "" {
		cert, key, leaf, err := setupReadTLS(previousCert, previousKey)
		if err == nil && leaf.VerifyHostname(host) == nil && !time.Now().Before(leaf.NotBefore) && time.Until(leaf.NotAfter) > 30*24*time.Hour {
			return cert, key, true, nil
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, false, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, false, err
	}
	if serial.Sign() == 0 {
		serial.SetInt64(1)
	}
	now := time.Now()
	leaf := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: host}, NotBefore: now.Add(-5 * time.Minute), NotAfter: now.Add(365 * 24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
	if ip := net.ParseIP(host); ip != nil {
		leaf.IPAddresses = []net.IP{ip}
	} else {
		leaf.DNSNames = []string{host}
	}
	der, err := x509.CreateCertificate(rand.Reader, leaf, leaf, &key.PublicKey, key)
	if err != nil {
		return nil, nil, false, err
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, false, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}), false, nil
}
