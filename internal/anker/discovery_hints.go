package anker

import (
	"anker/internal/discovery"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"sort"
	"sync"
	"time"
)

type discoveryHints struct {
	Version int              `json:"version"`
	State   string           `json:"state"`
	Hints   []discovery.Hint `json:"hints"`
}
type discoveryHintReader interface {
	DiscoveryHints(context.Context, Host) (discoveryHints, error)
}

func (c SSHCollector) DiscoveryHints(ctx context.Context, h Host) (discoveryHints, error) {
	var out discoveryHints
	err := sshRun(ctx, h, map[string]any{"version": 1, "operation": "discover"}, func(r io.Reader) error {
		decoder := json.NewDecoder(io.LimitReader(r, 256<<10))
		return decoder.Decode(&out)
	})
	if err == nil && (out.Version != 1 || len(out.Hints) > 512 || (out.State != "ok" && out.State != "partial" && out.State != "not_applicable")) {
		err = fmt.Errorf("ungültige Konfigurationshinweise")
	}
	return out, err
}
func (s *Service) discoverySeeds(ctx context.Context, c discovery.Config) ([]discovery.Hint, []string) {
	hosts, err := s.Hosts()
	if err != nil {
		return nil, []string{"Bekannte Hosts konnten nicht gelesen werden; Netzsuche läuft unabhängig weiter."}
	}
	hints := []discovery.Hint{}
	add := func(address, name, source string) {
		ip, err := netip.ParseAddr(address)
		if err != nil || !ip.IsGlobalUnicast() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			return
		}
		hints = append(hints, discovery.Hint{Address: ip.String(), Name: name, Source: source})
	}
	for _, h := range hosts {
		add(h.Address, h.Name, "known")
		if h.Inventory != nil {
			var addresses []struct {
				Info []struct {
					Local string `json:"local"`
				} `json:"addr_info"`
			}
			if json.Unmarshal(h.Inventory.Details["addresses"], &addresses) == nil {
				for _, iface := range addresses {
					for _, a := range iface.Info {
						add(a.Local, h.Name, "known")
					}
				}
			}
		}
	}
	if !c.ClustersEnabled() {
		return hints, nil
	}
	reader, ok := s.Collector.(discoveryHintReader)
	if !ok {
		return hints, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if len(hosts) > 64 {
		hosts = hosts[:64]
	}
	jobs := make(chan Host, len(hosts))
	for _, h := range hosts {
		jobs <- h
	}
	close(jobs)
	var mu sync.Mutex
	var workers sync.WaitGroup
	failed := 0
	partial := false
	for n := 0; n < 4; n++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for h := range jobs {
				var cached discoveryHints
				if s.Store.Get("discovery-hints", h.ID, &cached) == nil && cached.Version == 1 && len(cached.Hints) <= 512 {
					mu.Lock()
					for _, hint := range cached.Hints {
						if hint.Source == "cluster" || hint.Source == "pbs-config" {
							add(hint.Address, hint.Name, hint.Source)
						}
					}
					mu.Unlock()
				}
				if ctx.Err() != nil {
					mu.Lock()
					failed++
					mu.Unlock()
					continue
				}
				hostCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
				result, err := reader.DiscoveryHints(hostCtx, h)
				cancel()
				mu.Lock()
				if err != nil {
					failed++
				} else {
					partial = partial || result.State == "partial"
					for _, hint := range result.Hints {
						if hint.Source == "cluster" || hint.Source == "pbs-config" {
							add(hint.Address, hint.Name, hint.Source)
						}
					}
				}
				mu.Unlock()
				if err == nil {
					s.Store.Put("discovery-hints", h.ID, result)
				}
			}
		}()
	}
	workers.Wait()
	sort.SliceStable(hints, func(i, j int) bool {
		if hints[i].Source != hints[j].Source {
			if hints[i].Source == "known" {
				return true
			}
			if hints[j].Source == "known" {
				return false
			}
			return hints[i].Source < hints[j].Source
		}
		return hints[i].Address < hints[j].Address
	})
	warnings := []string{}
	if failed > 0 {
		warnings = append(warnings, "Konfigurationshinweise nicht von allen bekannten Hosts erreichbar. Bei älteren Hosthelfern Verbindung neu einrichten; alle ausgewählten Netze werden trotzdem geprüft.")
	}
	if partial {
		warnings = append(warnings, "Einige konfigurierte Peer-Adressen waren nicht als IP verfügbar; Hinweise können unvollständig sein.")
	}
	return hints, warnings
}
