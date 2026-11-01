package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

const (
	// MaxPersonalTokens bounds the tokens one person keeps at once.
	MaxPersonalTokens = 20
	// MaxPersonalTokenDays is the longest a personal token may live.
	MaxPersonalTokenDays = 365
	// MaxTokenNameLength bounds what a person calls a token.
	MaxTokenNameLength = 100
)

// PersonalToken is a token somebody made for a script or an assistant, as they see it; never its secret.
type PersonalToken struct {
	ID   uuid.UUID
	Name string
	// Write is whether it acts as far as its person may, rather than only reading.
	Write     bool
	CreatedAt time.Time
	ExpiresAt time.Time
}

// TokenInput is a personal token to make.
type TokenInput struct {
	Name  string
	Write bool
	// Days is how long it lives, from 1 to MaxPersonalTokenDays.
	Days int
}

// ErrTokenNotFound is a token the person does not hold.
var ErrTokenNotFound = errors.New("you hold no such token")

// TokenInvalid is a token that cannot be made, by field, each with the sentence to show by it.
type TokenInvalid map[string]string

func (e TokenInvalid) Error() string {
	for _, m := range e {
		return m
	}
	return "the token cannot be made"
}

// PersonalTokens are the tokens the person ctx acts for made and that have not run out, newest first.
func (s *Service) PersonalTokens(ctx context.Context) ([]PersonalToken, error) {
	out := []PersonalToken{}
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		rows, err := q.Query(ctx, `SELECT id, name, 'write' = ANY (scopes), created_at, expires_at FROM api_token
			WHERE account_id = (SELECT current_actor_id()) AND client_id IS NULL AND expires_at > now() ORDER BY created_at DESC, id`)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (PersonalToken, error) {
			var t PersonalToken
			err := row.Scan(&t.ID, &t.Name, &t.Write, &t.CreatedAt, &t.ExpiresAt)
			return t, err
		})
		return err
	})
	return out, err
}

// MakePersonalToken makes a token for the person ctx acts for and answers its secret, which is shown this once.
func (s *Service) MakePersonalToken(ctx context.Context, id uuid.UUID, in TokenInput) (PersonalToken, string, db.LSN, error) {
	name := strings.TrimSpace(in.Name)
	invalid := TokenInvalid{}
	if name == "" || utf8.RuneCountInString(name) > MaxTokenNameLength {
		invalid["name"] = fmt.Sprintf("Name the token in 1 to %d characters, after what uses it.", MaxTokenNameLength)
	}
	if in.Days < 1 || in.Days > MaxPersonalTokenDays {
		invalid["days"] = fmt.Sprintf("A token lives from 1 to %d days.", MaxPersonalTokenDays)
	}
	if len(invalid) > 0 {
		return PersonalToken{}, "", 0, invalid
	}
	secret, digest, err := NewPrefixedToken(PersonalTokenPrefix)
	if err != nil {
		return PersonalToken{}, "", 0, err
	}
	scopes := []string{"read"}
	if in.Write {
		scopes = append(scopes, "write")
	}
	t := PersonalToken{Name: name, Write: in.Write}
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		var held int
		if err := q.QueryRow(ctx, `SELECT count(*) FROM api_token WHERE account_id = $1 AND client_id IS NULL AND expires_at > now()`, id).Scan(&held); err != nil {
			return err
		}
		if held >= MaxPersonalTokens {
			return TokenInvalid{"name": fmt.Sprintf("You hold %d tokens already, the most there may be. Revoke one you no longer use.", MaxPersonalTokens)}
		}
		if _, err := q.Exec(ctx, `DELETE FROM api_token WHERE account_id = $1 AND client_id IS NULL AND expires_at <= now()`, id); err != nil {
			return err
		}
		return q.QueryRow(ctx, `INSERT INTO api_token (account_id, name, scopes, token_hash, expires_at)
			VALUES ($1, $2, $3, $4, now() + make_interval(days => $5)) RETURNING id, created_at, expires_at`,
			id, name, scopes, digest, in.Days).Scan(&t.ID, &t.CreatedAt, &t.ExpiresAt)
	})
	if err != nil {
		return PersonalToken{}, "", lsn, err
	}
	return t, secret, lsn, nil
}

// RevokePersonalToken ends a token of the person ctx acts for at once.
func (s *Service) RevokePersonalToken(ctx context.Context, id uuid.UUID) (db.LSN, error) {
	return s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		tag, err := q.Exec(ctx, `DELETE FROM api_token WHERE id = $1 AND client_id IS NULL`, id)
		if err == nil && tag.RowsAffected() == 0 {
			return ErrTokenNotFound
		}
		return err
	})
}
