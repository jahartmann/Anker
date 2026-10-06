package client

import (
	"anker/internal/anker"
	"bytes"
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestCLIStorageUsesServiceAndRefusesDemoGrowth(t *testing.T) {
	root := t.TempDir()
	store, err := anker.OpenStore(filepath.Join(root, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s, err := anker.NewService(root, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Demo = true
	server := httptest.NewServer(anker.Handler(s, anker.NewAuth(store), true))
	defer server.Close()
	var out bytes.Buffer
	c := &Client{HTTP: server.Client(), Base: server.URL, Out: &out}
	if err = c.Run(context.Background(), []string{"storage", "status"}); err != nil || !bytes.Contains(out.Bytes(), []byte(`"demo": true`)) {
		t.Fatal("storage command unavailable", err, out.String())
	}
	for _, args := range [][]string{{"storage", "plan", "data"}, {"storage", "grow", "data", "--plan", "abc", "--confirm", "/srv/anker"}, {"storage", "grow", "data"}, {"storage", "status", "extra"}} {
		if err = c.Run(context.Background(), args); err == nil {
			t.Fatal("invalid/demo operation accepted", args)
		}
	}
}
