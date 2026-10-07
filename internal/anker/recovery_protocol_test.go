package anker

import (
	"encoding/json"
	"testing"
)

func TestHostInventoryRoundTripPreservesRecoveryComparisonFields(t *testing.T) {
	raw := []byte(`{"hostname":"pve-test","pve_version":"9.0","interfaces":[{"name":"lo","mac":"00:00:00:00:00:00","type":"virtual","physical":false},{"name":"vmbr0","mac":"02:00:00:00:00:01","type":"bridge","physical":false},{"name":"eno1","mac":"02:00:00:00:00:01","pci":"0000:01:00.0","type":"physical","physical":true}],"disks":[{"name":"sda","id":"disk-1","size":1000000000,"type":"disk"},{"name":"sda1","id":"fs-1","size":999000000,"type":"part","parent":"sda","filesystem":"ext4","uuid":"fs-1","mount":"/"}]}`)
	var inv Inventory
	if err := json.Unmarshal(raw, &inv); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(inv)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	json.Unmarshal(encoded, &got)
	ports := got["interfaces"].([]any)
	lo := ports[0].(map[string]any)
	if physical, exists := lo["physical"]; !exists || physical != false {
		t.Fatalf("virtual physical flag lost: %s", encoded)
	}
	partition := got["disks"].([]any)[1].(map[string]any)
	if partition["parent"] != "sda" || partition["filesystem"] != "ext4" {
		t.Fatalf("partition fingerprint fields lost: %s", encoded)
	}
}
