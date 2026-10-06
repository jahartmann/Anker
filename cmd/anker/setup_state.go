package main

import (
	"anker/internal/updater"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
)

// Inspect access without creating a catalog or modifying a live database.
func setupInitialized(path string) (bool, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	dsn := url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}
	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return false, err
	}
	defer db.Close()
	var users int
	err = db.QueryRow("SELECT count(*) FROM records WHERE bucket='users'").Scan(&users)
	return users > 0, err
}

func setupInitializeAccess(admin, password string) error {
	cmd := exec.Command("runuser", "-u", "anker", "--", updater.BinaryPath, "--data", updater.DataDir, "--admin", admin, "init")
	for _, variable := range os.Environ() {
		if !strings.HasPrefix(variable, "ANKER_INITIAL_PASSWORD=") {
			cmd.Env = append(cmd.Env, variable)
		}
	}
	if password != "" {
		cmd.Env = append(cmd.Env, "ANKER_INITIAL_PASSWORD="+password)
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Administratorinitialisierung fehlgeschlagen: %w", err)
	}
	return nil
}
