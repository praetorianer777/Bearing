package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/praetorianer777/bearing/backend/internal/auth"
	"github.com/praetorianer777/bearing/backend/internal/db"
)

// Authenticator says who a session token signs in; auth.Service is the real one.
type Authenticator interface {
	Authenticate(ctx context.Context, token string) (auth.Account, string, error)
}

// Principal is who a request acts for.
type Principal struct {
	Account auth.Account
	// SessionKey is what the session is known by, where the request's writes are remembered.
	SessionKey string
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
		if safeMethod(r.Method) {
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
