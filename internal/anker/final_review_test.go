package anker

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestFinalReviewLegacyMetadataPrivacy(t *testing.T) {
	s, a, _ := comparisonPair(t, map[string]string{"etc/hosts": "safe"}, map[string]string{"etc/hosts": "safe"})
	value := base64.StdEncoding.EncodeToString([]byte("password=classified-final-review"))
	comparisonManifestFixture(t, s, a, func(e *Entry) { e.XAttrs = map[string]string{"user.comment": value} })
	for _, route := range []string{"/api/backups/" + a.ID + "/files", "/api/backups/" + a.ID + "/file?path=etc/hosts"} {
		w := comparisonRequest(t, s, "GET", route, nil, &User{ID: "reader", Role: "reader", Secrets: false})
		if w.Code == 200 && strings.Contains(w.Body.String(), value) {
			t.Errorf("secret xattr leaked at %s", route)
		}
	}
}

func TestFinalReviewLegacyCredentialDownloads(t *testing.T) {
	s, a, _ := comparisonPair(t, map[string]string{"etc/pve/storage.cfg": "pbs: backup\n server pbs.example\n password classified-final-review\n"}, map[string]string{"etc/pve/storage.cfg": "safe"})
	for _, suffix := range []string{"/file?path=etc/pve/storage.cfg", "/file-download?path=etc/pve/storage.cfg"} {
		route := "/api/backups/" + a.ID + suffix
		w := comparisonRequest(t, s, "GET", route, nil, &User{ID: "reader", Role: "reader", Secrets: false})
		if w.Code == 200 && strings.Contains(w.Body.String(), "classified-final-review") {
			t.Errorf("plaintext credential leaked at %s", route)
		}
	}
}

func TestFinalReviewReferenceProvenance(t *testing.T) {
	source := Inventory{PVEVersion: "8.4", Interfaces: []Interface{{Name: "eno1", Type: "physical"}}}
	files := map[string]string{"etc/network/interfaces": "iface vmbr0 inet static\n address 192.0.2.1/24\n bridge-ports eno1\n", "etc/network/interfaces.d/extra": "iface eno1 inet6 static\n address 2001:db8::1/64\n", "etc/pve/storage.cfg": "dir: data\n path /data\n", "etc/pve/qemu-server/100.conf": "scsi0: data:vm-100-disk-0\n"}
	s, b := recoverySetup(t, files, source, source)
	paths := []string{}
	for p := range files {
		paths = append(paths, p)
	}
	i, err := s.InspectRecovery(t.Context(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: paths})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(i)
	var v map[string]any
	json.Unmarshal(raw, &v)
	ports := v["ports"].([]any)
	p := ports[0].(map[string]any)
	if p["paths"] == nil || len(p["paths"].([]any)) != 2 {
		t.Fatalf("port provenance missing: %s", raw)
	}
	st := v["storage"].([]any)[0].(map[string]any)
	if st["paths"] == nil || len(st["paths"].([]any)) != 2 {
		t.Fatalf("storage provenance missing: %s", raw)
	}
	if v["network_stanzas"] == nil || len(v["network_stanzas"].([]any)) != 2 {
		t.Fatalf("source stanza choices missing: %s", raw)
	}
}

func TestFinalReviewAllBackupDisclosureBoundaries(t *testing.T) {
	for _, kind := range []string{"whitespace", "late-content", "path", "xattr", "invalid-xattr", "link"} {
		t.Run(kind, func(t *testing.T) {
			path := "etc/ordinary"
			data := "safe"
			secret := "private-final-value"
			switch kind {
			case "whitespace":
				data = "password " + secret
			case "late-content":
				data = strings.Repeat("x", comparisonBytes) + "\npassword " + secret
			case "path":
				path = "etc/shadow"
			}
			s, b, _ := comparisonPair(t, map[string]string{path: data}, map[string]string{path: "other"})
			encoded := base64.StdEncoding.EncodeToString([]byte("password " + secret))
			comparisonManifestFixture(t, s, b, func(e *Entry) {
				switch kind {
				case "xattr":
					e.XAttrs = map[string]string{"user.comment": encoded}
				case "invalid-xattr":
					e.XAttrs = map[string]string{"user.comment": "invalid*base64"}
				case "link":
					e.Link = "https://user:" + secret + "@example.test"
				}
			})
			for _, rights := range []bool{false, true} {
				user := &User{Role: "reader", Secrets: rights}
				for _, suffix := range []string{"/files", "/file?path=" + path} {
					w := comparisonRequest(t, s, "GET", "/api/backups/"+b.ID+suffix, nil, user)
					if w.Code != 200 || strings.Contains(w.Body.String(), secret) || strings.Contains(w.Body.String(), encoded) || strings.Contains(w.Body.String(), "invalid*base64") {
						t.Fatalf("default metadata leak: %s %d %s", suffix, w.Code, w.Body.String())
					}
				}
				w := comparisonRequest(t, s, "GET", "/api/backups/"+b.ID+"/file?path="+path+"&reveal=1", nil, user)
				if !rights {
					if w.Code != 403 {
						t.Fatalf("unauthorized reveal %d", w.Code)
					}
				} else if w.Code != 200 {
					t.Fatalf("authorized reveal %d", w.Code)
				}
				w = comparisonRequest(t, s, "GET", "/api/backups/"+b.ID+"/file-download?path="+path, nil, user)
				if !rights {
					if w.Code != 403 {
						t.Fatalf("unauthorized download %d", w.Code)
					}
				} else if w.Code != 200 || w.Body.String() != data {
					t.Fatal("authorized original changed")
				}
			}
		})
	}
}

func TestFinalReviewTypedNetworkRouteChangesOnlySelectedStanza(t *testing.T) {
	path := "etc/network/interfaces"
	original := "iface vmbr0 inet static\n address 192.0.2.1/24\n gateway 192.0.2.254\n dns-nameservers 192.0.2.53\niface vmbr1 inet static\n address 198.51.100.1/24\n gateway 198.51.100.254\n dns-nameservers 198.51.100.53\n"
	c, target := decisionTarget(map[string]string{path: "iface lo inet loopback\n"})
	target.Details["command_results"] = json.RawMessage(`{"file_inventory":{"state":"ok"},"pve_version":{"state":"ok"},"interfaces":{"state":"ok"},"addresses":{"state":"ok"},"routes":{"state":"ok"}}`)
	c.inv = target
	s, b := recoverySetup(t, map[string]string{path: original}, recoveryTestInventory(), target)
	s.Collector = c
	r := PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{path}, Decisions: map[string]FileDecision{path: DecisionPrepared}, ConsoleConfirmed: true, Mapping: Mapping{Network: map[string]RecoveryNetworkMapping{"vmbr0/inet": {AddressCIDR: "203.0.113.9/24", Gateway: "203.0.113.1", DNSServers: []string{"203.0.113.53", "2001:db8::53"}}}}}
	i, err := s.InspectRecovery(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	r.SnapshotID = i.SnapshotID
	w := comparisonRequest(t, s, "POST", "/api/plans", r, &User{Role: "restore", Secrets: true})
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var p Plan
	json.Unmarshal(w.Body.Bytes(), &p)
	if len(p.Blockers) != 0 || len(p.Steps) != 1 || p.Steps[0].Action != "manual" || len(p.Steps[0].Effects) != 3 {
		t.Fatalf("authority/effects mismatch %+v", p)
	}
	cmp, err := s.ComparePlanFile(p.ID, path, "source", true)
	if err != nil {
		t.Fatal(err)
	}
	changes := ""
	for _, h := range cmp.Hunks {
		for _, l := range h.Lines {
			if l.Kind != "context" {
				changes += l.Text
			}
		}
	}
	if !strings.Contains(changes, "address 203.0.113.9/24") || !strings.Contains(changes, "dns-nameservers 203.0.113.53 2001:db8::53") || strings.Contains(changes, "198.51.100.") {
		t.Fatalf("selected stanza changes: %s", changes)
	}
	if _, err = s.ApplyPlan(t.Context(), p.ID, p.ID); err == nil {
		t.Fatal("network unexpectedly writable")
	}
}

func TestFinalReviewBackupInventoryProjectionPreservesOriginal(t *testing.T) {
	source := recoveryTestInventory()
	source.Details["storage_config"] = json.RawMessage(`"password private-inventory"`)
	source.Details["command_results"] = json.RawMessage(`{"interfaces":{"state":"unexpected","stderr":"password private-inventory"}}`)
	s, b := recoverySetup(t, map[string]string{"etc/hosts": "safe"}, source, source)
	w := comparisonRequest(t, s, "GET", "/api/backups/"+b.ID+"/inventory", nil, &User{Role: "reader"})
	if w.Code != 200 || strings.Contains(w.Body.String(), "private-inventory") || strings.Contains(w.Body.String(), "details") || !strings.Contains(w.Body.String(), `"state":"unknown"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	original, err := s.Manifest(b.ID)
	raw, _ := json.Marshal(original.Inventory)
	if err != nil || !strings.Contains(string(raw), "private-inventory") {
		t.Fatal("original inventory mutated")
	}
}
