package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/Yata-Dash/Yata-Dash/internal/logging"
)

// logNoisePaths are high-frequency polling endpoints whose request lines would
// flood the log (the Logs tab polls /api/logs; QUI stats polls every few
// seconds). Their request tracing is skipped; real errors still log elsewhere.
var logNoisePaths = map[string]bool{
	"/api/logs":      true,
	"/api/qui/stats": true,
}

// requestLogger logs each HTTP request (method, path, status, duration) at
// TRACE level — trace is the request firehose, keeping Debug readable for
// app-internal diagnostics. Query strings are intentionally omitted — some
// carry credentials (e.g. QUI url/key) that must never reach the log file.
func requestLogger(lg *logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)
			if logNoisePaths[r.URL.Path] {
				return
			}
			lg.Tracef("http %d %s %s (%s)", ww.Status(), r.Method, r.URL.Path,
				time.Since(start).Round(time.Millisecond))
		})
	}
}

// Nil-safe log helpers so handlers (and tests that build Deps without a
// logger) can log without guarding every call site.
func (d *Deps) logInfof(format string, a ...any)  { if d.Log != nil { d.Log.Infof(format, a...) } }
func (d *Deps) logWarnf(format string, a ...any)  { if d.Log != nil { d.Log.Warnf(format, a...) } }
func (d *Deps) logErrorf(format string, a ...any) { if d.Log != nil { d.Log.Errorf(format, a...) } }
func (d *Deps) logDebugf(format string, a ...any) { if d.Log != nil { d.Log.Debugf(format, a...) } }

func registerLogs(r chi.Router, d *Deps) {
	r.Get("/logs", getLogs(d))
	r.Get("/logs/download", downloadLogs(d))
	r.Delete("/logs", clearLogs(d))
}

// getLogs returns recent log entries. Query params: limit (default 500),
// level (minimum level to include; default the active level).
func getLogs(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Log == nil {
			jsonOK(w, map[string]any{"entries": []any{}, "level": "info", "file": ""})
			return
		}
		limit := 500
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}
		minLevel := d.Log.GetLevel()
		if v := r.URL.Query().Get("level"); v != "" {
			minLevel = logging.ParseLevel(v)
		}
		jsonOK(w, map[string]any{
			"entries": d.Log.Recent(limit, minLevel),
			"level":   d.Log.GetLevel().String(),
			"file":    d.Log.FilePath(),
		})
	}
}

// downloadLogs streams the active log file as an attachment.
func downloadLogs(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Log == nil || d.Log.FilePath() == "" {
			jsonError(w, "no_log_file", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="yata.log"`)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		http.ServeFile(w, r, d.Log.FilePath())
	}
}

// clearLogs empties the buffer and truncates the log file.
func clearLogs(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if d.Log != nil {
			_ = d.Log.Clear()
			d.Log.Infof("logs cleared")
		}
		jsonOK(w, map[string]any{"ok": true})
	}
}
