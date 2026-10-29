package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

// ClientCookie names a browser that wrote without a session. It proves nothing: forging one only sends the
// forger's own reads to the primary.
const ClientCookie = "bearing_client"

// clientIDBytes is the entropy in a client cookie, as many as a UUID.
const clientIDBytes = 16

// Freshness is where the positions of recent writes are kept between requests; freshness.ValkeyTracker is the one
// shared across api processes.
type Freshness interface {
	Note(ctx context.Context, key string, lsn db.LSN)
	Required(ctx context.Context, key string) db.LSN
}

type ctxFreshnessKey struct{}

// writeNote carries where a request's writes are to be remembered.
type writeNote struct {
	fresh Freshness
	key   string
}

// noteWrite remembers a write's position at once, not after the handler: the response may already be on its way,
// and the next request must find it. Handlers call it before looking at the write's error, since a refused write
// may have committed something the caller must see.
func noteWrite(ctx context.Context, lsn db.LSN) {
	if n, ok := ctx.Value(ctxFreshnessKey{}).(*writeNote); ok && lsn != 0 {
		n.fresh.Note(ctx, n.key, lsn)
	}
}

// readYourWrites keeps a caller's reads off any replica that has not replayed their own last write or what the
// primary last showed them, by pinning the request.
func (s *Server) readYourWrites(next http.Handler) http.Handler {
	if s.Fresh == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := s.freshnessKey(w, r)
		if key == "" {
			next.ServeHTTP(w, r.WithContext(db.TrackReads(r.Context(), nil)))
			return
		}
		ctx := r.Context()
		if lsn := s.Fresh.Required(ctx, key); lsn != 0 {
			ctx = db.PinLSN(ctx, lsn)
		}
		// What the primary showed in a lagging replica's place is kept like a write, so the caller's next request
		// does not go back to the replica's older answer.
		asked := ctx
		ctx = db.TrackReads(ctx, func(lsn db.LSN) { s.Fresh.Note(asked, key, lsn) })
		ctx = context.WithValue(ctx, ctxFreshnessKey{}, &writeNote{fresh: s.Fresh, key: key})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// freshnessKey is the client cookie, minted for a request that may write; a read without one has no write to
// respect. Sessions and tokens take its place once there are accounts.
func (s *Server) freshnessKey(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(ClientCookie); err == nil && validClientID(c.Value) {
		return "c:" + c.Value
	}
	if safeMethod(r.Method) {
		return ""
	}
	buf := make([]byte, clientIDBytes)
	_, _ = rand.Read(buf)
	id := hex.EncodeToString(buf)
	http.SetCookie(w, &http.Cookie{Name: ClientCookie, Value: id, Path: "/", HttpOnly: true, Secure: s.Secure, SameSite: http.SameSiteLaxMode})
	return "c:" + id
}

// validClientID accepts only what freshnessKey hands out, so a cookie cannot put arbitrary keys in the store.
func validClientID(v string) bool {
	if len(v) != 2*clientIDBytes {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}
