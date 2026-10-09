package main

import (
	"anker/internal/anker"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"
)

func disasterPassphrase(path string) ([]byte, error) {
	if path == "" {
		if !term.IsTerminal(os.Stdin.Fd()) {
			return nil, errors.New("Passphrase-Datei (--passphrase-file, Rechte 0600) oder Terminal erforderlich")
		}
		fmt.Fprint(os.Stderr, "Disaster-Passphrase: ")
		p, err := term.ReadPassword(os.Stdin.Fd())
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return nil, err
		}
		if len(p) == 0 || len(p) > 4096 {
			return nil, errors.New("Ungültige Passphrasenlänge")
		}
		return p, nil
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || st.Size() > 4097 {
		return nil, errors.New("Passphrase-Datei muss regulär, höchstens 4097 Bytes und nur für Eigentümer lesbar sein")
	}
	p, err := io.ReadAll(io.LimitReader(f, 4098))
	if err != nil {
		return nil, err
	}
	p = bytes.TrimSuffix(p, []byte("\n"))
	if len(p) == 0 || len(p) > 4096 {
		clear(p)
		return nil, errors.New("Ungültige Passphrasenlänge")
	}
	return p, nil
}
func runDisaster(root string, args []string) error {
	if len(args) == 0 {
		return errors.New("disaster export|inspect|import: --passphrase-file DATEI; export --output PAKET [--config-root VERZEICHNIS] [--backup-data include|separate], inspect/import --input PAKET, import --destination NEUES-VERZEICHNIS")
	}
	action := args[0]
	if action != "export" && action != "inspect" && action != "import" {
		return errors.New("Unbekannter Disaster-Befehl")
	}
	f := flag.NewFlagSet("disaster "+action, flag.ContinueOnError)
	secret := f.String("passphrase-file", "", "Private Passphrase-Datei; sonst verdeckte Terminaleingabe")
	input := f.String("input", "", "Verschlüsseltes Disaster-Paket")
	output := f.String("output", "", "Neues verschlüsseltes Disaster-Paket")
	dest := f.String("destination", "", "Neues isoliertes Datenverzeichnis")
	backupData := f.String("backup-data", "include", "Sicherungsbytes: include oder separate (vorhandene Quellen prüfen, separat bereitstellen)")
	config := f.String("config-root", "", "Explizites Anker-Konfigurationsverzeichnis, beispielsweise /etc/anker")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return errors.New("Unerwartete Disaster-Argumente")
	}
	if (action == "export" && (*output == "" || *input != "" || *dest != "")) || (action != "export" && (*input == "" || *output != "" || *config != "")) || (action == "import" && *dest == "") || (action == "inspect" && *dest != "") {
		return errors.New("Unvollständige oder widersprüchliche Disaster-Optionen")
	}
	if (*backupData != "include" && *backupData != "separate") || (action != "export" && *backupData != "include") {
		return errors.New("Sicherungsdatenmodus nur für Export: include oder separate")
	}
	pass, err := disasterPassphrase(*secret)
	if err != nil {
		return err
	}
	defer clear(pass)
	if action == "export" {
		parent, err := os.OpenRoot(filepath.Dir(*output))
		if err != nil {
			return err
		}
		defer parent.Close()
		before, err := parent.Stat(".")
		if err != nil {
			return err
		}
		name := ".anker-disaster-encrypted-" + anker.ID()
		out, err := parent.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|unix.O_NOFOLLOW, 0600)
		if err != nil {
			return err
		}
		defer parent.Remove(name)
		defer out.Close()
		if err = anker.ExportDisasterWithBackupData(root, *config, *backupData, pass, out); err != nil {
			return err
		}
		if err = out.Sync(); err != nil {
			return err
		}
		if err = out.Close(); err != nil {
			return err
		}
		after, err := os.Stat(filepath.Dir(*output))
		if err != nil || !os.SameFile(before, after) {
			return errors.New("Export-Elternverzeichnis verändert")
		}
		// Descriptor-confined hard-link publish never replaces an existing entry.
		if err = parent.Link(name, filepath.Base(*output)); err != nil {
			return err
		}
		dir, err := parent.Open(".")
		if err != nil {
			return err
		}
		err = dir.Sync()
		dir.Close()
		if err != nil {
			return err
		}
		fmt.Println("Verschlüsseltes Disaster-Paket erstellt. Separat geschützt verwahren und Import offline prüfen.")
		return nil
	}
	fd, err := unix.Open(*input, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	in := os.NewFile(uintptr(fd), *input)
	defer in.Close()
	st, err := in.Stat()
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return errors.New("Disaster-Paket muss reguläre Datei sein")
	}
	var info anker.DisasterInfo
	if action == "inspect" {
		info, err = anker.InspectDisaster(in, pass)
	} else {
		info, err = anker.ImportDisaster(in, *dest, pass)
	}
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(info)
}
