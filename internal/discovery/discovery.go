// Package discovery finds Proxmox VE web services without sending credentials.
package discovery

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const MaxRanges = 128
const MaxAddresses = 65536

type Config struct {
	Ranges  []string `json:"ranges"`
	WebPort int      `json:"web_port"`
	SSHPort int      `json:"ssh_port"`
}
type Result struct {
	Address      string `json:"address"`
	Name         string `json:"name"`
	Version      string `json:"version"`
	SSHReachable bool   `json:"ssh_reachable"`
}
type State struct {
	Status  string   `json:"status"`
	Total   int      `json:"total"`
	Checked int      `json:"checked"`
	Results []Result `json:"results"`
	Error   string   `json:"error,omitempty"`
	Config  Config   `json:"config"`
}
type Scanner struct {
	mu     sync.Mutex
	state  State
	cancel context.CancelFunc
	// Probe is set only by tests before the scanner is used.
	Probe func(context.Context, string, Config) *Result
}

// ParseRange accepts CIDR, an IPv4 address plus contiguous dotted mask, or a single IP.
func ParseRange(value string) (netip.Prefix, error) {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 2 {
		ip, err := netip.ParseAddr(fields[0])
		mask, maskErr := netip.ParseAddr(fields[1])
		if err != nil || maskErr != nil || !ip.Is4() || !mask.Is4() {
			return netip.Prefix{}, errors.New("IPv4-Adresse und Subnetzmaske erwartet")
		}
		m := mask.As4()
		ones, bits := net.IPMask(m[:]).Size()
		if bits != 32 {
			return netip.Prefix{}, errors.New("Subnetzmaske muss zusammenhängend sein")
		}
		return netip.PrefixFrom(ip, ones).Masked(), nil
	}
	if len(fields) != 1 {
		return netip.Prefix{}, errors.New("Netzbereich fehlt oder ist ungültig")
	}
	value = fields[0]
	// Also accept 10.1.0.0/255.255.255.0, frequently used in VPN inventories.
	if pieces := strings.Split(value, "/"); len(pieces) == 2 && strings.Contains(pieces[1], ".") {
		return ParseRange(pieces[0] + " " + pieces[1])
	}
	if ip, err := netip.ParseAddr(value); err == nil && !ip.Is4In6() {
		return netip.PrefixFrom(ip, ip.BitLen()), nil
	}
	p, err := netip.ParsePrefix(value)
	if err != nil || p.Addr().Is4In6() {
		return netip.Prefix{}, errors.New("CIDR oder IPv4-Adresse mit Subnetzmaske erwartet")
	}
	return p.Masked(), nil
}
func Normalize(c Config) (Config, []string, error) {
	if c.WebPort == 0 {
		c.WebPort = 8006
	}
	if c.SSHPort == 0 {
		c.SSHPort = 22
	}
	if c.WebPort < 1 || c.WebPort > 65535 || c.SSHPort < 1 || c.SSHPort > 65535 {
		return c, nil, errors.New("Ports müssen zwischen 1 und 65535 liegen")
	}
	if len(c.Ranges) == 0 || len(c.Ranges) > MaxRanges {
		return c, nil, fmt.Errorf("1 bis %d Netzbereiche angeben", MaxRanges)
	}
	normalized := []string{}
	addresses := []string{}
	seen := map[netip.Addr]bool{}
	prefixes := map[netip.Prefix]bool{}
	for _, value := range c.Ranges {
		p, err := ParseRange(value)
		if err != nil {
			return c, nil, fmt.Errorf("Netzbereich %q: %w", value, err)
		}
		hostBits := p.Addr().BitLen() - p.Bits()
		if hostBits > 16 {
			return c, nil, fmt.Errorf("Netzbereich %s zu groß; kleinere Suchbereiche angeben (maximal %d Adressen pro Suche)", p, MaxAddresses)
		}
		if prefixes[p] {
			continue
		}
		prefixes[p] = true
		normalized = append(normalized, p.String())
		for ip := p.Addr(); ip.IsValid() && p.Contains(ip); ip = ip.Next() {
			if !ip.IsGlobalUnicast() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				return c, nil, fmt.Errorf("Netzbereich %s enthält nicht unterstützte Adressen", p)
			}
			// IPv4 /31 and /32 are valid endpoint networks, not broadcast networks.
			if ip.Is4() && p.Bits() < 31 {
				last := ip.Next()
				if ip == p.Addr() || !last.IsValid() || !p.Contains(last) {
					continue
				}
			}
			if !seen[ip] {
				seen[ip] = true
				addresses = append(addresses, ip.String())
			}
			if len(addresses) > MaxAddresses {
				return c, nil, fmt.Errorf("Maximal %d unterschiedliche Adressen pro Suche; Netze auf mehrere Suchläufe verteilen", MaxAddresses)
			}
		}
	}
	c.Ranges = normalized
	return c, addresses, nil
}

func LocalRanges() []string {
	out := []string{}
	seen := map[string]bool{}
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, _ := iface.Addrs()
		for _, address := range addresses {
			p, err := netip.ParsePrefix(address.String())
			if err != nil || !p.Addr().Is4() || !p.Addr().IsGlobalUnicast() || p.Addr().IsLinkLocalUnicast() {
				continue
			}
			name := p.Masked().String()
			if !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	sort.Strings(out)
	return out
}
func (s *Scanner) Snapshot() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.state
	if out.Status == "" {
		out.Status = "idle"
	}
	out.Results = append([]Result{}, out.Results...)
	out.Config.Ranges = append([]string{}, out.Config.Ranges...)
	sort.Slice(out.Results, func(i, j int) bool {
		a, _ := netip.ParseAddr(out.Results[i].Address)
		b, _ := netip.ParseAddr(out.Results[j].Address)
		return a.Less(b)
	})
	return out
}
func (s *Scanner) Start(c Config) error {
	c, addresses, err := Normalize(c)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return errors.New("Serversuche läuft bereits")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	s.cancel = cancel
	s.state = State{Status: "running", Total: len(addresses), Results: []Result{}, Config: c}
	go s.run(ctx, cancel, addresses, c)
	return nil
}
func (s *Scanner) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
}
func (s *Scanner) run(ctx context.Context, cancel context.CancelFunc, addresses []string, c Config) {
	defer cancel()
	probe := s.Probe
	var transport *http.Transport
	if probe == nil {
		transport = probeTransport()
		defer transport.CloseIdleConnections()
		probe = func(ctx context.Context, address string, c Config) *Result {
			return probeHost(ctx, address, c, transport)
		}
	}
	jobs := make(chan string)
	var workers sync.WaitGroup
	for i := 0; i < 64; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for address := range jobs {
				if ctx.Err() != nil {
					return
				}
				result := probe(ctx, address, c)
				s.mu.Lock()
				s.state.Checked++
				if result != nil {
					s.state.Results = append(s.state.Results, *result)
				}
				s.mu.Unlock()
			}
		}()
	}
send:
	for _, address := range addresses {
		select {
		case jobs <- address:
		case <-ctx.Done():
			break send
		}
	}
	close(jobs)
	workers.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancel = nil
	s.state.Status = "completed"
	if ctx.Err() != nil {
		s.state.Status = "cancelled"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			s.state.Error = "Zeitlimit erreicht; Ergebnisse sind unvollständig. Kleinere Suchbereiche verwenden."
		}
	}
}

// Self-signed certificates are normal on unconfigured PVE hosts. Discovery is
// untrusted evidence only: no credentials, cookies, redirects or proxy are used.
func probeTransport() *http.Transport {
	return &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DialContext: (&net.Dialer{Timeout: 1500 * time.Millisecond}).DialContext, TLSHandshakeTimeout: 2 * time.Second, DisableKeepAlives: true, MaxResponseHeaderBytes: 16384}
}

var pveTitle = regexp.MustCompile(`(?is)<title>[^<]*Proxmox Virtual Environment\s*</title>`)

func probeHost(parent context.Context, address string, c Config, transport *http.Transport) *Result {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	base := "https://" + net.JoinHostPort(address, strconv.Itoa(c.WebPort))
	get := func(path string) ([]byte, bool) {
		req, _ := http.NewRequestWithContext(ctx, "GET", base+path, nil)
		req.Header.Set("User-Agent", "Anker-Proxmox-Discovery")
		res, err := client.Do(req)
		if err != nil {
			return nil, false
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return nil, false
		}
		body, err := io.ReadAll(io.LimitReader(res.Body, 262145))
		return body, err == nil && len(body) <= 262144
	}
	// Proxmox Backup Server has a similarly shaped version endpoint. Require
	// the VE login page as well, so an open port/API alone is never a VE match.
	body, ok := get("/")
	if !ok || !pveTitle.Match(body) || !strings.Contains(string(body), "/pve2/") {
		return nil
	}
	version := ""
	if body, ok := get("/api2/json/version"); ok {
		var v struct {
			Data struct {
				Version string `json:"version"`
				Release string `json:"release"`
				RepoID  string `json:"repoid"`
			} `json:"data"`
		}
		if json.Unmarshal(body, &v) == nil && v.Data.Version != "" && v.Data.Release != "" && v.Data.RepoID != "" && len(v.Data.Version) < 100 {
			version = v.Data.Version
		}
	}
	result := &Result{Address: address, Version: version}
	dnsCtx, dnsCancel := context.WithTimeout(ctx, 500*time.Millisecond)
	if names, err := net.DefaultResolver.LookupAddr(dnsCtx, address); err == nil && len(names) > 0 {
		result.Name = strings.TrimSuffix(names[0], ".")
	}
	dnsCancel()
	sshCtx, sshCancel := context.WithTimeout(ctx, time.Second)
	defer sshCancel()
	if conn, err := (&net.Dialer{}).DialContext(sshCtx, "tcp", net.JoinHostPort(address, strconv.Itoa(c.SSHPort))); err == nil {
		result.SSHReachable = true
		conn.Close()
	}
	return result
}
