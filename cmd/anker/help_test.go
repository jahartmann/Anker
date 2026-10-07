package main

import (
	"anker/internal/client"
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Run the actual entry point in a child process so exit status and standard
// streams have the same behavior as an invocation from the shell.
func TestHelpProcess(t *testing.T) {
	if os.Getenv("ANKER_HELP_PROCESS") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"anker"}, os.Args[i+1:]...)
			main()
			os.Exit(0)
		}
	}
	os.Exit(2)
}

func TestHelpAliasesExitSuccessfullyWithoutDaemon(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"-help"}, {"--help"}, {"help"}, {"status", "--help"}, {"host", "add", "-help"}, {"setup", "--help"}, {"update", "install", "--help"}, {"serve", "-h"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "unused")
			command := exec.Command(os.Args[0], append([]string{"-test.run=^TestHelpProcess$", "--", "--data", root}, args...)...)
			command.Env = append(os.Environ(), "ANKER_HELP_PROCESS=1")
			var stdout, stderr bytes.Buffer
			command.Stdout, command.Stderr = &stdout, &stderr
			err := command.Run()
			output := stdout.Bytes()
			if err != nil || stderr.Len() != 0 {
				t.Fatalf("help must exit successfully on stdout: %v\n%s\n%s", err, output, stderr.String())
			}
			if !strings.Contains(string(output), "man anker") || !strings.Contains(string(output), "sudo anker") {
				t.Fatalf("handbook navigation missing: %s", output)
			}
			if strings.Contains(string(output), "nicht erreichbar") || strings.Contains(string(output), "Anker: ") {
				t.Fatalf("help reached service or reported an error: %s", output)
			}
			if _, err := os.Stat(root); !os.IsNotExist(err) {
				t.Fatalf("help touched data directory: %v", err)
			}
		})
	}
}

func TestNoArgumentsPrintHelpWhenRedirected(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=^TestHelpProcess$", "--")
	command.Env = append(os.Environ(), "ANKER_HELP_PROCESS=1", "ANKER_DATA="+filepath.Join(t.TempDir(), "unused"))
	output, err := command.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "man anker") {
		t.Fatalf("redirected invocation should print help: %v\n%s", err, output)
	}
}

func TestTerminalStartAndExplicitTUIUseSameEntry(t *testing.T) {
	sentinel := errors.New("terminal launched")
	for _, tc := range []struct {
		name string
		args []string
		tty  bool
	}{
		{"default terminal", nil, true},
		{"explicit terminal", []string{"tui"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			err := runWithTerminal(append([]string{"--data", t.TempDir()}, tc.args...), tc.tty, func(c *client.Client) error {
				calls++
				if c == nil {
					t.Fatal("terminal has no service client")
				}
				return sentinel
			})
			if calls != 1 || !errors.Is(err, sentinel) {
				t.Fatalf("terminal launch: calls=%d err=%v", calls, err)
			}
		})
	}
}
