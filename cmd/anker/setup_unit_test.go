package main

import (
	"strings"
	"testing"
)

func TestSetupMigratesUpdaterUnitWithoutDiscardingLocalSettings(t *testing.T) {
	original := []byte("[Unit]\nDescription=local description\nConditionPathExists=/etc/anker/update.json\nAfter=network-online.target\n[Service]\nExecStart=/usr/local/libexec/anker-updater updater-serve\nRestartSec=12\n")
	updated, changed, err := setupUpdaterUnitText(original)
	if err != nil || !changed {
		t.Fatal(err, changed)
	}
	if strings.Contains(string(updated), "ConditionPathExists=") || !strings.Contains(string(updated), "RestartSec=12") || !strings.Contains(string(updated), "Description=local description") {
		t.Fatal("local settings discarded", string(updated))
	}
	repeated, changed, err := setupUpdaterUnitText(updated)
	if err != nil || changed || string(repeated) != string(updated) {
		t.Fatal("unit migration not repeatable", err)
	}
	if _, _, err := setupUpdaterUnitText([]byte("ConditionPathExists=/etc/anker/update.json\nExecStart=/unrelated/application\n")); err == nil {
		t.Fatal("unrelated service rewritten")
	}
}
