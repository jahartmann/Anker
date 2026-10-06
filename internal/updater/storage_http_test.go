package updater

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStorageAPIRejectsArbitraryCommandsAndReflectsBusyState(t *testing.T) {
	m, _ := growthFixture(t)
	s := &Server{storageManager: m, state: State{Configured: true}}
	for _, body := range []string{`{"volume_id":"data","command":"mkfs.ext4 /dev/sda"}`, `{"volume_id":"data"} {}`, strings.Repeat("x", 1100)} {
		req := httptest.NewRequest("POST", "/storage/plan", bytes.NewBufferString(body))
		req.Header.Set("X-Anker-Request", "1")
		w := httptest.NewRecorder()
		s.handler(w, req)
		if w.Code != 400 {
			t.Fatal("unsafe request accepted", w.Code, w.Body.String())
		}
	}
	req := httptest.NewRequest("POST", "/storage/plan", bytes.NewBufferString(`{"volume_id":"data"}`))
	w := httptest.NewRecorder()
	s.handler(w, req)
	if w.Code != 403 {
		t.Fatal("missing mutation header accepted", w.Code)
	}
	req.Header.Set("X-Anker-Request", "1")
	w = httptest.NewRecorder()
	s.handler(w, req)
	var p map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil || w.Code != 200 || p["can_grow"] != true {
		t.Fatal("plan unavailable", w.Code, w.Body.String(), err)
	}
	s.busy = true
	if !s.snapshot().Busy {
		t.Fatal("setup cannot see storage operation")
	}
	req = httptest.NewRequest("POST", "/storage/grow", bytes.NewBufferString(`{"volume_id":"data"}`))
	req.Header.Set("X-Anker-Request", "1")
	w = httptest.NewRecorder()
	s.handler(w, req)
	if w.Code != 409 {
		t.Fatal("busy helper accepted resize", w.Code, w.Body.String())
	}
}
