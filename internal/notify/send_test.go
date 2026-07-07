package notify

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Yata-Dash/Yata-Dash/internal/models"
)

// A destination that 429s once then succeeds should be retried and delivered.
func TestSendRetriesOn429(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.Header().Set("Retry-After", "0.05")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"retry_after":0.05}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	dest := models.NotifyDestination{Type: "generic", URL: srv.URL, Enabled: true}
	if err := Send(dest, "title", "msg"); err != nil {
		t.Fatalf("expected delivery after retry, got error: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Fatalf("expected 2 attempts (429 then 200), got %d", got)
	}
}

// A destination that always 429s should give up after maxSendAttempts.
func TestSendGivesUpOnPersistent429(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Retry-After", "0.01")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"retry_after":0.01}`))
	}))
	defer srv.Close()

	dest := models.NotifyDestination{Type: "generic", URL: srv.URL, Enabled: true}
	if err := Send(dest, "title", "msg"); err == nil {
		t.Fatal("expected an error after exhausting retries")
	}
	if got := atomic.LoadInt32(&hits); got != maxSendAttempts {
		t.Fatalf("expected %d attempts, got %d", maxSendAttempts, got)
	}
}
