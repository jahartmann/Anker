package updater

import (
	"anker/internal/hostconnect"
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/sys/unix"
	"io"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type hostConnectionManager interface {
	Inspect(context.Context, hostconnect.InspectRequest) (hostconnect.Identity, error)
	Enroll(context.Context, hostconnect.EnrollRequest) (hostconnect.Enrollment, error)
}

func (s *Server) hostConnectionHandler(w http.ResponseWriter, r *http.Request) {
	reject := func(code int, message string) {
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]string{"error": message})
	}
	if r.Method != "POST" || (r.URL.Path != "/hosts/inspect" && r.URL.Path != "/hosts/enroll") {
		reject(405, "Methode nicht unterstützt")
		return
	}
	if r.Header.Get("X-Anker-Request") != "1" {
		reject(403, "Anfrageherkunft ungültig")
		return
	}
	inspect := r.URL.Path == "/hosts/inspect"
	var inspection hostconnect.InspectRequest
	var enrollment hostconnect.EnrollRequest
	var in any = &enrollment
	if inspect {
		in = &inspection
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(in) != nil || decoder.Decode(new(any)) != io.EOF {
		reject(400, "Angaben zur Hostanbindung ungültig")
		return
	}
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		reject(409, "Update oder Systemänderung läuft; anschließend erneut versuchen")
		return
	}
	s.busy = true
	s.mu.Unlock()
	defer func() { enrollment.Password = ""; s.mu.Lock(); s.busy = false; s.mu.Unlock() }()
	manager := s.configurationManager()
	unlock, err := manager.lock()
	if err != nil {
		reject(409, "Einrichtung läuft oder ist unvollständig; zuerst abschließen")
		return
	}
	defer unlock()
	if setupConfigurationIdle(manager.Dir) != nil {
		reject(409, "Einrichtung unvollständig; zuerst abschließen")
		return
	}
	stateDir := StateDir
	if s.installer != nil {
		stateDir = s.installer.StateDir
	}
	for _, marker := range []string{filepath.Join(manager.Dir, "update-maintenance"), filepath.Join(stateDir, "pending.json")} {
		if _, err = os.Lstat(marker); !os.IsNotExist(err) {
			reject(409, "Update oder Wartung läuft; zuerst abschließen")
			return
		}
	}
	fd, err := unix.Open(filepath.Join(stateDir, "local-install.lock"), unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0600)
	if err != nil {
		reject(409, "Updatesperre nicht verfügbar")
		return
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Uid != manager.ownerUID || stat.Mode&0022 != 0 {
		reject(400, "Ungültige Updatesperre")
		return
	}
	if unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB) != nil {
		reject(409, "Lokale Installation läuft; anschließend erneut versuchen")
		return
	}
	connector := s.hostManager
	if connector == nil {
		group, err := user.LookupGroup("anker")
		if err != nil {
			reject(503, "Anker-Systembenutzer fehlt; Einrichtung erneut ausführen")
			return
		}
		gid, err := strconv.Atoi(group.Gid)
		if err != nil {
			reject(503, "Anker-Systemgruppe ungültig")
			return
		}
		account, err := user.Lookup("anker")
		if err != nil {
			reject(503, "Anker-Systembenutzer fehlt; Einrichtung erneut ausführen")
			return
		}
		uid, err := strconv.Atoi(account.Uid)
		if err != nil {
			reject(503, "Anker-Systembenutzer ungültig")
			return
		}
		connector = hostconnect.Manager{Dir: manager.Dir, OwnerUID: int(manager.ownerUID), GroupGID: gid, KeyOwnerUID: uid}
	}
	timeout := 5 * time.Minute
	if inspect {
		timeout = 20 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	var result any
	if inspect {
		result, err = connector.Inspect(ctx, inspection)
	} else {
		result, err = connector.Enroll(ctx, enrollment)
	}
	if err != nil {
		message := err.Error()
		if enrollment.Password != "" {
			message = strings.ReplaceAll(message, enrollment.Password, "[geschützt]")
		}
		code := 400
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			code = 408
			message = "Verbindung unterbrochen oder Zeitlimit erreicht; Einrichtung erneut versuchen"
		}
		reject(code, message)
		return
	}
	json.NewEncoder(w).Encode(result)
}
