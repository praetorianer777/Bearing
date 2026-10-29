package org

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

// PersonKind says how a person is known: through the realm, by an account here, or neither.
type PersonKind string

const (
	Realm    PersonKind = "realm"
	Local    PersonKind = "local"
	External PersonKind = "external"
)

// Person is somebody in the organisation.
type Person struct {
	ID        uuid.UUID
	Name      string
	Email     *string
	Phone     *string
	Location  *string
	AccountID *uuid.UUID
	Issuer    *string
	Subject   *string
	LeftOn    *Day
	HasPhoto  bool
}

// Kind is how the person is known.
func (p Person) Kind() PersonKind {
	switch {
	case p.Subject != nil:
		return Realm
	case p.AccountID != nil:
		return Local
	}
	return External
}

// PersonInput is what a person is made or changed with; a nil field is left as it is, and an empty string clears
// an optional one.
type PersonInput struct {
	Name     *string
	Email    *string
	Phone    *string
	Location *string
}

// Fields are what a person's input sets, by the name the API gives them.
func (in PersonInput) Fields() []string {
	var out []string
	for name, set := range map[string]bool{"name": in.Name != nil, "email": in.Email != nil, "phone": in.Phone != nil, "location": in.Location != nil} {
		if set {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// SelfEditable are the fields of their own entry somebody may change, when administrators allow them.
var SelfEditable = []string{"name", "email", "phone", "location", "photo"}

// MaxPhotoBytes bounds a photo.
const MaxPhotoBytes = 1 << 20

var (
	ErrPersonNotFound = errors.New("there is no such person")
	ErrNoEntry        = errors.New("no entry in the organisation is linked to your account; ask an editor")
	ErrEmailTaken     = errors.New("another person has that email address")
	ErrAccountLinked  = errors.New("that account is linked to another person already")
	ErrAccountMissing = errors.New("there is no such account")
	ErrLeft           = errors.New("the person has left already")
	ErrNotLeft        = errors.New("the person has not left")
	ErrNotYours       = errors.New("that field of your entry is kept by editors")
	ErrBadPhoto       = errors.New("a photo is a PNG, JPEG, WebP or GIF picture of at most 1 MB")
)

func (in *PersonInput) check(creating bool) error {
	if creating && in.Name == nil {
		return Invalid{"name", "Give a name."}
	}
	if err := checkText("name", in.Name, 1, 200); err != nil {
		return err
	}
	if in.Email != nil {
		*in.Email = strings.TrimSpace(*in.Email)
		if *in.Email != "" {
			if a, err := mail.ParseAddress(*in.Email); err != nil || a.Address != *in.Email || len(*in.Email) > 254 {
				return Invalid{"email", "Give an email address."}
			}
		}
	}
	if err := checkText("phone", in.Phone, 0, 50); err != nil {
		return err
	}
	return checkText("location", in.Location, 0, 200)
}

const personColumns = `p.id, p.name, p.email, p.phone, p.location, p.account_id, p.issuer, p.subject, p.left_on,
	EXISTS (SELECT 1 FROM person_photo f WHERE f.person_id = p.id)`

func scanPerson(row pgx.CollectableRow) (Person, error) {
	var p Person
	var left *time.Time
	err := row.Scan(&p.ID, &p.Name, &p.Email, &p.Phone, &p.Location, &p.AccountID, &p.Issuer, &p.Subject, &left, &p.HasPhoto)
	p.LeftOn = dayPtr(left)
	return p, err
}

// People lists who is in the organisation on a day, or with left, who has left by then; query narrows to names and
// addresses holding it.
func (s *Service) People(ctx context.Context, asOf Day, left bool, query string) ([]Person, error) {
	var out []Person
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		rows, err := q.Query(ctx, `SELECT `+personColumns+` FROM person p
			WHERE (CASE WHEN $2 THEN p.left_on <= $1 ELSE p.left_on IS NULL OR p.left_on > $1 END)
			  AND ($3 = '' OR p.name ILIKE '%' || $3 || '%' OR p.email ILIKE '%' || $3 || '%')
			ORDER BY lower(p.name), p.id`, asOf.Time(), left, strings.TrimSpace(query))
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, scanPerson)
		return err
	})
	return out, err
}

// Person is one person.
func (s *Service) Person(ctx context.Context, id uuid.UUID) (Person, error) {
	var out Person
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		return s.person(ctx, q, `p.id = $1`, id, &out)
	})
	return out, err
}

// Me is the person linked to the account the context acts for.
func (s *Service) Me(ctx context.Context) (Person, error) {
	actor, ok := db.UserFrom(ctx)
	if !ok {
		return Person{}, ErrNoEntry
	}
	var out Person
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		return s.person(ctx, q, `p.account_id = $1`, actor, &out)
	})
	if errors.Is(err, ErrPersonNotFound) {
		return Person{}, ErrNoEntry
	}
	return out, err
}

func (s *Service) person(ctx context.Context, q db.DBTX, where string, arg any, out *Person) error {
	rows, err := q.Query(ctx, `SELECT `+personColumns+` FROM person p WHERE `+where, arg)
	if err != nil {
		return err
	}
	*out, err = pgx.CollectExactlyOneRow(rows, scanPerson)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPersonNotFound
	}
	return err
}

// CreatePerson adds somebody to the organisation, external until an account or the realm links them, as an editor.
func (s *Service) CreatePerson(ctx context.Context, in PersonInput) (Person, db.LSN, error) {
	if err := in.check(true); err != nil {
		return Person{}, 0, err
	}
	var out Person
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		var id uuid.UUID
		if err := q.QueryRow(ctx, `INSERT INTO person (name, email, phone, location)
			VALUES ($1, nullif($2, ''), nullif($3, ''), nullif($4, '')) RETURNING id`,
			in.Name, in.Email, in.Phone, in.Location).Scan(&id); err != nil {
			return personError(err)
		}
		return s.person(ctx, q, `p.id = $1`, id, &out)
	})
	return out, lsn, err
}

// UpdatePerson changes a person's fields, as an editor.
func (s *Service) UpdatePerson(ctx context.Context, id uuid.UUID, in PersonInput) (Person, db.LSN, error) {
	if err := in.check(false); err != nil {
		return Person{}, 0, err
	}
	return s.update(ctx, `id = $1`, id, in, ErrPersonNotFound)
}

// UpdateMe changes those fields of their own entry that administrators allow, as the person the context acts for.
func (s *Service) UpdateMe(ctx context.Context, in PersonInput) (Person, db.LSN, error) {
	actor, ok := db.UserFrom(ctx)
	if !ok {
		return Person{}, 0, ErrNoEntry
	}
	if err := in.check(false); err != nil {
		return Person{}, 0, err
	}
	return s.update(ctx, `account_id = $1`, actor, in, ErrNoEntry)
}

func (s *Service) update(ctx context.Context, where string, arg any, in PersonInput, missing error) (Person, db.LSN, error) {
	var out Person
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		var id uuid.UUID
		err := q.QueryRow(ctx, `UPDATE person SET name = coalesce($2, name),
			email = CASE WHEN $3::text IS NULL THEN email ELSE nullif($3, '') END,
			phone = CASE WHEN $4::text IS NULL THEN phone ELSE nullif($4, '') END,
			location = CASE WHEN $5::text IS NULL THEN location ELSE nullif($5, '') END
			WHERE `+where+` RETURNING id`, arg, in.Name, in.Email, in.Phone, in.Location).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return missing
		}
		if err != nil {
			return personError(err)
		}
		return s.person(ctx, q, `p.id = $1`, id, &out)
	})
	return out, lsn, err
}

// LinkAccount links a person to an account here, or unlinks them with nil, as an administrator: the account's holder
// then keeps the fields of the entry administrators allow.
func (s *Service) LinkAccount(ctx context.Context, id uuid.UUID, account *uuid.UUID) (Person, db.LSN, error) {
	var out Person
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		tag, err := q.Exec(ctx, `UPDATE person SET account_id = $2 WHERE id = $1`, id, account)
		if err != nil {
			return personError(err)
		}
		if tag.RowsAffected() == 0 {
			return ErrPersonNotFound
		}
		return s.person(ctx, q, `p.id = $1`, id, &out)
	})
	return out, lsn, err
}

// Leave marks a person as gone from a day, as an editor. The days before keep them.
func (s *Service) Leave(ctx context.Context, id uuid.UUID, on Day) (Person, db.LSN, error) {
	return s.setLeft(ctx, id, &on)
}

// Return takes back that a person left, as an editor.
func (s *Service) Return(ctx context.Context, id uuid.UUID) (Person, db.LSN, error) {
	return s.setLeft(ctx, id, nil)
}

func (s *Service) setLeft(ctx context.Context, id uuid.UUID, on *Day) (Person, db.LSN, error) {
	var out Person
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		var left *time.Time
		err := q.QueryRow(ctx, `SELECT left_on FROM person WHERE id = $1 FOR UPDATE`, id).Scan(&left)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPersonNotFound
		}
		if err != nil {
			return err
		}
		var to *time.Time
		switch {
		case on != nil && left != nil:
			return ErrLeft
		case on == nil && left == nil:
			return ErrNotLeft
		case on != nil:
			t := on.Time()
			to = &t
		}
		if _, err := q.Exec(ctx, `UPDATE person SET left_on = $2 WHERE id = $1`, id, to); err != nil {
			return err
		}
		return s.person(ctx, q, `p.id = $1`, id, &out)
	})
	return out, lsn, err
}

// Photo is a person's photo and what kind of picture it is.
func (s *Service) Photo(ctx context.Context, id uuid.UUID) (contentType string, data []byte, err error) {
	err = s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		err := q.QueryRow(ctx, `SELECT content_type, data FROM person_photo WHERE person_id = $1`, id).Scan(&contentType, &data)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPersonNotFound
		}
		return err
	})
	return contentType, data, err
}

// photoType is what kind of picture data is, from its bytes.
func photoType(data []byte) (string, error) {
	if len(data) == 0 || len(data) > MaxPhotoBytes {
		return "", ErrBadPhoto
	}
	switch kind := http.DetectContentType(data); kind {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
		return kind, nil
	}
	return "", ErrBadPhoto
}

// SetPhoto replaces a person's photo, or the photo of the person the context acts for with a nil id; nil data takes
// it away. Editors set anybody's; somebody signed in sets their own when administrators allow it.
func (s *Service) SetPhoto(ctx context.Context, id *uuid.UUID, data []byte) (db.LSN, error) {
	var kind string
	if data != nil {
		var err error
		if kind, err = photoType(data); err != nil {
			return 0, err
		}
	}
	return s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		var person uuid.UUID
		var err error
		if id != nil {
			err = q.QueryRow(ctx, `SELECT id FROM person WHERE id = $1`, *id).Scan(&person)
		} else {
			actor, _ := db.UserFrom(ctx)
			err = q.QueryRow(ctx, `SELECT id FROM person WHERE account_id = $1`, actor).Scan(&person)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			if id == nil {
				return ErrNoEntry
			}
			return ErrPersonNotFound
		}
		if err != nil {
			return err
		}
		if data == nil {
			_, err = q.Exec(ctx, `DELETE FROM person_photo WHERE person_id = $1`, person)
		} else {
			_, err = q.Exec(ctx, `INSERT INTO person_photo (person_id, content_type, data) VALUES ($1, $2, $3)
				ON CONFLICT (person_id) DO UPDATE SET content_type = excluded.content_type, data = excluded.data, updated_at = now()`,
				person, kind, data)
		}
		return personError(err)
	})
}

// SelfEditableFields are the fields of their own entry administrators let people change.
func (s *Service) SelfEditableFields(ctx context.Context) ([]string, error) {
	var out []string
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		err := q.QueryRow(ctx, `SELECT self_editable FROM installation`).Scan(&out)
		if errors.Is(err, pgx.ErrNoRows) {
			out = []string{}
			return nil
		}
		return err
	})
	return out, err
}

// SetSelfEditableFields chooses which fields of their own entry people may change, as an administrator.
func (s *Service) SetSelfEditableFields(ctx context.Context, fields []string) (db.LSN, error) {
	for _, f := range fields {
		if !slices.Contains(SelfEditable, f) {
			return 0, Invalid{"fields", "Choose among name, email, phone, location and photo."}
		}
	}
	slices.Sort(fields)
	fields = slices.Compact(fields)
	if fields == nil {
		fields = []string{}
	}
	return s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		_, err := q.Exec(ctx, `UPDATE installation SET self_editable = $1`, fields)
		return err
	})
}

// LinkPerson links an account that signs in to its entry in the organisation: the entry the realm knows by one of
// the account's identities, else an unlinked entry with the account's address, which then takes the identity too.
// It runs before anybody acts, as bearing_admin.
func (s *Service) LinkPerson(ctx context.Context, accountID uuid.UUID) (db.LSN, error) {
	return s.DB.WriteAdmin(ctx, func(ctx context.Context, q db.DBTX) error {
		var linked bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM person WHERE account_id = $1)`, accountID).Scan(&linked); err != nil || linked {
			return err
		}
		tag, err := q.Exec(ctx, `UPDATE person p SET account_id = $1 FROM account_identity i
			WHERE i.account_id = $1 AND p.issuer = i.issuer AND p.subject = i.subject AND p.account_id IS NULL`, accountID)
		if err != nil || tag.RowsAffected() > 0 {
			return err
		}
		_, err = q.Exec(ctx, `UPDATE person p SET account_id = a.id,
				issuer = (SELECT issuer FROM account_identity WHERE account_id = a.id ORDER BY last_sign_in DESC LIMIT 1),
				subject = (SELECT subject FROM account_identity WHERE account_id = a.id ORDER BY last_sign_in DESC LIMIT 1)
			FROM account a WHERE a.id = $1 AND p.email = a.email AND p.account_id IS NULL AND p.subject IS NULL`, accountID)
		return err
	})
}

func personError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch {
	case pgErr.ConstraintName == "person_email":
		return ErrEmailTaken
	case pgErr.ConstraintName == "person_account_id_key":
		return ErrAccountLinked
	case pgErr.ConstraintName == "person_account_id_fkey":
		return ErrAccountMissing
	case pgErr.Code == "42501":
		return ErrNotYours
	}
	return err
}
