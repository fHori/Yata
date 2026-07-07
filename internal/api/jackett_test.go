package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeJackett simulates Jackett's cookie-authenticated admin API: GET
// /UI/Login sets the test cookie (and auto-auths when no password is set),
// POST /UI/Dashboard exchanges the password for the session cookie, and the
// /api/v2.0 endpoints serve JSON only to an authenticated session (Jackett
// bounces anonymous requests to the HTML login page).
func fakeJackett(t *testing.T, password string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	authed := func(r *http.Request) bool {
		c, err := r.Cookie("Jackett")
		return err == nil && c.Value == "session"
	}
	mux.HandleFunc("/UI/Login", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "TestCookie", Value: "1", Path: "/"})
		if password == "" {
			http.SetCookie(w, &http.Cookie{Name: "Jackett", Value: "session", Path: "/"})
		}
		w.Write([]byte("<!DOCTYPE html><html>login</html>")) //nolint:errcheck
	})
	mux.HandleFunc("/UI/Dashboard", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.FormValue("password") == password {
			http.SetCookie(w, &http.Cookie{Name: "Jackett", Value: "session", Path: "/"})
		}
		w.Write([]byte("<!DOCTYPE html><html>dash</html>")) //nolint:errcheck
	})
	mux.HandleFunc("/api/v2.0/indexers", func(w http.ResponseWriter, r *http.Request) {
		if !authed(r) {
			w.Write([]byte("<!DOCTYPE html><html>login</html>")) //nolint:errcheck
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{"id":"seedpool","name":"seedpool","type":"private","configured":true,"site_link":"https://seedpool.org/"},
			{"id":"unconfigured","name":"Nope","type":"private","configured":false,"site_link":"https://nope.example/"}
		]`)) //nolint:errcheck
	})
	mux.HandleFunc("/api/v2.0/indexers/seedpool/Config", func(w http.ResponseWriter, r *http.Request) {
		if !authed(r) {
			w.Write([]byte("<!DOCTYPE html>")) //nolint:errcheck
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{"id":"apikey","type":"inputstring","name":"API Key","value":"sp-key-123"},
			{"id":"cookie","type":"inputstring","name":"Cookie","value":"session=abc"},
			{"id":"sitelink","type":"inputselect","name":"Site Link","value":"https://seedpool.org"}
		]`)) //nolint:errcheck
	})
	return httptest.NewServer(mux)
}

func TestJackettFlowWithPassword(t *testing.T) {
	srv := fakeJackett(t, "hunter2")
	defer srv.Close()

	client, err := jackettClient()
	if err != nil {
		t.Fatal(err)
	}
	if msg, _ := jackettLogin(client, srv.URL, "hunter2"); msg != "" {
		t.Fatalf("login failed: %s", msg)
	}
	list, msg, _ := jackettList(client, srv.URL)
	if msg != "" {
		t.Fatalf("list failed: %s", msg)
	}
	if len(list) != 2 || list[0].ID != "seedpool" || !list[0].Configured {
		t.Fatalf("unexpected list: %+v", list)
	}
	items := jackettConfig(client, srv.URL, "seedpool")
	got := map[string]string{}
	for _, it := range items {
		if s, ok := it.Value.(string); ok {
			got[it.ID] = s
		}
	}
	if got["apikey"] != "sp-key-123" || got["cookie"] != "session=abc" {
		t.Fatalf("credentials not extracted: %v", got)
	}
}

func TestJackettFlowNoPassword(t *testing.T) {
	srv := fakeJackett(t, "")
	defer srv.Close()

	client, _ := jackettClient()
	if msg, _ := jackettLogin(client, srv.URL, ""); msg != "" {
		t.Fatalf("auto-login failed: %s", msg)
	}
	if _, msg, _ := jackettList(client, srv.URL); msg != "" {
		t.Fatalf("list failed after auto-login: %s", msg)
	}
}

func TestJackettWrongPasswordIsAuthError(t *testing.T) {
	srv := fakeJackett(t, "hunter2")
	defer srv.Close()

	client, _ := jackettClient()
	if msg, _ := jackettLogin(client, srv.URL, "wrong"); msg != "" {
		t.Fatalf("login POST itself should not error: %s", msg)
	}
	_, msg, status := jackettList(client, srv.URL)
	if msg != "invalid Jackett admin password" || status != http.StatusUnauthorized {
		t.Fatalf("want auth error, got %q (%d)", msg, status)
	}
}
