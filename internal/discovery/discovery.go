// Package discovery finds PVE and PBS services with resumable, credential-free probes.
package discovery

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strings"
)

const MaxRanges = 128
const MaxAddresses = 65536

type Config struct {
	Ranges          []string `json:"ranges"`
	WebPort         int      `json:"web_port"`
	SSHPort         int      `json:"ssh_port"`
	PBSPort         int      `json:"pbs_port"`
	Parallel        int      `json:"parallel"`
	UseClusterHints *bool    `json:"use_cluster_hints,omitempty"`
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
	if c.PBSPort == 0 {
		c.PBSPort = 8007
	}
	if c.Parallel == 0 {
		c.Parallel = 256
	}
	if c.Parallel < 16 || c.Parallel > 256 {
		return c, nil, errors.New("16 bis 256 parallele Adressen erlaubt")
	}
	if c.SSHPort == 0 {
		c.SSHPort = 22
	}
	if c.WebPort < 1 || c.WebPort > 65535 || c.SSHPort < 1 || c.SSHPort > 65535 || c.PBSPort < 1 || c.PBSPort > 65535 {
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

func (c Config) ClustersEnabled() bool { return c.UseClusterHints == nil || *c.UseClusterHints }
func cloneConfig(c Config) Config {
	c.Ranges = append([]string{}, c.Ranges...)
	if c.UseClusterHints != nil {
		v := *c.UseClusterHints
		c.UseClusterHints = &v
	}
	return c
}
