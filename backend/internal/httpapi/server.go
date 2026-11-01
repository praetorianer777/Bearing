package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/praetorianer777/bearing/backend/internal/auth"
	"github.com/praetorianer777/bearing/backend/internal/db"
	"github.com/praetorianer777/bearing/backend/internal/org"
	"github.com/praetorianer777/bearing/backend/internal/theme"
)

// APIPrefix is where every operation but the probes is served.
const APIPrefix = "/api/v1"

// Database is what the handlers need of the cluster, so tests can stand in for it.
type Database interface {
	Ping(ctx context.Context) error
	Stats() db.Stats
}

// Server holds what the handlers share.
type Server struct {
	DB    Database
	Fresh Freshness
	Log   *slog.Logger
	// Accounts says who a session signs in; Auth is the rest of signing in. Both are the one auth.Service, apart in
	// tests that need only the first.
	Accounts      Authenticator
	Auth          *auth.Service
	SessionCookie string
	SessionTTL    time.Duration
	// MailOn says whether the installation sends mail, which decides whether a forgotten password can be reset.
	MailOn bool
	// SSO is the identity provider's side of signing in; nil offers passwords only.
	SSO SingleSignOn
	// AppURL is the web client's address, where a sign-in at the provider ends.
	AppURL string
	// Secure marks the cookies the api sets Secure.
	Secure         bool
	RequestTimeout time.Duration
	// Themes keeps the installation's themes.
	Themes *theme.Service
	// Org is the organisation model.
	Org *org.Service
	// OAuth is the authorization server for partner apps; nil serves none of it and takes no bearer tokens.
	OAuth OAuth

	// router is what Routes built, which the MCP tools run operations through.
	router http.Handler
}

// Routes is the whole HTTP surface, built from the operations table.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, s.logRequests, s.recoverPanics)
	if s.RequestTimeout > 0 {
		r.Use(middleware.Timeout(s.RequestTimeout))
	}
	r.NotFound(func(w http.ResponseWriter, r *http.Request) { respondError(w, r, s.Log, ErrNotFound()) })
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) { respondError(w, r, s.Log, ErrMethodNotAllowed()) })

	api := chi.NewRouter()
	// Freshness before identity: the account a session signs in is read no older than the session's last write.
	api.Use(s.sameSite, s.readYourWrites, s.identify)
	api.NotFound(func(w http.ResponseWriter, r *http.Request) { respondError(w, r, s.Log, ErrNotFound()) })
	api.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) { respondError(w, r, s.Log, ErrMethodNotAllowed()) })
	for _, op := range operations {
		if op.tag == "oauth" && s.OAuth == nil {
			continue
		}
		h := s.guard(op, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { op.handle(s, w, r) }))
		if op.root {
			r.Method(op.method, op.path, h)
		} else {
			api.Method(op.method, op.path, h)
		}
	}
	r.Mount(APIPrefix, api)
	s.router = r
	return r
}

// logRequests logs each request once it is answered, at debug for the probes so they do not drown the log.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		level := slog.LevelInfo
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			level = slog.LevelDebug
		}
		s.Log.Log(r.Context(), level, "request", "method", r.Method, "path", r.URL.Path, "status", ww.Status(),
			"duration_ms", time.Since(start).Milliseconds(), "request_id", middleware.GetReqID(r.Context()))
	})
}

// recoverPanics turns a handler's panic into the error envelope, so a bug answers 500 and not a dropped connection.
func (s *Server) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.Log.Error("handler panicked", "panic", v, "path", r.URL.Path, "request_id", middleware.GetReqID(r.Context()))
				respondError(w, r, s.Log, ErrInternal(nil))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
