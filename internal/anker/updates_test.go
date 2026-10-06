package anker

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestUpdateDrainGuardsJobsAndQueue(t *testing.T) {
	s := testService(t)
	s.Store.Put("jobs", "busy", Job{ID: "busy", HostID: "host1", State: "running"})
	if err := s.PrepareUpdate(); err == nil {
		t.Fatal("running restore allowed update")
	}
	s.Store.Delete("jobs", "busy")
	if err := s.PrepareUpdate(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.QueueBackup("host1"); err == nil {
		t.Fatal("new job while drained")
	}
	s.ReleaseUpdate()
	if s.Updating() {
		t.Fatal("drain not released")
	}
	_ = context.Background()
}
func TestUpdateControlOnlyAvailableThroughLocalSocket(t *testing.T) {
	s := testService(t)
	a := NewAuth(s.Store)
	for _, local := range []bool{false, true} {
		req := httptest.NewRequest("GET", "/api/update-control", nil)
		rr := httptest.NewRecorder()
		Handler(s, a, local).ServeHTTP(rr, req)
		want := 404
		if local {
			want = 200
		}
		if rr.Code != want {
			t.Fatal(local, rr.Code)
		}
	}
}
