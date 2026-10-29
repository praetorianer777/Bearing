package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// tokenBytes is the entropy in a session, invitation or reset token.
const tokenBytes = 32

// newToken is a token for a cookie or a link, and the SHA-256 that is all the server keeps of it.
func newToken() (token string, hash []byte, err error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("make a token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, hashToken(token), nil
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// Sessions keeps who a session cookie signs in, in Valkey, so every api process shares them and a session ends
// everywhere at once. Only the token's hash is a key, so the store cannot be read back into cookies.
type Sessions struct {
	client *redis.Client
	ttl    time.Duration
}

// NewSessions keeps each session for ttl from its start.
func NewSessions(client *redis.Client, ttl time.Duration) *Sessions {
	return &Sessions{client: client, ttl: ttl}
}

const (
	sessionPrefix  = "bearing:session:"
	accountPrefix  = "bearing:account-sessions:"
	sessionIDBytes = 2 * sha256.Size
)

// SessionKey is what the session a token starts is known by, as Lookup returns it, so a caller can remember the
// session's first write under it before its first request.
func SessionKey(token string) string { return hex.EncodeToString(hashToken(token)) }

// ErrNoSession is a cookie that names no live session.
var ErrNoSession = errors.New("no such session")

// Start begins a session for account and returns its token, for the cookie.
func (s *Sessions) Start(ctx context.Context, account uuid.UUID) (string, error) {
	token, hash, err := newToken()
	if err != nil {
		return "", err
	}
	key := hex.EncodeToString(hash)
	pipe := s.client.TxPipeline()
	pipe.Set(ctx, sessionPrefix+key, account.String(), s.ttl)
	pipe.SAdd(ctx, accountPrefix+account.String(), key)
	pipe.Expire(ctx, accountPrefix+account.String(), s.ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return "", fmt.Errorf("keep the session: %w", err)
	}
	return token, nil
}

// Lookup is the account a session token signs in, with the key the session is known by.
func (s *Sessions) Lookup(ctx context.Context, token string) (account uuid.UUID, key string, err error) {
	if token == "" {
		return uuid.Nil, "", ErrNoSession
	}
	key = SessionKey(token)
	v, err := s.client.Get(ctx, sessionPrefix+key).Result()
	if errors.Is(err, redis.Nil) {
		return uuid.Nil, "", ErrNoSession
	}
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("read the session: %w", err)
	}
	account, err = uuid.Parse(v)
	if err != nil {
		return uuid.Nil, "", ErrNoSession
	}
	return account, key, nil
}

// End ends the session a token names; ending one that is already gone is no error.
func (s *Sessions) End(ctx context.Context, token string) error {
	account, key, err := s.Lookup(ctx, token)
	if errors.Is(err, ErrNoSession) {
		return nil
	}
	if err != nil {
		return err
	}
	pipe := s.client.TxPipeline()
	pipe.Del(ctx, sessionPrefix+key)
	pipe.SRem(ctx, accountPrefix+account.String(), key)
	_, err = pipe.Exec(ctx)
	return err
}

// EndAll ends every session of account, as a new password or a switched off account must.
func (s *Sessions) EndAll(ctx context.Context, account uuid.UUID) error {
	set := accountPrefix + account.String()
	keys, err := s.client.SMembers(ctx, set).Result()
	if err != nil {
		return fmt.Errorf("list the sessions: %w", err)
	}
	pipe := s.client.TxPipeline()
	for _, key := range keys {
		pipe.Del(ctx, sessionPrefix+key)
	}
	pipe.Del(ctx, set)
	_, err = pipe.Exec(ctx)
	return err
}
