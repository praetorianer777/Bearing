package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

// SignInWithIdentity signs in somebody single sign-on vouches for. An identity seen before signs in its account; a
// verified address matching an account made ahead of time is linked to it; somebody in a group an administrator
// granted a role gets an account of their own as a reader, and holds the group's role while they are in it. Anybody else asks to join, and ErrAwaitingAdmission says so. The
// groups the provider names replace the ones it named last time. It returns the sign-in's write position, for the
// session's first reads to respect.
func (s *Service) SignInWithIdentity(ctx context.Context, id Identity) (Account, string, db.LSN, error) {
	a, lsn, err := s.AccountForIdentity(ctx, id)
	if err != nil {
		return Account{}, "", lsn, err
	}
	s.signedIn(ctx, a.ID)
	token, err := s.Sessions.Start(ctx, a.ID)
	return a, token, lsn, err
}

// AccountForIdentity is the account somebody single sign-on vouches for has, found or made as a sign-in finds or
// makes it, without a session: a partner trading their token acts as them. Somebody who has none asks to join, and
// ErrAwaitingAdmission says so; a switched off account is ErrDisabled.
func (s *Service) AccountForIdentity(ctx context.Context, id Identity) (Account, db.LSN, error) {
	var (
		accountID uuid.UUID
		awaiting  bool
	)
	lsn, err := s.DB.WriteAdmin(ctx, func(ctx context.Context, q db.DBTX) error {
		err := q.QueryRow(ctx, `UPDATE account_identity SET groups = $3, last_sign_in = now()
			WHERE issuer = $1 AND subject = $2 RETURNING account_id`, id.Issuer, id.Subject, id.Groups).Scan(&accountID)
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		// An account made ahead of time for this address, which no identity of this provider has claimed.
		err = q.QueryRow(ctx, `SELECT id FROM account a WHERE email = $1 AND NOT EXISTS (
			SELECT 1 FROM account_identity i WHERE i.account_id = a.id AND i.issuer = $2)`, id.Email, id.Issuer).Scan(&accountID)
		if errors.Is(err, pgx.ErrNoRows) {
			var granted bool
			err = q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM group_grant WHERE group_name = ANY($1))`, id.Groups).Scan(&granted)
			if err == nil && !granted {
				_, err = q.Exec(ctx, `INSERT INTO join_request (issuer, subject, email, name, groups) VALUES ($1, $2, $3, $4, $5)
					ON CONFLICT (issuer, subject) DO UPDATE SET email = $3, name = $4, groups = $5, requested_at = now()`,
					id.Issuer, id.Subject, id.Email, id.Name, id.Groups)
				// The request is committed, and the refusal said after: an error here would roll the request back.
				awaiting = err == nil
				return err
			}
			if err != nil {
				return err
			}
			err = q.QueryRow(ctx, `INSERT INTO account (email, name, role) VALUES ($1, $2, 'reader') RETURNING id`,
				id.Email, strings.TrimSpace(id.Name)).Scan(&accountID)
			if err := uniqueEmail(err); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `INSERT INTO account_identity (account_id, issuer, subject, groups) VALUES ($1, $2, $3, $4)`,
			accountID, id.Issuer, id.Subject, id.Groups); err != nil {
			return err
		}
		_, err = q.Exec(ctx, `DELETE FROM join_request WHERE issuer = $1 AND subject = $2`, id.Issuer, id.Subject)
		return err
	})
	if err != nil {
		return Account{}, 0, err
	}
	if awaiting {
		return Account{}, lsn, ErrAwaitingAdmission
	}
	a, err := s.Account(db.PinPrimary(db.WithUser(ctx, accountID)), accountID)
	if err != nil {
		return Account{}, 0, err
	}
	if a.Disabled {
		return Account{}, 0, ErrDisabled
	}
	return a, lsn, nil
}

// JoinRequests lists who asked to join, as an administrator.
func (s *Service) JoinRequests(ctx context.Context) ([]JoinRequest, error) {
	var out []JoinRequest
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		rows, err := q.Query(ctx, `SELECT id, email, name, groups, requested_at FROM join_request ORDER BY requested_at`)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (JoinRequest, error) {
			var j JoinRequest
			return j, r.Scan(&j.ID, &j.Email, &j.Name, &j.Groups, &j.RequestedAt)
		})
		return err
	})
	return out, err
}

// Admit lets somebody in who asked to join, with a role, as an administrator; their next sign-in signs them in.
func (s *Service) Admit(ctx context.Context, requestID uuid.UUID, role Role) (Account, db.LSN, error) {
	if !role.Valid() {
		return Account{}, 0, ErrInvalidRole
	}
	var a Account
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		var id Identity
		err := q.QueryRow(ctx, `DELETE FROM join_request WHERE id = $1 RETURNING issuer, subject, email, name, groups`, requestID).
			Scan(&id.Issuer, &id.Subject, &id.Email, &id.Name, &id.Groups)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		var accountID uuid.UUID
		if err := uniqueEmail(q.QueryRow(ctx, `INSERT INTO account (email, name, role) VALUES ($1, $2, $3) RETURNING id`,
			id.Email, id.Name, role).Scan(&accountID)); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `INSERT INTO account_identity (account_id, issuer, subject, groups) VALUES ($1, $2, $3, $4)`,
			accountID, id.Issuer, id.Subject, id.Groups); err != nil {
			return err
		}
		return scanAccount(q.QueryRow(ctx, `SELECT `+accountColumns+` FROM account WHERE id = $1`, accountID), &a)
	})
	return a, lsn, err
}

// Decline turns a request to join down, as an administrator. Asking again at the next sign-in stays possible.
func (s *Service) Decline(ctx context.Context, requestID uuid.UUID) (db.LSN, error) {
	return s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		return affected(q.Exec(ctx, `DELETE FROM join_request WHERE id = $1`, requestID))
	})
}

// GroupGrants lists the groups whose members come in at once, as an administrator.
func (s *Service) GroupGrants(ctx context.Context) ([]GroupGrant, error) {
	var out []GroupGrant
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		rows, err := q.Query(ctx, `SELECT group_name, role FROM group_grant ORDER BY group_name`)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (GroupGrant, error) {
			var g GroupGrant
			return g, r.Scan(&g.Group, &g.Role)
		})
		return err
	})
	return out, err
}

// SetGroupGrants replaces every group grant with these, as an administrator.
func (s *Service) SetGroupGrants(ctx context.Context, grants []GroupGrant) (db.LSN, error) {
	for _, a := range grants {
		if !a.Role.Valid() {
			return 0, ErrInvalidRole
		}
	}
	return s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		if _, err := q.Exec(ctx, `DELETE FROM group_grant`); err != nil {
			return err
		}
		for _, a := range grants {
			if _, err := q.Exec(ctx, `INSERT INTO group_grant (group_name, role) VALUES ($1, $2)
				ON CONFLICT (group_name) DO UPDATE SET role = excluded.role`, strings.TrimSpace(a.Group), a.Role); err != nil {
				return err
			}
		}
		return nil
	})
}

// Prepare makes accounts without a password for addresses single sign-on will bring, so their first sign-in links
// to them with the role given. An address that has an account already is left as it is.
func (s *Service) Prepare(ctx context.Context, members map[string]Role) error {
	_, err := s.DB.WriteAdmin(ctx, func(ctx context.Context, q db.DBTX) error {
		for email, role := range members {
			if !role.Valid() {
				return ErrInvalidRole
			}
			name, _, _ := strings.Cut(email, "@")
			if _, err := q.Exec(ctx, `INSERT INTO account (email, name, role) VALUES ($1, $2, $3) ON CONFLICT (email) DO NOTHING`,
				email, name, role); err != nil {
				return err
			}
		}
		return nil
	})
	return err
}
