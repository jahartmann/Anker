package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSetupProgressReportsFailureAndStopsBeforeReturn(t *testing.T) {
	var output bytes.Buffer
	display := setupDisplay{out: &output, animated: true, width: 60}
	want := errors.New("service unavailable")
	calls := 0
	err := display.Wait("Webdienst", func() error { calls++; return want })
	if calls != 1 || !errors.Is(err, want) {
		t.Fatal("progress changed the operation or its error", calls, err)
	}
	before := output.String()
	if !strings.Contains(before, "Webdienst") || !strings.Contains(before, "fehlgeschlagen") {
		t.Fatal("failed operation has no visible failure state", before)
	}
	time.Sleep(150 * time.Millisecond)
	if output.String() != before {
		t.Fatal("animation continued after returning to the caller")
	}
}

func TestSetupProgressWithoutAnimationIsReadableInLogs(t *testing.T) {
	var output bytes.Buffer
	display := setupDisplay{out: &output, width: 60}
	calls := 0
	err := display.Wait("Webdienst", func() error { calls++; return nil })
	if err != nil || calls != 1 {
		t.Fatal("operation did not finish", err, calls)
	}
	text := output.String()
	if strings.ContainsAny(text, "\x1b\r") || !strings.Contains(text, "Webdienst") || !strings.Contains(text, "bereit") {
		t.Fatal("nonanimated progress is not a readable completed operation", text)
	}
}
