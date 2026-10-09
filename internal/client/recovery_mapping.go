package client

import (
	"anker/internal/anker"
	"fmt"
	"net/netip"
	"strings"
)

// ParseRecoveryNetwork is shared by CLI and guided terminal inputs. Validation
// here improves feedback; stanza support and write authority remain service gates.
func ParseRecoveryNetwork(values []string) (map[string]anker.RecoveryNetworkMapping, error) {
	out := map[string]anker.RecoveryNetworkMapping{}
	for _, text := range values {
		key, raw, ok := strings.Cut(text, "=")
		parts := strings.Split(key, "/")
		fields := strings.Split(raw, "|")
		if !ok || len(parts) != 2 || parts[0] == "" || (parts[1] != "inet" && parts[1] != "inet6") || len(fields) != 3 || strings.ContainsAny(parts[0], " \t\n") {
			return nil, fmt.Errorf("Netzwerk als STANZA/inet|inet6=ADRESSE/CIDR|GATEWAY|DNS,DNS angeben")
		}
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("Netzwerkstanza %s ist mehrfach angegeben", key)
		}
		value := anker.RecoveryNetworkMapping{AddressCIDR: strings.TrimSpace(fields[0]), Gateway: strings.TrimSpace(fields[1])}
		if value.AddressCIDR != "" {
			p, err := netip.ParsePrefix(value.AddressCIDR)
			if err != nil || p.Addr().Is4() != (parts[1] == "inet") {
				return nil, fmt.Errorf("Adresse/CIDR passt nicht zur IP-Familie von %s", key)
			}
		}
		if value.Gateway != "" {
			p, err := netip.ParseAddr(value.Gateway)
			if err != nil || p.Is4() != (parts[1] == "inet") {
				return nil, fmt.Errorf("Gateway passt nicht zur IP-Familie von %s", key)
			}
		}
		if strings.TrimSpace(fields[2]) != "" {
			for _, dns := range strings.Split(fields[2], ",") {
				dns = strings.TrimSpace(dns)
				if _, err := netip.ParseAddr(dns); err != nil {
					return nil, fmt.Errorf("Ungültiger DNS-Server für %s", key)
				}
				value.DNSServers = append(value.DNSServers, dns)
			}
		}
		out[key] = value
	}
	return out, nil
}

// Missing provenance is conservatively retained for older service responses.
func RecoveryReferenceNeeded(paths []string, decisions map[string]anker.FileDecision) bool {
	if len(paths) == 0 {
		return true
	}
	for _, p := range paths {
		if decisions[p] != anker.DecisionKeep {
			return true
		}
	}
	return false
}
func RelevantRecoveryMapping(mapping anker.Mapping, inspection anker.RecoveryInspection, decisions map[string]anker.FileDecision) anker.Mapping {
	result := mapping
	result.Interfaces = map[string]string{}
	result.Storage = map[string]string{}
	result.Network = map[string]anker.RecoveryNetworkMapping{}
	for _, p := range inspection.Ports {
		if RecoveryReferenceNeeded(p.Paths, decisions) && mapping.Interfaces[p.Name] != "" {
			result.Interfaces[p.Name] = mapping.Interfaces[p.Name]
		}
	}
	for _, s := range inspection.Storage {
		if RecoveryReferenceNeeded(s.Paths, decisions) && mapping.Storage[s.ID] != "" {
			result.Storage[s.ID] = mapping.Storage[s.ID]
		}
	}
	for _, s := range inspection.NetworkStanzas {
		if RecoveryReferenceNeeded([]string{s.Path}, decisions) {
			if value, ok := mapping.Network[s.Key]; ok {
				result.Network[s.Key] = value
			}
		}
	}
	return result
}
