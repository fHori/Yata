// Package ident controls how Yata identifies itself in HTTP requests to
// trackers (API fetches AND profile scrapes). Several tracker staff asked to
// be able to monitor Yata's traffic once it goes live — to judge whether they
// need to request rate-limit adjustments — so identification is ON by
// default. The mechanism is def-resolved per tracker ("identify" in the
// scrape block, type → tracker cascade):
//
//	"ua"     (default) — browser-style User-Agent with a "Yata/<version>"
//	         suffix. Greppable in default access logs, filterable with a
//	         one-line nginx/WAF rule, and far less likely to trip bot
//	         heuristics than a bare non-browser UA.
//	"header" — plain browser User-Agent plus an "X-Yata-Version" request
//	         header, for sites whose session security or WAF reacts badly to
//	         UA changes (headers need a log_format tweak to be visible).
//	"none"   — plain browser User-Agent, no identification. For trackers
//	         whose protection (e.g. Cloudflare bot-fight mode) challenges
//	         anything that doesn't look exactly like a browser.
package ident

import (
	"net/http"

	"github.com/Yata-Dash/Yata-Dash/internal/version"
)

// browserUA mimics a real browser because many trackers bind sessions to the
// User-Agent and/or challenge obviously non-browser clients.
const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"

// HeaderName is the identification header sent in mode "header".
const HeaderName = "X-Yata-Version"

// Apply sets the request's identification headers for the resolved mode.
func Apply(req *http.Request, mode string) {
	switch mode {
	case "none":
		req.Header.Set("User-Agent", browserUA)
	case "header":
		req.Header.Set("User-Agent", browserUA)
		req.Header.Set(HeaderName, version.Version)
	default: // "ua" (and any unrecognised value — identify rather than hide)
		req.Header.Set("User-Agent", browserUA+" Yata/"+version.Version)
	}
}
