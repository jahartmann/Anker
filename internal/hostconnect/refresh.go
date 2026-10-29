package hostconnect

import (
	hostassets "anker/host"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"golang.org/x/crypto/ssh"
	"net"
	"strings"
	"time"
)

const HelperVersion = "2.1"

func HelperHash() string { return fmt.Sprintf("%x", sha256.Sum256(hostassets.Helper)) }

type RefreshResult struct {
	SHA256  string `json:"sha256"`
	Version string `json:"version"`
}

// Refresh replaces only the fixed helper. Neither host profiles, authorized keys
// nor sudo policy are touched. The password never enters remote command text.
func (m Manager) Refresh(ctx context.Context, request EnrollRequest) (RefreshResult, error) {
	r, e := validate(InspectRequest{Address: request.Address, Port: request.Port})
	if e != nil {
		return RefreshResult{}, e
	}
	if !request.Confirmed || !usernamePattern.MatchString(request.Username) || len(request.Password) == 0 || len(request.Password) > 4096 || strings.ContainsAny(request.Password, "\x00\r\n") {
		return RefreshResult{}, errors.New("Bestätigte Hostidentität und Administratorzugang erforderlich")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	dir, e := m.openDir()
	if e != nil {
		return RefreshResult{}, phaseError("SSH-Konfiguration", ctx)
	}
	defer dir.Close()
	trust, e := m.readTrust(dir)
	if e != nil {
		return RefreshResult{}, phaseError("SSH-Vertrauen", ctx)
	}
	client, e := connect(ctx, r, clientConfig(request.Username, passwordMethods(request.Password), func(_ string, _ net.Addr, key ssh.PublicKey) error {
		known, changed, err := identityState(trust, endpoint(r), key)
		if err != nil || !known || changed || ssh.FingerprintSHA256(key) != request.Fingerprint {
			return errors.New("Bereits vertraute, unveränderte Hostidentität erforderlich")
		}
		return nil
	}))
	if e != nil {
		return RefreshResult{}, phaseError("Helferzugang", ctx)
	}
	defer client.Close()
	script := strings.ReplaceAll(refreshScript, "HELPER_BASE64", base64.StdEncoding.EncodeToString(hostassets.Helper))
	script = strings.ReplaceAll(script, "HELPER_SHA256", HelperHash())
	command := "python3 -c " + shellQuote(script)
	var input []byte
	if request.Username != "root" {
		command = "sh -c " + shellQuote("set -eu\nIFS= read -r anker_password\nprintf '%s\\n' \"$anker_password\" | sudo -S -p '' -v\nunset anker_password\nsudo -n "+command)
		input = []byte(request.Password + "\n")
	}
	output, e := run(client, command, input, 4096)
	if e != nil || strings.TrimSpace(string(output)) != HelperHash() {
		return RefreshResult{}, errors.New("Helferaktualisierung nicht bestätigt; Host lesend prüfen. Profile und Rollenschlüssel wurden nicht verändert.")
	}
	return RefreshResult{SHA256: HelperHash(), Version: HelperVersion}, nil
}

const refreshScript = `import os,stat,fcntl,base64,hashlib,tempfile
assert os.geteuid()==0
# Reject writable or symlinked ancestors before any replacement.
for path in ['/usr','/usr/local','/usr/local/lib','/usr/local/lib/anker','/var','/var/lib']:
 s=os.lstat(path);assert stat.S_ISDIR(s.st_mode) and s.st_uid==0 and not s.st_mode&0o022
os.makedirs('/var/lib/anker-host',mode=0o700,exist_ok=True)
s=os.lstat('/var/lib/anker-host');assert stat.S_ISDIR(s.st_mode) and s.st_uid==0 and not s.st_mode&0o022
folder='/usr/local/lib/anker';target=folder+'/anker-host'
s=os.lstat(target);assert stat.S_ISREG(s.st_mode) and s.st_uid==0 and not s.st_mode&0o022
fd=os.open('/var/lib/anker-host/restore.lock',os.O_CREAT|os.O_RDWR|os.O_NOFOLLOW|os.O_NONBLOCK,0o600)
s=os.fstat(fd);assert stat.S_ISREG(s.st_mode) and s.st_uid==0 and not s.st_mode&0o022
fcntl.flock(fd,fcntl.LOCK_EX|fcntl.LOCK_NB)
data=base64.b64decode('HELPER_BASE64');assert hashlib.sha256(data).hexdigest()=='HELPER_SHA256'
compile(data,'anker-host','exec')
old=open(target,'rb').read();assert old.startswith(b'#!/')
handle,tmp=tempfile.mkstemp(prefix='.anker-helper-',dir=folder)
try:
 os.fchmod(handle,0o755)
 with os.fdopen(handle,'wb') as out:out.write(data);out.flush();os.fsync(out.fileno())
 os.replace(tmp,target)
 directory=os.open(folder,os.O_DIRECTORY);os.fsync(directory);os.close(directory)
 assert hashlib.sha256(open(target,'rb').read()).hexdigest()=='HELPER_SHA256'
 print('HELPER_SHA256')
finally:
 if os.path.exists(tmp):os.unlink(tmp)
`
