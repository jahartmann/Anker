package discovery

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestMultiSiteMasksAndOverlaps(t *testing.T) {
	ranges := []string{}
	for i := 0; i < 35; i++ {
		ranges = append(ranges, fmt.Sprintf("10.%d.0.17 255.255.255.0", i))
	}
	ranges = append(ranges, "10.1.0.0/24", "10.1.0.0/255.255.255.0", "10.1.0.128/25")
	c, addresses, err := Normalize(Config{Ranges: ranges})
	if err != nil || len(addresses) != 35*254 || len(c.Ranges) != 36 || c.Ranges[0] != "10.0.0.0/24" || c.WebPort != 8006 || c.SSHPort != 22 {
		t.Fatalf("%+v count=%d %v", c, len(addresses), err)
	}
	seen := map[string]bool{}
	for _, a := range addresses {
		if seen[a] {
			t.Fatal("duplicate", a)
		}
		seen[a] = true
	}
	for _, value := range []string{"10.2.0.0/24", "10.2.0.4 255.255.255.0", "10.2.0.4/255.255.255.0"} {
		p, err := ParseRange(value)
		if err != nil || p.String() != "10.2.0.0/24" {
			t.Fatal(value, p, err)
		}
	}
	for _, entry := range []struct {
		net   string
		count int
	}{{"10.1.2.0/31", 2}, {"10.1.2.3", 1}, {"fd01::/126", 4}} {
		_, a, err := Normalize(Config{Ranges: []string{entry.net}})
		if err != nil || len(a) != entry.count {
			t.Fatal(entry, a, err)
		}
	}
}
func TestRejectInvalidAndUnboundedNetworks(t *testing.T) {
	for _, ranges := range [][]string{nil, {"10.0.0.0 255.0.255.0"}, {"10.0.0.0/8"}, {"::/64"}, {"::ffff:10.0.0.1"}, {"127.0.0.0/24"}, {"224.0.0.1/32"}, {"bad"}, {"10.0.0.0/16", "10.1.0.0/16"}} {
		if _, _, err := Normalize(Config{Ranges: ranges}); err == nil {
			t.Fatal("accepted", ranges)
		}
	}
	ranges := make([]string, 129)
	for i := range ranges {
		ranges[i] = "10.0.0.1"
	}
	if _, _, err := Normalize(Config{Ranges: ranges}); err == nil {
		t.Fatal("range limit")
	}
	if _, _, err := Normalize(Config{Ranges: []string{"10.0.0.1"}, SSHPort: -1}); err == nil {
		t.Fatal("bad port")
	}
}
func awaitDone(t *testing.T, s *Scanner) State {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		v := s.Snapshot()
		if v.Status != "running" {
			return v
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("scanner did not finish")
	return State{}
}
func TestScannerProgressCancelAndRestart(t *testing.T) {
	var calls atomic.Int32
	s := &Scanner{Probe: func(ctx context.Context, address string, c Config) *Result { calls.Add(1); <-ctx.Done(); return nil }}
	c := Config{Ranges: []string{"10.0.0.0/24"}}
	if err := s.Start(c); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(c); err == nil {
		t.Fatal("concurrent scan")
	}
	s.Cancel()
	v := awaitDone(t, s)
	if v.Status != "cancelled" || v.Checked > v.Total {
		t.Fatal(v)
	}
	s.Probe = func(ctx context.Context, address string, c Config) *Result { return &Result{Address: address} }
	if err := s.Start(Config{Ranges: []string{"10.0.0.1/32"}}); err != nil {
		t.Fatal(err)
	}
	v = awaitDone(t, s)
	if v.Checked != 1 || v.Total != 1 || len(v.Results) != 1 || v.Status != "completed" {
		t.Fatal(v)
	}
	v.Results[0].Address = "mutated"
	v.Config.Ranges[0] = "mutated"
	if s.Snapshot().Results[0].Address != "10.0.0.1" || s.Snapshot().Config.Ranges[0] != "10.0.0.1/32" {
		t.Fatal("snapshot aliases state")
	}
}
func TestProbeSelfSignedPVEAndPBSWithoutCredentials(t *testing.T) {
	for _, kind := range []string{"pve", "pve-protected-version", "pbs", "pbs-current", "unrelated", "redirect", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			var paths []string
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Method != "GET" {
					t.Error("credentials or mutation", r.Header)
				}
				paths = append(paths, r.URL.Path)
				if kind == "redirect" {
					http.Redirect(w, r, "https://example.invalid", 302)
					return
				}
				if r.URL.Path == "/api2/json/version" {
					if kind == "pve-protected-version" {
						w.WriteHeader(401)
						return
					}
					fmt.Fprint(w, `{"data":{"version":"9.0.2","release":"9.0","repoid":"01234567"}}`)
					return
				}
				switch kind {
				case "pve", "pve-protected-version":
					fmt.Fprint(w, `<title>pve01 - Proxmox Virtual Environment</title><script src="/pve2/ext6/pvemanagerlib.js"></script>`)
				case "pbs":
					fmt.Fprint(w, `<title>Proxmox Backup Server</title><script src="/pbs/app.js"></script>`)
				case "pbs-current":
					fmt.Fprint(w, `<title>pbs01 - Proxmox Backup Server</title><link href="/pve2/images/logo-128.png"/><script src="/js/proxmox-backup-gui.js"></script>`)
				case "oversized":
					fmt.Fprint(w, `<title>Proxmox Virtual Environment</title>/pve2/`)
					w.Write(make([]byte, 262145))
				default:
					fmt.Fprint(w, `<title>Other</title>`)
				}
			}))
			defer server.Close()
			host, port, _ := net.SplitHostPort(server.Listener.Addr().String())
			n, _ := strconv.Atoi(port)
			transport := probeTransport()
			defer transport.CloseIdleConnections()
			result := probeHost(context.Background(), host, Config{WebPort: n, SSHPort: n}, transport)
			want := kind == "pve" || kind == "pve-protected-version" || kind == "pbs" || kind == "pbs-current"
			if (result != nil) != want {
				t.Fatal(kind, result)
			}
			if kind == "pve" && (result.Version != "9.0.2" || !result.SSHReachable) {
				t.Fatal(result)
			}
			if strings.HasPrefix(kind, "pbs") && result.Kind != "pbs" {
				t.Fatal("PBS misclassified", result)
			}
			if kind == "redirect" && !reflect.DeepEqual(paths, []string{"/"}) {
				t.Fatal(paths)
			}
		})
	}
}
