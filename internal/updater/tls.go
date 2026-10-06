package updater

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"math"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const TLSConfigPath = "/etc/anker/tls-renewal.json"

var ErrTLSBusy = errors.New("Einrichtung oder Zertifikatsänderung läuft; erneut versuchen")

type TLSPolicy struct {
	Automatic        bool   `json:"automatic"`
	RenewBeforeDays  int    `json:"renew_before_days"`
	LastRenewedAt    string `json:"last_renewed_at,omitempty"`
	ManagedCert      string `json:"managed_cert,omitempty"`
	ManagedPublicKey string `json:"managed_public_key,omitempty"`
}

type TLSStatus struct {
	Enabled         bool     `json:"enabled"`
	Managed         bool     `json:"managed"`
	Automatic       bool     `json:"automatic"`
	RenewBeforeDays int      `json:"renew_before_days"`
	ExpiresAt       string   `json:"expires_at,omitempty"`
	ValidFrom       string   `json:"valid_from,omitempty"`
	DaysRemaining   int      `json:"days_remaining"`
	Fingerprint     string   `json:"fingerprint,omitempty"`
	Names           []string `json:"names,omitempty"`
	LastRenewedAt   string   `json:"last_renewed_at,omitempty"`
	Message         string   `json:"message,omitempty"`
	LastError       string   `json:"last_error,omitempty"`
}

// Requests cannot choose certificate paths or identities. Only the root-owned
// service configuration selects them. The existing setup lock serializes edits.
type TLSManager struct {
	Dir      string
	ownerUID uint32
	now      func() time.Time
}

func (m *TLSManager) clock() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}
func (m *TLSManager) read(path string, max int64) ([]byte, os.FileInfo, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || info.Size() > max || !ok || owner.Uid != m.ownerUID || info.Mode().Perm()&0022 != 0 {
		return nil, nil, errors.New("TLS-Dateien müssen root gehören, regulär und für andere nicht schreibbar sein")
	}
	data, err := io.ReadAll(io.LimitReader(file, max+1))
	if len(data) > int(max) {
		return nil, nil, errors.New("TLS-Datei zu groß")
	}
	return data, info, err
}
func (m *TLSManager) lock() (func(), error) {
	if err := setupConfigurationIdle(m.Dir); err != nil {
		return nil, err
	}
	fd, err := unix.Open(filepath.Join(m.Dir, "setup.lock"), unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	var info unix.Stat_t
	if err = unix.Fstat(fd, &info); err != nil || info.Mode&unix.S_IFMT != unix.S_IFREG || info.Uid != m.ownerUID || info.Mode&0022 != 0 {
		unix.Close(fd)
		return nil, errors.New("Ungültige TLS-/Einrichtungssperre")
	}
	if err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		unix.Close(fd)
		return nil, ErrTLSBusy
	}
	return func() { unix.Close(fd) }, nil
}
func (m *TLSManager) policy() (TLSPolicy, error) {
	p := TLSPolicy{RenewBeforeDays: 30}
	data, _, err := m.read(filepath.Join(m.Dir, "tls-renewal.json"), 4096)
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err = d.Decode(&p); err != nil {
		return p, err
	}
	if err = d.Decode(new(any)); err != io.EOF {
		return p, errors.New("TLS-Einstellungen enthalten zusätzliche Daten")
	}
	if p.RenewBeforeDays < 7 || p.RenewBeforeDays > 90 {
		return p, errors.New("Erneuerungsvorlauf muss zwischen 7 und 90 Tagen liegen")
	}
	return p, nil
}
func LoadTLSPolicy(dir string) (TLSPolicy, error) { return (&TLSManager{Dir: dir}).policy() }
func envValue(data []byte, name string) string {
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, name+"=") {
			value := strings.TrimPrefix(line, name+"=")
			if decoded, err := strconv.Unquote(value); err == nil {
				return decoded
			}
			return value
		}
	}
	return ""
}
func (m *TLSManager) current() (string, string, []byte, os.FileInfo, *tls.Certificate, *x509.Certificate, error) {
	data, _, err := m.read(filepath.Join(m.Dir, "service.env"), 64<<10)
	if err != nil {
		return "", "", nil, nil, nil, nil, err
	}
	cert, key := envValue(data, "ANKER_TLS_CERT"), envValue(data, "ANKER_TLS_KEY")
	if cert == "" && key == "" {
		return "", "", nil, nil, nil, nil, nil
	}
	if cert == "" || key == "" {
		return "", "", nil, nil, nil, nil, errors.New("TLS-Zertifikat und Schlüssel fehlen teilweise")
	}
	certData, info, err := m.read(cert, 1<<20)
	if err != nil {
		return "", "", nil, nil, nil, nil, err
	}
	keyData, _, err := m.read(key, 1<<20)
	if err != nil {
		return "", "", nil, nil, nil, nil, err
	}
	pair, err := tls.X509KeyPair(certData, keyData)
	if err != nil {
		return "", "", nil, nil, nil, nil, errors.New("TLS-Zertifikat und Schlüssel passen nicht")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return "", "", nil, nil, nil, nil, err
	}
	host := envValue(data, "ANKER_PUBLIC_HOST")
	if host != "" {
		if err = leaf.VerifyHostname(host); err != nil {
			return "", "", nil, nil, nil, nil, errors.New("TLS-Zertifikat passt nicht zur eingerichteten Webadresse")
		}
	}
	return cert, key, certData, info, &pair, leaf, nil
}
func (m *TLSManager) managed(cert, key string, leaf *x509.Certificate) bool {
	p, err := m.policy()
	if err != nil {
		return false
	}
	publicHash := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	if p.ManagedCert != cert || p.ManagedPublicKey != fmt.Sprintf("%X", publicHash) {
		return false
	}
	dir := filepath.Join(m.Dir, "tls")
	name := filepath.Base(cert)
	return filepath.Dir(cert) == dir && filepath.Dir(key) == dir && regexp.MustCompile(`^server-[0-9]+\.crt$`).MatchString(name) && filepath.Base(key) == strings.TrimSuffix(name, ".crt")+".key" && !leaf.IsCA && bytes.Equal(leaf.RawIssuer, leaf.RawSubject) && leaf.CheckSignature(leaf.SignatureAlgorithm, leaf.RawTBSCertificate, leaf.Signature) == nil
}
func (m *TLSManager) Status() (TLSStatus, error) {
	p, err := m.policy()
	if err != nil {
		return TLSStatus{}, err
	}
	s := TLSStatus{RenewBeforeDays: p.RenewBeforeDays, LastRenewedAt: p.LastRenewedAt}
	if data, _, readErr := m.read(filepath.Join(m.Dir, "tls-renewal-error"), 4096); readErr == nil {
		s.LastError = string(data)
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return s, readErr
	}
	cert, key, _, _, _, leaf, err := m.current()
	if err != nil {
		return s, err
	}
	if leaf == nil {
		s.Message = "Webzugriff über SSH-Tunnel; kein TLS-Zertifikat eingerichtet."
		return s, nil
	}
	s.Enabled = true
	s.Managed = m.managed(cert, key, leaf)
	s.Automatic = p.Automatic && s.Managed
	s.ExpiresAt = leaf.NotAfter.UTC().Format(time.RFC3339)
	s.ValidFrom = leaf.NotBefore.UTC().Format(time.RFC3339)
	s.DaysRemaining = int(math.Ceil(leaf.NotAfter.Sub(m.clock()).Hours() / 24))
	fingerprint := sha256.Sum256(leaf.Raw)
	s.Fingerprint = fmt.Sprintf("%X", fingerprint)
	s.Names = append([]string{}, leaf.DNSNames...)
	for _, ip := range leaf.IPAddresses {
		s.Names = append(s.Names, ip.String())
	}
	if s.Managed {
		s.Message = "Selbstsigniertes Zertifikat. Nach Erneuerung kann eine neue Browserfreigabe nötig sein."
	} else {
		s.Message = "Importiertes Zertifikat. Erneuerung erfolgt über dessen Aussteller; Anker überschreibt es nicht."
	}
	return s, nil
}
func (m *TLSManager) SavePolicy(p TLSPolicy) (TLSStatus, error) {
	if p.RenewBeforeDays < 7 || p.RenewBeforeDays > 90 {
		return TLSStatus{}, errors.New("Erneuerungsvorlauf muss zwischen 7 und 90 Tagen liegen")
	}
	unlock, err := m.lock()
	if err != nil {
		return TLSStatus{}, err
	}
	defer unlock()
	current, err := m.Status()
	if err != nil {
		return TLSStatus{}, err
	}
	if !current.Managed {
		return TLSStatus{}, errors.New("Automatische Erneuerung ist nur für von Anker verwaltete selbstsignierte Zertifikate verfügbar")
	}
	p.LastRenewedAt = current.LastRenewedAt
	previous, err := m.policy()
	if err != nil {
		return TLSStatus{}, err
	}
	p.ManagedCert, p.ManagedPublicKey = previous.ManagedCert, previous.ManagedPublicKey
	data, err := json.Marshal(p)
	if err != nil {
		return TLSStatus{}, err
	}
	if err = atomic(filepath.Join(m.Dir, "tls-renewal.json"), append(data, '\n'), 0600, int(m.ownerUID), os.Getegid()); err != nil {
		return TLSStatus{}, err
	}
	return m.Status()
}
func (m *TLSManager) renew(force bool) (TLSStatus, error) {
	unlock, err := m.lock()
	if err != nil {
		return TLSStatus{}, err
	}
	defer unlock()
	policy, err := m.policy()
	if err != nil {
		return TLSStatus{}, err
	}
	cert, key, previous, info, pair, leaf, err := m.current()
	if err != nil {
		return TLSStatus{}, err
	}
	if leaf == nil || !m.managed(cert, key, leaf) {
		if !force {
			return m.Status()
		}
		return TLSStatus{}, errors.New("Nur Ankers selbstsigniertes Zertifikat kann erneuert werden; importierte Zertifikate bleiben unverändert")
	}
	if !force && (!policy.Automatic || leaf.NotAfter.Sub(m.clock()) > time.Duration(policy.RenewBeforeDays)*24*time.Hour) {
		return m.Status()
	}
	signer, ok := pair.PrivateKey.(crypto.Signer)
	if !ok {
		return TLSStatus{}, errors.New("TLS-Schlüssel unterstützt keine Erneuerung")
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return TLSStatus{}, err
	}
	if serial.Sign() == 0 {
		serial.SetInt64(1)
	}
	now := m.clock()
	template := &x509.Certificate{SerialNumber: serial, Subject: leaf.Subject, NotBefore: now.Add(-5 * time.Minute), NotAfter: now.Add(365 * 24 * time.Hour), DNSNames: leaf.DNSNames, IPAddresses: append([]net.IP{}, leaf.IPAddresses...), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, signer.Public(), signer)
	if err != nil {
		return TLSStatus{}, err
	}
	renewed := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	// Validate before touching the active file. Its matching private key is retained,
	// so a single atomic certificate replacement cannot expose a mismatched pair.
	encodedKey, _, err := m.read(key, 1<<20)
	if err != nil {
		return TLSStatus{}, err
	}
	if _, err = tls.X509KeyPair(renewed, encodedKey); err != nil {
		return TLSStatus{}, err
	}
	owner := info.Sys().(*syscall.Stat_t)
	if err = atomic(cert+".previous", previous, info.Mode().Perm(), int(owner.Uid), int(owner.Gid)); err != nil {
		return TLSStatus{}, err
	}
	if err = atomic(cert, renewed, info.Mode().Perm(), int(owner.Uid), int(owner.Gid)); err != nil {
		return TLSStatus{}, err
	}
	policy.LastRenewedAt = now.UTC().Format(time.RFC3339)
	data, _ := json.Marshal(policy)
	if err = atomic(filepath.Join(m.Dir, "tls-renewal.json"), append(data, '\n'), 0600, int(m.ownerUID), os.Getegid()); err != nil {
		return TLSStatus{}, errors.New("Zertifikat erneuert, Zeitstempel konnte nicht gespeichert werden; TLS-Einstellungen prüfen")
	}
	return m.Status()
}
func (m *TLSManager) clearError() {
	if err := os.Remove(filepath.Join(m.Dir, "tls-renewal-error")); err == nil {
		syncDir(m.Dir)
	}
}
func (m *TLSManager) Renew() (TLSStatus, error) {
	status, err := m.renew(true)
	if err == nil {
		m.clearError()
		status.LastError = ""
	}
	return status, err
}
func (m *TLSManager) Maintain() error {
	_, err := m.renew(false)
	if errors.Is(err, ErrTLSBusy) {
		return err
	}
	if err == nil {
		m.clearError()
	} else {
		message := []byte(err.Error())
		if len(message) > 4096 {
			message = message[:4096]
		}
		atomic(filepath.Join(m.Dir, "tls-renewal-error"), message, 0600, int(m.ownerUID), os.Getegid())
	}
	return err
}
func (m *TLSManager) Certificate() ([]byte, error) {
	_, _, data, _, _, leaf, err := m.current()
	if err != nil {
		return nil, err
	}
	if leaf == nil {
		return nil, errors.New("Kein TLS-Zertifikat eingerichtet")
	}
	return data, nil
}

// A crashed setup releases its flock but its configuration may still be partial.
// Keep system changes blocked until setup has recovered the recorded state.
func setupConfigurationIdle(dir string) error {
	if _, err := os.Lstat(filepath.Join(dir, "setup-pending.json")); err == nil {
		return ErrTLSBusy
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}
