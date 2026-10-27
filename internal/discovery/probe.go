package discovery

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func probeTCP(ctx context.Context, address string, port int, timeout time.Duration) (bool, bool) {
	conn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", net.JoinHostPort(address, strconv.Itoa(port)))
	if err == nil {
		conn.Close()
		return true, false
	}
	return false, !errors.Is(err, syscall.ECONNREFUSED)
}

// No credentials, redirects, cookies or proxy. Self-signed certificates are
// expected; this classifier supplies candidates, never a trusted host identity.
func probeTransport() *http.Transport {
	return &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DialContext: (&net.Dialer{Timeout: 1500 * time.Millisecond}).DialContext, TLSHandshakeTimeout: 2 * time.Second, MaxResponseHeaderBytes: 16384, MaxIdleConns: 64, MaxIdleConnsPerHost: 2}
}

var pveTitle = regexp.MustCompile(`(?is)<title>[^<]*Proxmox Virtual Environment\s*</title>`)
var pbsTitle = regexp.MustCompile(`(?is)<title>[^<]*Proxmox Backup Server\s*</title>`)

func probeHost(ctx context.Context, address string, c Config, transport *http.Transport) *Result {
	return identifyHost(ctx, address, c.WebPort, c.SSHPort, transport)
}
func identifyHost(parent context.Context, address string, port, sshPort int, transport *http.Transport) *Result {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	base := "https://" + net.JoinHostPort(address, strconv.Itoa(port))
	get := func(path string) ([]byte, bool) {
		req, _ := http.NewRequestWithContext(ctx, "GET", base+path, nil)
		req.Header.Set("User-Agent", "Anker-Proxmox-Discovery")
		response, err := client.Do(req)
		if err != nil {
			return nil, false
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return nil, false
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, 262145))
		return data, err == nil && len(data) <= 262144
	}
	body, ok := get("/")
	if !ok {
		return nil
	}
	page := string(body)
	pve := pveTitle.Match(body) && strings.Contains(page, "/pve2/")
	pbs := pbsTitle.Match(body) && (strings.Contains(page, "/js/proxmox-backup-gui.js") || strings.Contains(page, "/pbs/"))
	if pve == pbs {
		return nil
	}
	kind := "pve"
	if pbs {
		kind = "pbs"
	}
	version := ""
	if data, ok := get("/api2/json/version"); ok {
		var response struct {
			Data struct{ Version, Release, RepoID string }
		}
		if json.Unmarshal(data, &response) == nil && len(response.Data.Version) < 100 {
			if pbs || (response.Data.Release != "" && response.Data.RepoID != "") {
				version = response.Data.Version
			}
		}
	}
	result := &Result{Address: address, Kind: kind, WebPort: port, Version: version}
	dnsCtx, dnsCancel := context.WithTimeout(ctx, 500*time.Millisecond)
	if names, err := net.DefaultResolver.LookupAddr(dnsCtx, address); err == nil && len(names) > 0 {
		result.Name = strings.TrimSuffix(names[0], ".")
	}
	dnsCancel()
	sshCtx, sshCancel := context.WithTimeout(ctx, time.Second)
	defer sshCancel()
	if conn, err := (&net.Dialer{}).DialContext(sshCtx, "tcp", net.JoinHostPort(address, strconv.Itoa(sshPort))); err == nil {
		result.SSHReachable = true
		conn.Close()
	}
	return result
}
