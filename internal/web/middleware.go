package web

import (
	"net/http"
	"strings"
	"time"
)

// requireAllowedHost rejects requests addressed to a hostname this deployment
// does not answer to.
//
// This is the defence against DNS rebinding, which is the real attack on an
// unauthenticated service on a home network. A page on attacker.com whose
// hostname resolves to 192.168.1.50 reaches this application as a same-origin
// request from the attacker's own origin, so checking Origin proves nothing:
// the attacker's origin is exactly what the browser sends. Checking the Host
// header is what closes it, because the attacker cannot make the browser claim
// to be talking to a name they do not control.
//
// The default policy accepts what a home server legitimately answers to and
// refuses public names, so a NAS install needs no configuration to be safe.
func (s *Server) requireAllowedHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.cfg.HostAllowed(r.Host) {
			s.log.Warn("refused a request for an unexpected host",
				"host", r.Host, "path", r.URL.Path, "remote", clientIP(r))
			http.Error(w, "Host no permitido", http.StatusMisdirectedRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// securityHeaders sets the headers that cost nothing and remove whole classes of
// problem.
//
// The content security policy needs no nonces or hashes because there is no
// inline script or style anywhere in the application, which is a constraint
// worth keeping.
func (s *Server) securityHeaders(next http.Handler) http.Handler {
	const csp = "default-src 'none'; " +
		"script-src 'self'; " +
		"style-src 'self'; " +
		"img-src 'self' data:; " +
		"font-src 'self'; " +
		"connect-src 'self'; " +
		"form-action 'self'; " +
		"base-uri 'none'; " +
		"frame-ancestors 'none'"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// statusRecorder remembers what status was written, for the access log.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusRecorder) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusRecorder) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// logRequests writes one line per request.
//
// The query string is deliberately left out. It carries what somebody searched
// their own songbook for, which is none of the log's business.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		level := s.log.Debug
		if rec.status >= 500 {
			level = s.log.Error
		} else if rec.status >= 400 {
			level = s.log.Warn
		}
		level("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"bytes", rec.bytes,
			"took", time.Since(started))
	})
}

// recoverPanics turns a bug into a 500 instead of a dropped connection.
func (s *Server) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				s.log.Error("panic while serving", "path", r.URL.Path, "value", v)
				http.Error(w, "Error interno del servidor", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// clientIP is best-effort, for a log line only. It is never used to make a
// decision, so a forged header costs nothing.
func clientIP(r *http.Request) string {
	if host, _, ok := strings.Cut(r.RemoteAddr, ":"); ok {
		return host
	}
	return r.RemoteAddr
}
