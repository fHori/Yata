package ident

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Yata-Dash/Yata-Dash/internal/version"
)

func newReq(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "https://tracker.example/users/me", nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func TestApplyModes(t *testing.T) {
	// Default ("ua", empty, or unknown value): browser UA + Yata suffix.
	for _, mode := range []string{"", "ua", "banana"} {
		req := newReq(t)
		Apply(req, mode)
		ua := req.Header.Get("User-Agent")
		if !strings.HasPrefix(ua, "Mozilla/5.0") || !strings.Contains(ua, "Yata/"+version.Version) {
			t.Errorf("mode %q: UA must be browser-style with Yata suffix, got %q", mode, ua)
		}
		if req.Header.Get(HeaderName) != "" {
			t.Errorf("mode %q: no %s header expected", mode, HeaderName)
		}
	}

	// "header": plain browser UA + X-Yata-Version header.
	req := newReq(t)
	Apply(req, "header")
	if strings.Contains(req.Header.Get("User-Agent"), "Yata") {
		t.Errorf("header mode must keep the plain browser UA, got %q", req.Header.Get("User-Agent"))
	}
	if req.Header.Get(HeaderName) != version.Version {
		t.Errorf("header mode: want %s=%s, got %q", HeaderName, version.Version, req.Header.Get(HeaderName))
	}

	// "none": plain browser UA, nothing else.
	req = newReq(t)
	Apply(req, "none")
	if strings.Contains(req.Header.Get("User-Agent"), "Yata") || req.Header.Get(HeaderName) != "" {
		t.Error("none mode must not identify")
	}
}
