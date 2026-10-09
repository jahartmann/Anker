package anker

import (
	"sort"
	"strings"
)

// Source stanza keys are the same keys consumed by prepareNetwork. Ambiguous
// duplicate declarations cannot be offered as one editable stanza.
type RecoveryNetworkStanza struct {
	Key    string `json:"key"`
	Path   string `json:"path"`
	Name   string `json:"name"`
	Family string `json:"family"`
}

func recoveryNetworkReferences(files map[string]string, source Inventory) (map[string][]string, []RecoveryNetworkStanza) {
	paths := map[string][]string{}
	declarations := map[string]int{}
	candidates := []RecoveryNetworkStanza{}
	for path, data := range files {
		names, _ := networkReferences(map[string]string{path: data}, source)
		for _, name := range names {
			paths[name] = append(paths[name], path)
		}
		for _, line := range strings.Split(data, "\n") {
			code, _ := codeComment(line)
			fields := strings.Fields(code)
			if len(fields) != 4 || fields[0] != "iface" {
				continue
			}
			key := fields[1] + "/" + fields[2]
			declarations[key]++
			if fields[3] == "static" && (fields[2] == "inet" || fields[2] == "inet6") {
				candidates = append(candidates, RecoveryNetworkStanza{key, path, fields[1], fields[2]})
			}
		}
	}
	for _, p := range paths {
		sort.Strings(p)
	}
	stanzas := []RecoveryNetworkStanza{}
	for _, stanza := range candidates {
		if declarations[stanza.Key] == 1 {
			stanzas = append(stanzas, stanza)
		}
	}
	sort.Slice(stanzas, func(a, b int) bool { return stanzas[a].Key < stanzas[b].Key })
	return paths, stanzas
}
