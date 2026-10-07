package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// The config and all parent directories are controlled by root; HTTP requests cannot override it.
func protectedRead(p string, max int64) ([]byte, error) {
	fd, err := unix.Open(p, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), p)
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !st.Mode().IsRegular() || st.Size() > max || !ok || sys.Uid != 0 || st.Mode().Perm()&0022 != 0 {
		return nil, errors.New("Update-Konfiguration muss root gehören und darf nicht schreibbar für andere sein")
	}
	b := make([]byte, st.Size())
	_, err = f.ReadAt(b, 0)
	return b, err
}
func LoadConfig() (Config, string, error) {
	var c Config
	b, err := protectedRead(ConfigPath, 64<<10)
	if err != nil {
		return c, "", err
	}
	if err = json.Unmarshal(b, &c); err != nil {
		return c, "", err
	}
	if err = validateConfig(c); err != nil {
		return c, "", err
	}
	token := ""
	if c.TokenFile != "" {
		b, err = protectedRead(c.TokenFile, 4096)
		if err != nil {
			return c, "", err
		}
		token = strings.TrimSpace(string(b))
	}
	return c, token, nil
}

type Server struct {
	mu             sync.Mutex
	state          State
	source         *GitHub
	installer      *Installer
	busy           bool
	tlsManager     *TLSManager
	storageManager *StorageManager
	hostManager    hostConnectionManager
}

func (s *Server) save() error {
	b, err := json.Marshal(s.state)
	if err != nil {
		return err
	}
	return atomic(filepath.Join(s.installer.StateDir, "status.json"), b, 0600, -1, -1)
}
func (s *Server) snapshot() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.state
	v.Busy = s.busy
	return v
}
func (s *Server) handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	fail := func(code int, message string) {
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]string{"error": message})
	}
	if r.URL.Path == "/configuration" || r.URL.Path == "/configure" {
		s.configurationHandler(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/tls") {
		s.tlsHandler(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/storage/") {
		s.storageHandler(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/hosts/") {
		s.hostConnectionHandler(w, r)
		return
	}
	if r.URL.Path == "/status" && r.Method == "GET" {
		json.NewEncoder(w).Encode(s.snapshot())
		return
	}
	if r.Method != "POST" || (r.URL.Path != "/check" && r.URL.Path != "/install") {
		fail(404, "Nicht gefunden")
		return
	}
	if r.Header.Get("X-Anker-Request") != "1" {
		fail(403, "Anfrageherkunft ungültig")
		return
	}
	if !s.snapshot().Configured {
		fail(503, "Signierte Updates noch nicht eingerichtet; Updatequelle in den Web-Einstellungen einrichten")
		return
	}
	var in struct {
		Version string `json:"version"`
	}
	if r.URL.Path == "/install" {
		dir := "/etc/anker"
		if s.tlsManager != nil {
			dir = s.tlsManager.Dir
		}
		if err := setupConfigurationIdle(dir); err != nil {
			fail(409, "Einrichtung unvollständig; sudo anker setup erneut ausführen")
			return
		}
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
		d.DisallowUnknownFields()
		if err := d.Decode(&in); err != nil {
			fail(400, "Version erforderlich")
			return
		}
	}
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		fail(409, "Eine Updateprüfung oder Installation läuft bereits")
		return
	}
	s.busy = true
	s.mu.Unlock()
	if r.URL.Path == "/check" {
		result, err := s.source.Check(r.Context())
		s.mu.Lock()
		s.busy = false
		s.state.CheckedAt = time.Now().UTC().Format(time.RFC3339)
		s.state.Available = nil
		if err != nil {
			s.state.Status = "check_failed"
			s.state.Message = err.Error()
		} else {
			s.state.Status = "idle"
			s.state.Message = ""
			if Newer(result.Version, s.state.Current) {
				s.state.Available = &result
			}
		}
		saveErr := s.save()
		state := s.state
		s.mu.Unlock()
		if saveErr != nil {
			fail(500, "Updatezustand konnte nicht gespeichert werden")
			return
		}
		json.NewEncoder(w).Encode(state)
		return
	}
	s.mu.Lock()
	valid := s.state.Available != nil && s.state.Available.Version == in.Version && Newer(in.Version, s.state.Current)
	if !valid {
		s.busy = false
		s.mu.Unlock()
		fail(409, "Zuerst dieses Release prüfen")
		return
	}
	s.state.Status = "installing"
	s.state.Message = "Signatur und Download werden geprüft"
	s.state.Target = in.Version
	if err := s.save(); err != nil {
		s.busy = false
		s.mu.Unlock()
		fail(500, "Updatezustand konnte nicht gespeichert werden")
		return
	}
	state := s.state
	s.mu.Unlock()
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(state)
	go s.install(in.Version)
}
func (s *Server) install(version string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	r, err := s.source.Check(ctx)
	if err == nil && r.Version != version {
		err = errors.New("Release hat sich geändert; bitte erneut prüfen")
	}
	if err == nil {
		var b []byte
		b, err = s.source.Download(ctx, r)
		if err == nil {
			err = s.installer.Install(ctx, r, b)
		}
	}
	helperErr := error(nil)
	if err == nil {
		helperErr = copyFile(BinaryPath, HelperPath, 0755, -1, -1)
	}
	s.mu.Lock()
	s.busy = false
	s.state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err != nil {
		s.state.Status = "failed"
		s.state.Message = err.Error()
	} else {
		s.state.Status = "successful"
		s.state.Message = "Update installiert und Start geprüft"
		if helperErr != nil {
			s.state.Message += "; Updater-Binärdatei konnte nicht erneuert werden. Server-Administrator muss /usr/local/libexec/anker-updater prüfen."
		}
		s.state.Current = version
		s.state.Available = nil
	}
	saveErr := s.save()
	s.mu.Unlock()
	if saveErr != nil {
		fmt.Fprintln(os.Stderr, "Updatezustand konnte nicht gespeichert werden")
	}
	// Restart the privileged helper from the newly installed, verified binary as well.
	if err == nil && helperErr == nil {
		exec.Command("/usr/bin/systemctl", "--no-block", "restart", "anker-updater.service").Run()
	}
}
func Serve(ctx context.Context, current string) error {
	if os.Geteuid() != 0 {
		return errors.New("Updater muss über den systemd-Dienst als root laufen")
	}
	c, token, err := LoadConfig()
	if err != nil {
		if _, configErr := os.Lstat(ConfigPath); !errors.Is(configErr, os.ErrNotExist) {
			return err
		}
	}
	configured := err == nil
	if err = os.MkdirAll(StateDir, 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(StateDir, "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return errors.New("Updater läuft bereits")
	}
	ctl := SystemControl{Socket: filepath.Join(DataDir, "anker.sock")}
	i := &Installer{Binary: BinaryPath, Data: DataDir, StateDir: StateDir, Control: ctl}
	hadPending := false
	if _, err = os.Stat(filepath.Join(StateDir, "pending.json")); err == nil {
		hadPending = true
	}
	if err = i.Recover(ctx); err != nil {
		return fmt.Errorf("Unterbrochenes Update: %w", err)
	}
	if i.recoveredHelper {
		// Preserve the recovery result across exec; the restored process finds
		// a completed journal but should still report what happened.
		var recovered State
		if b, readErr := os.ReadFile(filepath.Join(StateDir, "status.json")); readErr == nil {
			json.Unmarshal(b, &recovered)
		}
		recovered.Status = "rolled_back"
		recovered.Message = "Unterbrochenes Update auf vorherige Version zurückgesetzt"
		b, marshalErr := json.Marshal(recovered)
		if marshalErr != nil {
			return marshalErr
		}
		if err = atomic(filepath.Join(StateDir, "status.json"), b, 0600, -1, -1); err != nil {
			return err
		}
		// The executable was restored on disk; exec it so the old helper code
		// actually runs as well. Go's service lock closes across exec.
		return syscall.Exec(HelperPath, []string{HelperPath, "updater-serve"}, os.Environ())
	}
	activeLocalInstall := false
	if _, pendingErr := os.Stat(filepath.Join(StateDir, "pending.json")); pendingErr == nil {
		// Recovery deferred to the live local coordinator. Do not release its
		// maintenance guard or describe it as a completed rollback.
		activeLocalInstall = true
		hadPending = false
	} else if !os.IsNotExist(pendingErr) {
		return pendingErr
	}
	if hadPending {
		if _, guardErr := os.Stat(Maintenance); guardErr == nil {
			if err = ctl.Release(ctx); err != nil {
				return err
			}
		} else if !os.IsNotExist(guardErr) {
			return guardErr
		}
		if v, err := ctl.call(ctx, "GET", ""); err == nil {
			current = v
		}
	}
	if _, err := os.Stat(Maintenance); err == nil && !activeLocalInstall {
		if err = ctl.Release(ctx); err != nil {
			return err
		}
	}
	if v, err := ctl.call(ctx, "GET", ""); err == nil {
		current = v
	}
	s := &Server{source: NewGitHub(c, token), installer: i, tlsManager: &TLSManager{Dir: "/etc/anker"}, state: State{Configured: configured, Repository: c.Repository, Current: current, Status: "idle"}}
	s.storageManager = &StorageManager{ConfigDir: "/etc/anker", StateDir: StateDir}
	if err := s.storageManager.Load(); err != nil {
		fmt.Fprintln(os.Stderr, "Speicherverwaltung nicht verfügbar; Operationsjournal prüfen:", err)
		s.storageManager = nil
	}
	if b, err := os.ReadFile(filepath.Join(StateDir, "status.json")); err == nil {
		var old State
		if json.Unmarshal(b, &old) == nil && old.Repository == c.Repository {
			s.state.CheckedAt = old.CheckedAt
			s.state.UpdatedAt = old.UpdatedAt
			s.state.Message = old.Message
			s.state.Status = old.Status
			if old.Status == "installing" {
				s.state.Status = "failed"
				s.state.Message = "Update wurde unterbrochen; Installation erneut prüfen"
			}
		}
	}
	if hadPending {
		s.state.Status = "rolled_back"
		s.state.Message = "Unterbrochenes Update auf vorherige Version zurückgesetzt"
	}
	if !configured {
		s.state.Status = "unconfigured"
		s.state.Message = "Signierte Updates noch nicht eingerichtet; Updatequelle in den Web-Einstellungen einrichten."
	}
	if err = s.save(); err != nil {
		return err
	}
	if _, err = os.Lstat(Socket); err == nil {
		if existing, err := net.DialTimeout("unix", Socket, time.Second); err == nil {
			existing.Close()
			return errors.New("Update-Socket wird bereits verwendet")
		}
		if err = os.Remove(Socket); err != nil {
			return err
		}
	}
	listener, err := net.Listen("unix", Socket)
	if err != nil {
		return err
	}
	defer listener.Close()
	if err = os.Chmod(Socket, 0660); err != nil {
		return err
	}
	server := &http.Server{Handler: http.HandlerFunc(s.handler), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8 << 10}
	go s.maintainTLS(ctx)
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
