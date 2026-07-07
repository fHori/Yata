// Package notify sends alert notifications to webhook destinations and holds
// the rule-evaluation engine that decides when to fire them.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Yata-Dash/Yata-Dash/internal/models"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

// maxSendAttempts bounds 429 retries (1 initial try + retries).
const maxSendAttempts = 4

// destLocks serialises delivery PER DESTINATION so a burst of alerts (e.g. the
// setup announce firing for several trackers at once) doesn't hammer one
// webhook with parallel POSTs and trip its rate limit. Combined with 429
// retry/backoff below, every queued message still gets delivered in order.
var destLocks sync.Map // destKey → *sync.Mutex

func destLock(dest models.NotifyDestination) *sync.Mutex {
	key := dest.Type + "|" + dest.URL + "|" + dest.Token + "|" + dest.ChatID
	m, _ := destLocks.LoadOrStore(key, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu
}

// Send delivers a title/message to one destination, formatting the payload for
// the destination type. Sends to the same destination are serialised, and a
// 429 (rate limited) is retried honouring the service's retry-after.
func Send(dest models.NotifyDestination, title, message string) error {
	var (
		url     string
		headers map[string]string
		payload map[string]any
	)
	switch strings.ToLower(dest.Type) {
	case "discord":
		url = dest.URL
		payload = map[string]any{"content": fmt.Sprintf("**%s**\n%s", title, message)}
	case "telegram":
		if dest.Token == "" || dest.ChatID == "" {
			return fmt.Errorf("telegram needs a bot token and chat id")
		}
		url = fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", dest.Token)
		payload = map[string]any{"chat_id": dest.ChatID, "text": fmt.Sprintf("*%s*\n%s", title, message), "parse_mode": "Markdown"}
	case "gotify":
		if dest.Token == "" {
			return fmt.Errorf("gotify needs an app token")
		}
		url = strings.TrimRight(dest.URL, "/") + "/message"
		headers = map[string]string{"X-Gotify-Key": dest.Token}
		payload = map[string]any{"title": title, "message": message, "priority": 5}
	case "generic":
		url = dest.URL
		payload = map[string]any{"title": title, "message": message, "sent_at": time.Now().Unix()}
	default:
		return fmt.Errorf("unknown destination type %q", dest.Type)
	}

	mu := destLock(dest)
	defer mu.Unlock()
	return postWithRetry(url, headers, payload)
}

func postWithRetry(url string, headers map[string]string, payload map[string]any) error {
	if strings.TrimSpace(url) == "" {
		return fmt.Errorf("missing URL")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var lastErr error
	for attempt := 0; attempt < maxSendAttempts; attempt++ {
		status, retryAfter, snippet, err := doPost(url, headers, body)
		if err != nil {
			return err // network/transport error — not retryable here
		}
		if status < 300 {
			return nil
		}
		if status == 429 && attempt < maxSendAttempts-1 {
			wait := retryAfter
			if wait <= 0 {
				wait = 500 * time.Millisecond
			}
			if wait > 5*time.Second {
				wait = 5 * time.Second
			}
			lastErr = fmt.Errorf("rate limited (429), retrying in %s", wait.Round(time.Millisecond))
			time.Sleep(wait)
			continue
		}
		return fmt.Errorf("destination returned %d: %s", status, snippet)
	}
	return fmt.Errorf("gave up after %d attempts: %v", maxSendAttempts, lastErr)
}

// doPost performs a single POST and returns the status, any retry-after hint,
// and a short body snippet.
func doPost(url string, headers map[string]string, body []byte) (int, time.Duration, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, 0, "", err
	}
	defer resp.Body.Close()
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 400))
	return resp.StatusCode, retryAfterFrom(resp.Header.Get("Retry-After"), snippet), strings.TrimSpace(string(snippet)), nil
}

// retryAfterFrom extracts a retry delay from the Retry-After header (seconds)
// or a JSON body with a `retry_after` field (seconds, e.g. Discord).
func retryAfterFrom(header string, body []byte) time.Duration {
	if header != "" {
		if f, err := strconv.ParseFloat(strings.TrimSpace(header), 64); err == nil && f > 0 {
			return time.Duration(f * float64(time.Second))
		}
	}
	var b struct {
		RetryAfter float64 `json:"retry_after"`
	}
	if json.Unmarshal(body, &b) == nil && b.RetryAfter > 0 {
		return time.Duration(b.RetryAfter * float64(time.Second))
	}
	return 0
}
