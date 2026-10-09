package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCLIClusterSetReferencesOnly(t *testing.T) {
	paths := []string{}
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		if r.Method == "POST" {
			json.NewDecoder(r.Body).Decode(&payload)
		}
		json.NewEncoder(w).Encode(map[string]any{"purpose": "manualSourcePlanning"})
	}))
	defer server.Close()
	c := &Client{HTTP: server.Client(), Base: server.URL}
	for _, args := range [][]string{{"cluster-set", "list"}, {"cluster-set", "capture", "berlin"}, {"cluster-set", "show", "set1"}, {"cluster-set", "export", "set1"}} {
		if e := c.Run(context.Background(), args); e != nil {
			t.Fatal(e)
		}
	}
	want := "GET /api/cluster-sets\nPOST /api/cluster-sets\nGET /api/cluster-sets/set1\nGET /api/cluster-sets/set1/export"
	if strings.Join(paths, "\n") != want || payload["cluster_id"] != "berlin" {
		t.Fatal(paths, payload)
	}
	for _, args := range [][]string{{"cluster-set", "capture"}, {"cluster-set", "restore", "set1"}, {"cluster-set", "show", "set1", "extra"}} {
		if e := c.Run(context.Background(), args); e == nil {
			t.Fatal("accepted invalid command", args)
		}
	}
}
