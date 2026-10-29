package auth

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/praetorianer777/bearing/backend/internal/db"
	mailer "github.com/praetorianer777/bearing/backend/internal/mail"
)

// MailJob is the kind of job that sends one mail; the worker handles it.
const MailJob = "mail.send"

// Enqueuer adds a job for the worker; jobs.Queue is the real one.
type Enqueuer interface {
	Enqueue(ctx context.Context, kind string, payload any) (string, error)
}

// Service is the accounts and how people come in.
type Service struct {
	DB       *db.Cluster
	Sessions *Sessions
	// Mail queues the mails the service sends; nil sends none, and the links are handed on by hand.
	Mail          Enqueuer
	AppURL        string
	Params        PasswordParams
	InvitationTTL time.Duration
	ResetTTL      time.Duration
}

// decoyHash is verified against when an email names no account, so a wrong address takes as long as a wrong
// password and the time does not say which addresses have accounts.
var decoyHash = func() string {
	h, err := HashPassword("the decoy password nobody has", DefaultPasswordParams())
	if err != nil {
		panic(err)
	}
	return h
}()

const accountColumns = `id, email, name, role, disabled_at IS NOT NULL, created_at`

func scanAccount(row pgx.Row, a *Account) error {
	return row.Scan(&a.ID, &a.Email, &a.Name, &a.Role, &a.Disabled, &a.CreatedAt)
}

// NormaliseEmail trims an address and refuses one that is not an address.
func NormaliseEmail(email string) (string, error) {
	email = strings.TrimSpace(email)
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || len(email) > 254 {
		return "", fmt.Errorf("%q is not an email address", email)
	}
	return email, nil
}

// Installation is what the installation is called and whether it is set up; anybody may ask.
func (s *Service) Installation(ctx context.Context) (Installation, error) {
	var in Installation
	err := s.DB.ReadAdmin(ctx, func(ctx context.Context, q db.DBTX) error {
		err := q.QueryRow(ctx, `SELECT name FROM installation`).Scan(&in.Name)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		in.SetUp = err == nil
		return err
	})
	return in, err
}

// SetUp names the installation and makes its first administrator, once.
func (s *Service) SetUp(ctx context.Context, name, adminName, email, password string) (Account, error) {
	if err := ValidatePassword(password); err != nil {
		return Account{}, err
	}
	hash, err := HashPassword(password, s.Params)
	if err != nil {
		return Account{}, err
	}
	var a Account
	_, err = s.DB.WriteAdmin(ctx, func(ctx context.Context, q db.DBTX) error {
		// The lock makes two people setting up at once queue, so the second finds the first's row.
		if _, err := q.Exec(ctx, `LOCK TABLE installation IN EXCLUSIVE MODE`); err != nil {
			return err
		}
		var exists bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM installation)`).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return ErrAlreadySetUp
		}
		if _, err := q.Exec(ctx, `INSERT INTO installation (name) VALUES ($1)`, strings.TrimSpace(name)); err != nil {
			return err
		}
		return scanAccount(q.QueryRow(ctx, `INSERT INTO account (email, name, password_hash, role)
			VALUES ($1, $2, $3, 'administrator') RETURNING `+accountColumns, email, strings.TrimSpace(adminName), hash), &a)
	})
	a.HasPassword = err == nil
	return a, err
}

// SignIn checks an email and password and starts a session, returning its token.
func (s *Service) SignIn(ctx context.Context, email, password string) (Account, string, error) {
	var (
		a    Account
		hash *string
	)
	err := s.DB.ReadAdmin(ctx, func(ctx context.Context, q db.DBTX) error {
		return q.QueryRow(ctx, `SELECT `+accountColumns+`, password_hash FROM account WHERE email = $1`, strings.TrimSpace(email)).
			Scan(&a.ID, &a.Email, &a.Name, &a.Role, &a.Disabled, &a.CreatedAt, &hash)
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Account{}, "", err
	}
	stored := decoyHash
	if hash != nil {
		stored = *hash
	}
	ok, rehash, verr := VerifyPassword(password, stored, s.Params)
	if verr != nil || !ok || hash == nil {
		return Account{}, "", ErrBadCredentials
	}
	if a.Disabled {
		return Account{}, "", ErrDisabled
	}
	a.HasPassword = true
	if rehash {
		if fresh, err := HashPassword(password, s.Params); err == nil {
			_, _ = s.DB.WriteAdmin(ctx, func(ctx context.Context, q db.DBTX) error {
				_, err := q.Exec(ctx, `UPDATE account SET password_hash = $2 WHERE id = $1`, a.ID, fresh)
				return err
			})
		}
	}
	token, err := s.Sessions.Start(ctx, a.ID)
	return a, token, err
}

// Authenticate is the account a session token signs in; a switched off account's sessions sign nobody in.
func (s *Service) Authenticate(ctx context.Context, token string) (Account, string, error) {
	id, key, err := s.Sessions.Lookup(ctx, token)
	if err != nil {
		return Account{}, "", err
	}
	ctx = db.WithUser(ctx, id)
	a, err := s.Account(ctx, id)
	if errors.Is(err, ErrNotFound) {
		// An account made a moment ago, as accepting an invitation does, may not have reached the replica yet.
		a, err = s.Account(db.PinPrimary(ctx), id)
	}
	if errors.Is(err, ErrNotFound) || (err == nil && a.Disabled) {
		return Account{}, "", ErrNoSession
	}
	return a, key, err
}

// Account is one account, read as the person ctx acts for.
func (s *Service) Account(ctx context.Context, id uuid.UUID) (Account, error) {
	var a Account
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		return scanAccount(q.QueryRow(ctx, `SELECT `+accountColumns+` FROM account WHERE id = $1`, id), &a)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	if err != nil {
		return a, err
	}
	a.HasPassword, err = s.hasPassword(ctx, id)
	return a, err
}

func (s *Service) hasPassword(ctx context.Context, id uuid.UUID) (bool, error) {
	var has bool
	err := s.DB.ReadAdmin(ctx, func(ctx context.Context, q db.DBTX) error {
		return q.QueryRow(ctx, `SELECT password_hash IS NOT NULL FROM account WHERE id = $1`, id).Scan(&has)
	})
	return has, err
}

// SignOut ends the session a token names.
func (s *Service) SignOut(ctx context.Context, token string) error { return s.Sessions.End(ctx, token) }

// Accounts lists every account, as the person ctx acts for sees them.
func (s *Service) Accounts(ctx context.Context) ([]Account, error) {
	var out []Account
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		rows, err := q.Query(ctx, `SELECT `+accountColumns+` FROM account ORDER BY lower(name), email`)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (Account, error) {
			var a Account
			return a, scanAccount(r, &a)
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	withPassword := map[uuid.UUID]bool{}
	err = s.DB.ReadAdmin(ctx, func(ctx context.Context, q db.DBTX) error {
		rows, err := q.Query(ctx, `SELECT id FROM account WHERE password_hash IS NOT NULL`)
		if err != nil {
			return err
		}
		ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		for _, id := range ids {
			withPassword[id] = true
		}
		return err
	})
	for i := range out {
		out[i].HasPassword = withPassword[out[i].ID]
	}
	return out, err
}

// CreateAccount makes a local account with a password, as an administrator.
func (s *Service) CreateAccount(ctx context.Context, name, email string, role Role, password string) (Account, db.LSN, error) {
	if !role.Valid() {
		return Account{}, 0, ErrInvalidRole
	}
	if err := ValidatePassword(password); err != nil {
		return Account{}, 0, err
	}
	hash, err := HashPassword(password, s.Params)
	if err != nil {
		return Account{}, 0, err
	}
	var a Account
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		return scanAccount(q.QueryRow(ctx, `INSERT INTO account (email, name, password_hash, role) VALUES ($1, $2, $3, $4)
			RETURNING `+accountColumns, email, strings.TrimSpace(name), hash, role), &a)
	})
	a.HasPassword = err == nil
	return a, lsn, uniqueEmail(err)
}

// SetPassword gives an account a new password, as an administrator, and ends its sessions.
func (s *Service) SetPassword(ctx context.Context, id uuid.UUID, password string) (db.LSN, error) {
	if err := ValidatePassword(password); err != nil {
		return 0, err
	}
	hash, err := HashPassword(password, s.Params)
	if err != nil {
		return 0, err
	}
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		return affected(q.Exec(ctx, `UPDATE account SET password_hash = $2 WHERE id = $1`, id, hash))
	})
	if err != nil {
		return lsn, err
	}
	return lsn, s.Sessions.EndAll(ctx, id)
}

// SetDisabled switches an account off or back on, as an administrator; switching off ends its sessions.
func (s *Service) SetDisabled(ctx context.Context, id uuid.UUID, disabled bool) (db.LSN, error) {
	if actor, _ := db.UserFrom(ctx); actor == id && disabled {
		return 0, ErrSelf
	}
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		return affected(q.Exec(ctx, `UPDATE account SET disabled_at = CASE WHEN $2 THEN coalesce(disabled_at, now()) END
			WHERE id = $1`, id, disabled))
	})
	if err != nil || !disabled {
		return lsn, err
	}
	return lsn, s.Sessions.EndAll(ctx, id)
}

func affected(tag pgconn.CommandTag, err error) error {
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func uniqueEmail(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrEmailTaken
	}
	return err
}

// link is an address in the web client, for a mail or for an administrator to hand on.
func (s *Service) link(path, token string) string { return s.AppURL + path + token }

// Invite invites an address in with a role, as an administrator, replacing an open invitation to the same address.
// It returns the link, which the mail carries when the installation sends mail.
func (s *Service) Invite(ctx context.Context, email string, role Role) (Invitation, string, db.LSN, error) {
	if !role.Valid() {
		return Invitation{}, "", 0, ErrInvalidRole
	}
	token, hash, err := newToken()
	if err != nil {
		return Invitation{}, "", 0, err
	}
	actor, _ := db.UserFrom(ctx)
	var in Invitation
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		var exists bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM account WHERE email = $1)`, email).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return ErrEmailTaken
		}
		if _, err := q.Exec(ctx, `DELETE FROM invitation WHERE email = $1 AND accepted_at IS NULL`, email); err != nil {
			return err
		}
		return q.QueryRow(ctx, `INSERT INTO invitation (email, role, token_hash, invited_by, expires_at)
			VALUES ($1, $2, $3, $4, now() + $5::interval)
			RETURNING id, email, role, invited_by, created_at, expires_at`, email, role, hash, actor, s.InvitationTTL.String()).
			Scan(&in.ID, &in.Email, &in.Role, &in.InvitedBy, &in.CreatedAt, &in.ExpiresAt)
	})
	if err != nil {
		return in, "", lsn, err
	}
	link := s.link("/invitations/", token)
	if s.Mail != nil {
		name := s.installationName(ctx)
		_, err = s.Mail.Enqueue(ctx, MailJob, mailer.Mail{
			To:      email,
			Subject: fmt.Sprintf("You are invited to %s", name),
			Body: fmt.Sprintf("You are invited to %s on Bearing.\n\nOpen this link to choose your name and password:\n%s\n\n"+
				"The link works once, until %s.\n", name, link, in.ExpiresAt.UTC().Format("2 January 2006, 15:04 MST")),
		})
	}
	return in, link, lsn, err
}

func (s *Service) installationName(ctx context.Context) string {
	in, err := s.Installation(ctx)
	if err != nil || in.Name == "" {
		return "Bearing"
	}
	return in.Name
}

// Invitations lists the open invitations, as an administrator.
func (s *Service) Invitations(ctx context.Context) ([]Invitation, error) {
	var out []Invitation
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		rows, err := q.Query(ctx, `SELECT id, email, role, invited_by, created_at, expires_at FROM invitation
			WHERE accepted_at IS NULL ORDER BY created_at DESC`)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (Invitation, error) {
			var in Invitation
			return in, r.Scan(&in.ID, &in.Email, &in.Role, &in.InvitedBy, &in.CreatedAt, &in.ExpiresAt)
		})
		return err
	})
	return out, err
}

// RevokeInvitation takes back an open invitation, as an administrator.
func (s *Service) RevokeInvitation(ctx context.Context, id uuid.UUID) (db.LSN, error) {
	return s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		return affected(q.Exec(ctx, `DELETE FROM invitation WHERE id = $1 AND accepted_at IS NULL`, id))
	})
}

// InvitationFor is the address and role an invitation link brings in, for the page that accepts it.
func (s *Service) InvitationFor(ctx context.Context, token string) (Invitation, error) {
	var in Invitation
	err := s.DB.ReadAdmin(ctx, func(ctx context.Context, q db.DBTX) error {
		return q.QueryRow(ctx, `SELECT id, email, role, invited_by, created_at, expires_at FROM invitation
			WHERE token_hash = $1 AND accepted_at IS NULL AND expires_at > now()`, hashToken(token)).
			Scan(&in.ID, &in.Email, &in.Role, &in.InvitedBy, &in.CreatedAt, &in.ExpiresAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return in, ErrLinkInvalid
	}
	return in, err
}

// AcceptInvitation makes the invited account with the name and password its person chose, and signs them in.
func (s *Service) AcceptInvitation(ctx context.Context, token, name, password string) (Account, string, error) {
	if err := ValidatePassword(password); err != nil {
		return Account{}, "", err
	}
	hash, err := HashPassword(password, s.Params)
	if err != nil {
		return Account{}, "", err
	}
	var a Account
	_, err = s.DB.WriteAdmin(ctx, func(ctx context.Context, q db.DBTX) error {
		var (
			id    uuid.UUID
			email string
			role  Role
		)
		err := q.QueryRow(ctx, `UPDATE invitation SET accepted_at = now()
			WHERE token_hash = $1 AND accepted_at IS NULL AND expires_at > now() RETURNING id, email, role`, hashToken(token)).
			Scan(&id, &email, &role)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrLinkInvalid
		}
		if err != nil {
			return err
		}
		return uniqueEmail(scanAccount(q.QueryRow(ctx, `INSERT INTO account (email, name, password_hash, role)
			VALUES ($1, $2, $3, $4) RETURNING `+accountColumns, email, strings.TrimSpace(name), hash, role), &a))
	})
	if err != nil {
		return Account{}, "", err
	}
	a.HasPassword = true
	session, err := s.Sessions.Start(ctx, a.ID)
	return a, session, err
}

// RequestPasswordReset mails a reset link to an address with a password account. It says nothing about whether
// there is one, so the answer is the same for every address.
func (s *Service) RequestPasswordReset(ctx context.Context, email string) error {
	if s.Mail == nil {
		return ErrNoPasswordToMail
	}
	token, hash, err := newToken()
	if err != nil {
		return err
	}
	var found bool
	_, err = s.DB.WriteAdmin(ctx, func(ctx context.Context, q db.DBTX) error {
		tag, err := q.Exec(ctx, `INSERT INTO password_reset (token_hash, account_id, expires_at)
			SELECT $2, id, now() + $3::interval FROM account
			WHERE email = $1 AND password_hash IS NOT NULL AND disabled_at IS NULL`, strings.TrimSpace(email), hash, s.ResetTTL.String())
		found = err == nil && tag.RowsAffected() > 0
		return err
	})
	if err != nil || !found {
		return err
	}
	_, err = s.Mail.Enqueue(ctx, MailJob, mailer.Mail{
		To:      strings.TrimSpace(email),
		Subject: fmt.Sprintf("A new password for %s", s.installationName(ctx)),
		Body: fmt.Sprintf("Somebody asked for a new password for this address on Bearing.\n\nOpen this link to choose one:\n%s\n\n"+
			"The link works once, for %s. If you did not ask, ignore this mail; your password stays as it is.\n",
			s.link("/reset-password/", token), s.ResetTTL),
	})
	return err
}

// ResetPassword sets the password a reset link was sent for, and ends every session of the account.
func (s *Service) ResetPassword(ctx context.Context, token, password string) error {
	if err := ValidatePassword(password); err != nil {
		return err
	}
	hash, err := HashPassword(password, s.Params)
	if err != nil {
		return err
	}
	var id uuid.UUID
	_, err = s.DB.WriteAdmin(ctx, func(ctx context.Context, q db.DBTX) error {
		err := q.QueryRow(ctx, `UPDATE password_reset SET used_at = now()
			WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now() RETURNING account_id`, hashToken(token)).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrLinkInvalid
		}
		if err != nil {
			return err
		}
		_, err = q.Exec(ctx, `UPDATE account SET password_hash = $2 WHERE id = $1`, id, hash)
		return err
	})
	if err != nil {
		return err
	}
	return s.Sessions.EndAll(ctx, id)
}
