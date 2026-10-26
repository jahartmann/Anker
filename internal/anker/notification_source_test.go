package anker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestOverdueNotificationsRejectUnusableSourceAndRecover(t *testing.T) {
	for _, kind := range []string{"integrity-error", "future-clock", "invalid-clock", "damaged", "partial"} {
		t.Run(kind, func(t *testing.T) {
			s := testService(t)
			b, err := s.CreateBackup(context.Background(), "host1")
			if err != nil {
				t.Fatal(err)
			}
			at := time.Now().UTC()
			b.CreatedAt = at.Format(time.RFC3339Nano)
			switch kind {
			case "integrity-error":
				b.VerificationError = "checksum mismatch"
			case "future-clock":
				b.CreatedAt = at.Add(24 * time.Hour).Format(time.RFC3339Nano)
			case "invalid-clock":
				b.CreatedAt = "invalid-time"
			case "damaged":
				b.Status = "damaged"
			case "partial":
				b.Status = "partial"
			}
			if err := s.Store.Put("backups", b.ID, b); err != nil {
				t.Fatal(err)
			}
			var count atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { count.Add(1); w.WriteHeader(http.StatusNoContent) }))
			defer server.Close()
			settings, err := s.Settings()
			if err != nil {
				t.Fatal(err)
			}
			settings.Webhook = server.URL
			if err := s.SaveSettings(settings); err != nil {
				t.Fatal(err)
			}
			if err := s.CheckStaleBackups(at); err != nil {
				t.Fatal(err)
			}
			if err := s.CheckStaleBackups(at.Add(time.Minute)); err != nil {
				t.Fatal(err)
			}
			if count.Load() != 1 {
				t.Fatalf("unusable source must alert once, got %d notifications", count.Load())
			}
			b.Status, b.VerificationError, b.CreatedAt = "successful", "", at.Format(time.RFC3339Nano)
			if err := s.Store.Put("backups", b.ID, b); err != nil {
				t.Fatal(err)
			}
			if err := s.CheckStaleBackups(at.Add(2 * time.Minute)); err != nil {
				t.Fatal(err)
			}
			if count.Load() != 2 {
				t.Fatalf("expected a recovery notification after a valid source, got %d", count.Load())
			}
		})
	}
}

func TestOverdueNotificationDoesNotDiscardUnreadableIncidentState(t *testing.T) {
	s := testService(t)
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { count.Add(1); w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	settings, err := s.Settings()
	if err != nil {
		t.Fatal(err)
	}
	settings.Webhook = server.URL
	if err := s.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.Put("stale_notifications", "host1", "not-an-incident-record"); err != nil {
		t.Fatal(err)
	}
	if err := s.CheckStaleBackups(time.Now()); err == nil {
		t.Fatal("unreadable incident state must be reported")
	}
	if count.Load() != 0 {
		t.Fatal("must not invent incident state and send duplicate notifications")
	}
}
