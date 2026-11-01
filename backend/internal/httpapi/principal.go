package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/auth"
	"github.com/praetorianer777/bearing/backend/internal/db"
	"github.com/praetorianer777/bearing/backend/internal/oauth"
)

// Authenticator says who a session token signs in, and reads the account a bearer token acts for; auth.Service is
// the real one.
type Authenticator interface {
	Authenticate(ctx context.Context, token string) (auth.Account, string, error)
	Account(ctx context.Context, id uuid.UUID) (auth.Account, error)
}

// Principal is who a request acts for.
type Principal struct {
	Account auth.Account
	// SessionKey is what the session is known by, where the request's writes are remembered.
	SessionKey string
	// Via is the bearer token the request acts through; nil for a session.
	Via *oauth.Via
}

type principalKey struct{}

// PrincipalFrom is who the request acts for, nil for nobody.
func PrincipalFrom(ctx context.Context) *Principal {
	p, _ := ctx.Value(principalKey{}).(*Principal)
	return p
}

// identify reads the session cookie and, when it names a live session, has the request act for its account, down
// to the database's policies. A cookie that names nothing is dropped, so the browser stops sending it.
func (s *Server) identify(next http.Handler) http.Handler {
	if s.Accounts == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok && s.OAuth != nil {
			s.bearer(w, r, next, strings.TrimSpace(token))
			return
		}
		c, err := r.Cookie(s.cookieName())
		if err != nil || c.Value == "" {
			next.ServeHTTP(w, r)
			return
		}
		account, key, err := s.Accounts.Authenticate(r.Context(), c.Value)
		if errors.Is(err, auth.ErrNoSession) {
			s.clearSession(w)
			next.ServeHTTP(w, r)
			return
		}
		if err != nil {
			respondError(w, r, s.Log, ErrUnavailable("Signing in cannot be checked right now.", err))
			return
		}
		ctx := context.WithValue(r.Context(), principalKey{}, &Principal{Account: account, SessionKey: key})
		next.ServeHTTP(w, r.WithContext(db.WithUser(ctx, account.ID)))
	})
}

// bearer has the request act for whom a bearer token acts for, through the token. A token that acts for nobody is
// refused outright rather than read as nobody signed in, so a client learns its token is spent.
func (s *Server) bearer(w http.ResponseWriter, r *http.Request, next http.Handler, token string) {
	id, via, err := s.OAuth.Bearer(r.Context(), token)
	if errors.Is(err, oauth.ErrNoToken) {
		s.challenge(w, r, "invalid_token", "This token is unknown, expired or revoked.")
		return
	}
	if err != nil {
		respondError(w, r, s.Log, ErrUnavailable("The token cannot be checked right now.", err))
		return
	}
	ctx := db.WithVia(db.WithUser(r.Context(), id), via.ID)
	account, err := s.Accounts.Account(ctx, id)
	if errors.Is(err, auth.ErrNotFound) {
		// An account made a moment ago may not have reached the replica yet; the token was read on the primary.
		account, err = s.Accounts.Account(db.PinPrimary(ctx), id)
	}
	if errors.Is(err, auth.ErrNotFound) || (err == nil && account.Disabled) {
		s.challenge(w, r, "invalid_token", "This token is unknown, expired or revoked.")
		return
	}
	if err != nil {
		respondError(w, r, s.Log, ErrUnavailable("The token cannot be checked right now.", err))
		return
	}
	ctx = context.WithValue(ctx, principalKey{}, &Principal{Account: account, Via: &via})
	next.ServeHTTP(w, r.WithContext(ctx))
}

// challenge refuses a bearer request as RFC 6750 asks, naming where the protected resource's metadata is.
func (s *Server) challenge(w http.ResponseWriter, r *http.Request, code, message string) {
	header := `Bearer realm="bearing"`
	if s.OAuth != nil {
		header += `, resource_metadata="` + s.OAuth.ResourceMetadataURL() + `"`
	}
	if code != "" {
		header += `, error="` + code + `"`
	}
	w.Header().Set("WWW-Authenticate", header)
	respondError(w, r, s.Log, &APIError{Status: http.StatusUnauthorized, Code: "unauthorized", Message: message})
}

// guard holds an operation to who may call it: public ones anybody, the rest somebody signed in with the role the
// operation needs. The database's policies hold the same line again, for a handler that strays.
func (s *Server) guard(op operation, h http.Handler) http.Handler {
	if op.public {
		return h
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := PrincipalFrom(r.Context())
		if p == nil {
			respondError(w, r, s.Log, ErrUnauthorized())
			return
		}
		if p.Via != nil && p.Via.MCPOnly && !op.mcp {
			respondError(w, r, s.Log, ErrForbidden("This token was given to a connected app for the MCP endpoint, and acts there only."))
			return
		}
		if p.Via != nil && p.Via.ReadOnly && !safeMethod(op.method) && !op.mcp {
			respondError(w, r, s.Log, ErrForbidden("This token may only read."))
			return
		}
		if op.role != "" && !p.Account.Role.AtLeast(op.role) {
			respondError(w, r, s.Log, ErrForbidden(roleNeeded(op.role)))
			return
		}
		h.ServeHTTP(w, r)
	})
}

// sameSite refuses a write that a browser says came from another site, so a page elsewhere cannot use the
// session cookie. Browsers name the site in Sec-Fetch-Site; older ones only in Origin. A caller that sends neither,
// such as a script, carries no cookie of a browser's.
func (s *Server) sameSite(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if safeMethod(r.Method) || crossSiteOK(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
			if site != "same-origin" && site != "none" {
				respondError(w, r, s.Log, ErrCrossSite())
				return
			}
		} else if origin := r.Header.Get("Origin"); origin != "" {
			if u, err := url.Parse(origin); err != nil || u.Host != r.Host {
				respondError(w, r, s.Log, ErrCrossSite())
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) cookieName() string {
	if s.SessionCookie == "" {
		return "bearing_session"
	}
	return s.SessionCookie
}

func (s *Server) setSession(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Value: token, Path: "/", HttpOnly: true, Secure: s.Secure,
		SameSite: http.SameSiteLaxMode, MaxAge: int(s.SessionTTL / time.Second)})
}

func (s *Server) clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Value: "", Path: "/", HttpOnly: true, Secure: s.Secure,
		SameSite: http.SameSiteLaxMode, MaxAge: -1})
}

func roleNeeded(role auth.Role) string {
	if role == auth.RoleAdministrator {
		return "Only an administrator may do that."
	}
	return "That needs the " + string(role) + " role or a higher one."
}

// crossSiteOK are the endpoints apps elsewhere call by design. None of them reads the session cookie, so a page
// elsewhere gains nothing by sending it.
func crossSiteOK(path string) bool {
	for _, p := range []string{oauth.RegisterPath, oauth.TokenPath, oauth.RevokePath} {
		if strings.HasSuffix(path, p) {
			return true
		}
	}
	return false
}
