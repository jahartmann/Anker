package anker

import (
	"strings"
	"testing"
)

func TestSSHUsesStrictIdentityChecks(t *testing.T) {
	h := Host{Address: "192.0.2.2", SSHUser: "anker", SSHPort: 22, KeyPath: "/tmp/key", KnownHostsPath: "/tmp/known"}
	args, err := SSHArgs(h)
	if err != nil {
		t.Fatal(err)
	}
	s := strings.Join(args, " ")
	for _, want := range []string{"StrictHostKeyChecking=yes", "BatchMode=yes", "UserKnownHostsFile=/tmp/known", "IdentitiesOnly=yes", "anker@192.0.2.2"} {
		if !strings.Contains(s, want) {
			t.Fatal(s)
		}
	}
}
func TestSSHRejectsOptionInjection(t *testing.T) {
	_, err := SSHArgs(Host{Address: "-oProxyCommand=evil", SSHUser: "anker", SSHPort: 22})
	if err == nil {
		t.Fatal("accepted SSH injection")
	}
}

func TestBackupKeyCannotAuthorizeRestore(t *testing.T) {
	h := Host{Address: "192.0.2.2", SSHUser: "anker", SSHPort: 22, KeyPath: "/tmp/key", KnownHostsPath: "/tmp/known"}
	read, err := SSHArgs(h)
	if err != nil || !strings.Contains(strings.Join(read, " "), "--read-only") {
		t.Fatal("backup channel is not read-only", read, err)
	}
	if _, err := RestoreSSHArgs(h); err == nil {
		t.Fatal("restore accepted backup credentials")
	}
	h.RestoreSSHUser = "anker-restore"
	h.RestoreKeyPath = "/tmp/restore-key"
	restore, err := RestoreSSHArgs(h)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(restore, " ")
	if !strings.Contains(joined, "anker-restore@") || !strings.Contains(joined, "/tmp/restore-key") || strings.Contains(joined, "--read-only") {
		t.Fatal(joined)
	}
}
