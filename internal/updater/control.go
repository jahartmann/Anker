package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const Maintenance = "/etc/anker/update-maintenance"

type SystemControl struct {
	Socket string
	guard  string
}

func (c SystemControl) guardPath() string {
	if c.guard != "" {
		return c.guard
	}
	return Maintenance
}

func UnixClient(socket string, timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}}
}
func (c SystemControl) call(ctx context.Context, method, path string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, method, "http://anker.local/api/update-control"+path, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Anker-Request", "1")
	res, err := UnixClient(c.Socket, 10*time.Second).Do(req)
	if err != nil {
		return "", errors.New("Anker-Dienst nicht erreichbar")
	}
	defer res.Body.Close()
	var v struct {
		Version string `json:"version"`
		Error   string `json:"error"`
	}
	if err = json.NewDecoder(res.Body).Decode(&v); err != nil {
		return "", err
	}
	if res.StatusCode != 200 {
		return "", fmt.Errorf("Dienstprüfung: %s", v.Error)
	}
	return v.Version, nil
}
func (c SystemControl) Prepare(ctx context.Context) (string, error) {
	// The guard must survive a helper crash before the in-memory drain acknowledgement.
	if err := atomic(c.guardPath(), []byte("update\n"), 0640, -1, -1); err != nil {
		return "", err
	}
	version, err := c.call(ctx, "POST", "/prepare")
	if err != nil {
		c.Release(context.Background())
		return "", err
	}
	return version, nil
}
func (c SystemControl) Release(ctx context.Context) error {
	// Keep the durable retry marker until the running service has acknowledged.
	// Type=simple may have started while its socket is still unavailable at boot.
	if _, err := c.call(ctx, "POST", "/release"); err != nil {
		return err
	}
	if err := os.Remove(c.guardPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return syncDir(filepath.Dir(c.guardPath()))
}
func systemctl(ctx context.Context, action string) error {
	bounded, cancel := context.WithTimeout(ctx, 70*time.Second)
	defer cancel()
	if err := exec.CommandContext(bounded, "/usr/bin/systemctl", action, "anker.service").Run(); err != nil {
		return fmt.Errorf("Anker-Dienst konnte nicht %s werden", action)
	}
	return nil
}
func (c SystemControl) Stop(ctx context.Context) error  { return systemctl(ctx, "stop") }
func (c SystemControl) Start(ctx context.Context) error { return systemctl(ctx, "start") }
func (c SystemControl) Health(ctx context.Context, want string) error {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		version, err := c.call(ctx, "GET", "")
		if err == nil && strings.TrimPrefix(version, "v") == strings.TrimPrefix(want, "v") {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("Neue Version besteht die Startprüfung nicht")
		case <-ticker.C:
		}
	}
}
