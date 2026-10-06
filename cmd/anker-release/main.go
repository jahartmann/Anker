// Release tooling is kept separate from the server binary.
package main

import (
	"anker/internal/updater"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func writeNew(p string, b []byte, mode os.FileMode) error {
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	cerr := f.Close()
	if err == nil {
		err = cerr
	}
	return err
}
func run() error {
	mode := flag.String("mode", "sign", "keygen or sign")
	dir := flag.String("dir", "dist", "output directory")
	version := flag.String("version", "", "release tag vX.Y.Z")
	flag.Parse()
	if err := os.MkdirAll(*dir, 0700); err != nil {
		return err
	}
	if *mode == "keygen" {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return err
		}
		der, err := x509.MarshalPKIXPublicKey(pub)
		if err != nil {
			return err
		}
		for _, f := range []struct {
			name string
			b    []byte
			mode os.FileMode
		}{{"private.key", []byte(base64.StdEncoding.EncodeToString(priv) + "\n"), 0600}, {"public.key", []byte(base64.StdEncoding.EncodeToString(pub) + "\n"), 0644}, {"public.pem", pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), 0644}} {
			if err := writeNew(filepath.Join(*dir, f.name), f.b, f.mode); err != nil {
				return err
			}
		}
		fmt.Println("Signierschlüssel erstellt. private.key außerhalb des Repositorys aufbewahren.")
		return nil
	}
	if *mode != "sign" {
		return errors.New("unbekannter Modus")
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(os.Getenv("ANKER_SIGNING_KEY")))
	if err != nil || len(key) != ed25519.PrivateKeySize {
		return errors.New("ANKER_SIGNING_KEY muss einen base64-kodierten Ed25519-Schlüssel enthalten")
	}
	// Validate the internal public half rather than signing with malformed private bytes.
	expected := ed25519.NewKeyFromSeed(key[:32])
	if string(expected) != string(key) {
		return errors.New("Signierschlüssel inkonsistent")
	}
	m := updater.Manifest{Version: *version, Format: 1}
	checksums := []string{}
	for _, arch := range []string{"amd64", "arm64"} {
		for _, kind := range []string{"binary", "bundle"} {
			name := "anker-linux-" + arch
			if kind == "bundle" {
				name += ".tar.gz"
			}
			b, err := os.ReadFile(filepath.Join(*dir, name))
			if err != nil {
				return err
			}
			hash := sha256.Sum256(b)
			sha := hex.EncodeToString(hash[:])
			m.Assets = append(m.Assets, updater.Artifact{Kind: kind, Name: name, OS: "linux", Arch: arch, Size: int64(len(b)), SHA256: sha})
			checksums = append(checksums, sha+"  "+name)
		}
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	sig := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(key, raw)) + "\n")
	if _, _, err = updater.VerifyManifest(raw, sig, base64.StdEncoding.EncodeToString(key[32:]), *version, "linux", "amd64"); err != nil {
		return err
	}
	for _, f := range []struct {
		name string
		b    []byte
	}{{"release.json", raw}, {"release.json.sig", sig}, {"SHA256SUMS", []byte(strings.Join(checksums, "\n") + "\n")}} {
		if err = writeNew(filepath.Join(*dir, f.name), f.b, 0644); err != nil {
			return err
		}
	}
	return nil
}
