package theme

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

// PackageFormat names the shape of an exported theme, so a file from a later version can be told apart from one
// this version reads.
const PackageFormat = "armature-theme/1"

// MaxPackageBytes bounds an import: every file at its limit, base64 inflating each by a third, and the spec.
const MaxPackageBytes = MaxAssetsPerSpec*MaxAssetBytes*4/3 + MaxSpecBytes

// MaxNameRunes is how long a theme's name may be.
const MaxNameRunes = 100

// Package is a theme as one file: its name, its spec and every file it draws from.
type Package struct {
	Format string          `json:"format"`
	Name   string          `json:"name"`
	Spec   Spec            `json:"spec"`
	Assets []PackagedAsset `json:"assets"`
}

// PackagedAsset is one file of a theme, its bytes carried inline.
type PackagedAsset struct {
	// ID is the id the spec and the CSS refer to it by; the import gives it a new one.
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	ContentType string    `json:"contentType"`
	Data        []byte    `json:"data"`
}

// Theme is one theme as the reader sees it.
type Theme struct {
	ID        uuid.UUID
	OwnerID   uuid.UUID
	OwnerName string
	Name      string
	Shared    bool
	// Default says the installation shows it to whoever has not chosen; Active says the reader chose it.
	Default bool
	Active  bool
	// InUse counts the people who chose it.
	InUse     int
	Spec      Spec
	Assets    []Asset
	UpdatedAt time.Time
}

// Asset is a file a theme draws from; Data is filled only when the file itself is asked for.
type Asset struct {
	ID          uuid.UUID
	Name        string
	ContentType string
	Size        int
	Data        []byte
}

// Input is a theme as the editor sends it; nil leaves a field alone on an edit.
type Input struct {
	Name   *string `json:"name,omitempty"`
	Shared *bool   `json:"shared,omitempty"`
	Spec   *Spec   `json:"spec,omitempty"`
}

// Source says where the theme somebody sees comes from.
type Source string

const (
	// SourceChosen is a theme the person picked for themselves.
	SourceChosen Source = "chosen"
	// SourceEveryone is the installation's default, shown to whoever has not chosen.
	SourceEveryone Source = "everyone"
	// SourceBuiltIn is Bearing's own look: nothing chosen and no default, or the built-in look chosen over it.
	SourceBuiltIn Source = ""
)

var (
	// ErrNotAThemeFile is returned for a file that is not an exported theme.
	ErrNotAThemeFile = errors.New("that is not a theme file: export one from Armature, Exciter, Stator or Bearing to see the shape")
	// ErrNotFound is a theme or a file that is not there, or not in the reader's sight.
	ErrNotFound = errors.New("there is no such theme")
	// ErrDuplicateName is a name another of the owner's themes has.
	ErrDuplicateName = errors.New("you already have a theme with that name")
	// ErrNotYours is a change to a theme the actor may see but not change.
	ErrNotYours = errors.New("only the owner, or an administrator once it is shared, changes a theme")
	// ErrDefaultNotShared is a private theme named as everybody's.
	ErrDefaultNotShared = errors.New("the theme everybody sees has to be a shared one")
	// ErrNoName is a theme without a name, or with too long a one.
	ErrNoName = errors.New("a theme needs a name of at most 100 characters")
)

// Invalid is a theme whose spec or files this installation will not take, saying why.
type Invalid struct{ Err error }

func (e Invalid) Error() string { return e.Err.Error() }
func (e Invalid) Unwrap() error { return e.Err }

// Service keeps themes, their files and who chose which. Row level security says who sees and changes what; the
// service asks again only to tell "not there" from "not yours".
type Service struct {
	DB *db.Cluster
}

// selectThemes reads themes with the reader's marks. The count comes from theme_in_use, since a person sees only
// their own choice.
const selectThemes = `
SELECT t.id, t.owner_id, coalesce(a.name, ''), t.name, t.shared, t.is_default, theme_in_use(t.id),
       EXISTS (SELECT 1 FROM user_theme ut WHERE ut.theme_id = t.id AND ut.account_id = current_actor_id()),
       t.spec, t.updated_at
FROM theme t
LEFT JOIN account a ON a.id = t.owner_id`

func scanTheme(row pgx.CollectableRow) (Theme, error) {
	var t Theme
	var spec []byte
	if err := row.Scan(&t.ID, &t.OwnerID, &t.OwnerName, &t.Name, &t.Shared, &t.Default, &t.InUse, &t.Active, &spec, &t.UpdatedAt); err != nil {
		return t, err
	}
	t.Assets = []Asset{}
	if err := json.Unmarshal(spec, &t.Spec); err != nil {
		return t, fmt.Errorf("theme %s: %w", t.ID, err)
	}
	t.Spec.normalise()
	return t, nil
}

// readThemes reads the themes a condition picks, their files named but not read.
func readThemes(ctx context.Context, q db.DBTX, where string, args ...any) ([]Theme, error) {
	rows, err := q.Query(ctx, selectThemes+" "+where, args...)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, scanTheme)
	if err != nil || len(out) == 0 {
		return out, err
	}
	ids := make([]uuid.UUID, len(out))
	byID := map[uuid.UUID]*Theme{}
	for i := range out {
		ids[i] = out[i].ID
		byID[out[i].ID] = &out[i]
	}
	rows, err = q.Query(ctx, `SELECT theme_id, id, name, content_type, octet_length(data) FROM theme_asset WHERE theme_id = ANY($1) ORDER BY theme_id, name, id`, ids)
	if err != nil {
		return nil, err
	}
	var themeID uuid.UUID
	var a Asset
	_, err = pgx.ForEachRow(rows, []any{&themeID, &a.ID, &a.Name, &a.ContentType, &a.Size}, func() error {
		byID[themeID].Assets = append(byID[themeID].Assets, a)
		return nil
	})
	return out, err
}

func readOne(ctx context.Context, q db.DBTX, id uuid.UUID) (Theme, error) {
	got, err := readThemes(ctx, q, `WHERE t.id = $1`, id)
	if err != nil {
		return Theme{}, err
	}
	if len(got) == 0 {
		return Theme{}, ErrNotFound
	}
	return got[0], nil
}

// List is every theme the reader may see: theirs first, then the shared ones.
func (s *Service) List(ctx context.Context) ([]Theme, error) {
	var out []Theme
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		var err error
		out, err = readThemes(ctx, q, `ORDER BY t.owner_id <> current_actor_id(), lower(t.name), t.id`)
		return err
	})
	return out, err
}

// Get is one theme the reader may see.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Theme, error) {
	var out Theme
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		var err error
		out, err = readOne(ctx, q, id)
		return err
	})
	return out, err
}

// Active is the reader's chosen theme, else the installation's default, else nil for the built-in look. A choice of
// a theme since taken private lapses quietly to the built-in look.
func (s *Service) Active(ctx context.Context) (*Theme, Source, error) {
	var out *Theme
	var source Source
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		var chosen *uuid.UUID
		err := q.QueryRow(ctx, `SELECT theme_id FROM user_theme WHERE account_id = current_actor_id()`).Scan(&chosen)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			got, err := readThemes(ctx, q, `WHERE t.is_default`)
			if err != nil || len(got) == 0 {
				return err
			}
			out, source = &got[0], SourceEveryone
			return nil
		case err != nil || chosen == nil:
			return err
		}
		got, err := readOne(ctx, q, *chosen)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		out, source = &got, SourceChosen
		return err
	})
	return out, source, err
}

// Default is the installation's default theme, nil for the built-in look, whatever the reader chose.
func (s *Service) Default(ctx context.Context) (*Theme, error) {
	var out *Theme
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		got, err := readThemes(ctx, q, `WHERE t.is_default`)
		if err == nil && len(got) > 0 {
			out = &got[0]
		}
		return err
	})
	return out, err
}

// Choose makes a theme the reader's. Nil returns them to the installation's default; nil with builtIn keeps the
// built-in look over it.
func (s *Service) Choose(ctx context.Context, id *uuid.UUID, builtIn bool) (*Theme, db.LSN, error) {
	var out *Theme
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		if id == nil && !builtIn {
			_, err := q.Exec(ctx, `DELETE FROM user_theme WHERE account_id = current_actor_id()`)
			return err
		}
		if id != nil {
			if _, err := readOne(ctx, q, *id); err != nil {
				return err
			}
		}
		if _, err := q.Exec(ctx, `
			INSERT INTO user_theme (account_id, theme_id) VALUES (current_actor_id(), $1)
			ON CONFLICT (account_id) DO UPDATE SET theme_id = EXCLUDED.theme_id`, id); err != nil || id == nil {
			return err
		}
		got, err := readOne(ctx, q, *id)
		out = &got
		return err
	})
	if err != nil {
		return nil, 0, err
	}
	return out, lsn, nil
}

// SetDefault names the shared theme everybody sees until they choose, as an administrator; nil is the built-in look.
func (s *Service) SetDefault(ctx context.Context, id *uuid.UUID) (db.LSN, error) {
	return s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		if id != nil {
			got, err := readOne(ctx, q, *id)
			if err != nil {
				return err
			}
			if !got.Shared {
				return ErrDefaultNotShared
			}
		}
		if _, err := q.Exec(ctx, `UPDATE theme SET is_default = false WHERE is_default`); err != nil || id == nil {
			return err
		}
		_, err := q.Exec(ctx, `UPDATE theme SET is_default = true WHERE id = $1`, *id)
		return err
	})
}

func cleanName(name *string) (string, error) {
	if name == nil {
		return "", ErrNoName
	}
	trimmed := strings.TrimSpace(*name)
	if trimmed == "" || len([]rune(trimmed)) > MaxNameRunes {
		return "", ErrNoName
	}
	return trimmed, nil
}

// Create makes a theme of the actor's own. Files come once it exists, so the spec cannot name any yet.
func (s *Service) Create(ctx context.Context, in Input) (Theme, db.LSN, error) {
	name, err := cleanName(in.Name)
	if err != nil {
		return Theme{}, 0, err
	}
	spec := Spec{}
	if in.Spec != nil {
		spec = *in.Spec
	}
	if err := Validate(&spec, uuid.Nil, nil); err != nil {
		return Theme{}, 0, Invalid{err}
	}
	encoded, err := json.Marshal(spec)
	if err != nil {
		return Theme{}, 0, err
	}
	var out Theme
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		var id uuid.UUID
		if err := q.QueryRow(ctx, `INSERT INTO theme (owner_id, name, shared, spec) VALUES (current_actor_id(), $1, $2, $3) RETURNING id`,
			name, in.Shared != nil && *in.Shared, encoded).Scan(&id); err != nil {
			return duplicate(err)
		}
		out, err = readOne(ctx, q, id)
		return err
	})
	return out, lsn, err
}

// changeable reads a theme the actor may change: theirs, or shared and they an administrator.
func changeable(ctx context.Context, q db.DBTX, id uuid.UUID) (Theme, error) {
	current, err := readOne(ctx, q, id)
	if err != nil {
		return current, err
	}
	var may bool
	if err := q.QueryRow(ctx, `SELECT owner_id = current_actor_id() OR (shared AND current_actor_is_administrator()) FROM theme WHERE id = $1`,
		id).Scan(&may); err != nil {
		return current, err
	}
	if !may {
		return current, ErrNotYours
	}
	return current, nil
}

// Update changes a theme: its owner's to do, or an administrator's once shared.
func (s *Service) Update(ctx context.Context, id uuid.UUID, in Input) (Theme, db.LSN, error) {
	var out Theme
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		current, err := changeable(ctx, q, id)
		if err != nil {
			return err
		}
		name, shared, spec := current.Name, current.Shared, current.Spec
		if in.Name != nil {
			if name, err = cleanName(in.Name); err != nil {
				return err
			}
		}
		if in.Shared != nil {
			shared = *in.Shared
		}
		if in.Spec != nil {
			spec = *in.Spec
		}
		if err := Validate(&spec, id, assetSet(current)); err != nil {
			return Invalid{err}
		}
		encoded, err := json.Marshal(spec)
		if err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `UPDATE theme SET name = $2, shared = $3, spec = $4 WHERE id = $1`, id, name, shared, encoded); err != nil {
			return duplicate(err)
		}
		out, err = readOne(ctx, q, id)
		if errors.Is(err, ErrNotFound) {
			// An administrator who takes somebody's theme private no longer sees it.
			current.Name, current.Shared, current.Spec, current.Default = name, shared, spec, false
			out, err = current, nil
		}
		return err
	})
	return out, lsn, err
}

func assetSet(t Theme) map[uuid.UUID]bool {
	out := map[uuid.UUID]bool{}
	for _, a := range t.Assets {
		out[a.ID] = true
	}
	return out
}

// Delete removes a theme and its files. Whoever had chosen it returns to the installation's default.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) (db.LSN, error) {
	return s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		if _, err := changeable(ctx, q, id); err != nil {
			return err
		}
		_, err := q.Exec(ctx, `DELETE FROM theme WHERE id = $1`, id)
		return err
	})
}

// AddAsset puts a file on a theme. What it is comes from its bytes, never from what it claims.
func (s *Service) AddAsset(ctx context.Context, id uuid.UUID, name string, data []byte) (Asset, db.LSN, error) {
	if len(data) > MaxAssetBytes {
		return Asset{}, 0, Invalid{ErrAssetTooLarge}
	}
	kind, err := sniff(name, data)
	if err != nil {
		return Asset{}, 0, Invalid{err}
	}
	out := Asset{Name: cleanFileName(name), ContentType: kind, Size: len(data)}
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		current, err := changeable(ctx, q, id)
		if err != nil {
			return err
		}
		if len(current.Assets) >= MaxAssetsPerSpec {
			return Invalid{ErrTooManyAssets}
		}
		return q.QueryRow(ctx, `INSERT INTO theme_asset (theme_id, name, content_type, data) VALUES ($1, $2, $3, $4) RETURNING id`,
			id, out.Name, out.ContentType, data).Scan(&out.ID)
	})
	return out, lsn, err
}

// cleanFileName keeps a file's own name, short of any folders and at most 200 characters, or calls it "file".
func cleanFileName(name string) string {
	name = strings.TrimSpace(name[strings.LastIndexAny(name, `/\`)+1:])
	if runes := []rune(name); len(runes) > 200 {
		name = string(runes[:200])
	}
	if name == "" {
		return "file"
	}
	return name
}

// RemoveAsset takes a file off a theme, unless the theme still names it.
func (s *Service) RemoveAsset(ctx context.Context, id, assetID uuid.UUID) (db.LSN, error) {
	return s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		current, err := changeable(ctx, q, id)
		if err != nil {
			return err
		}
		if !assetSet(current)[assetID] {
			return ErrNotFound
		}
		for _, used := range current.Spec.AssetIDs() {
			if used == assetID {
				return ErrAssetInUse
			}
		}
		_, err = q.Exec(ctx, `DELETE FROM theme_asset WHERE theme_id = $1 AND id = $2`, id, assetID)
		return err
	})
}

// maxNameTries is how far an import numbers a taken name, "Name (2)" on, before it says the name is taken.
const maxNameTries = 50

var ownAssetURL = regexp.MustCompile(`/api/v1/themes/([0-9a-f-]{36})/assets/([0-9a-f-]{36})`)

// Import makes the package a theme of the actor's own. Its files get new ids, which the spec and the CSS are
// rewritten to name; what each file is comes from its bytes. A name the actor already has gets a number.
func (s *Service) Import(ctx context.Context, pkg *Package) (Theme, db.LSN, error) {
	if pkg == nil || pkg.Format != PackageFormat {
		return Theme{}, 0, ErrNotAThemeFile
	}
	name := strings.TrimSpace(pkg.Name)
	if name == "" || len([]rune(name)) > MaxNameRunes {
		return Theme{}, 0, ErrNotAThemeFile
	}
	if len(pkg.Assets) > MaxAssetsPerSpec {
		return Theme{}, 0, Invalid{ErrTooManyAssets}
	}
	types := make([]string, len(pkg.Assets))
	for i, a := range pkg.Assets {
		kind, err := sniff(a.Name, a.Data)
		if err != nil {
			return Theme{}, 0, Invalid{fmt.Errorf("%s: %w", a.Name, err)}
		}
		types[i] = kind
	}

	var made Theme
	lsn, err := s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		rows, err := q.Query(ctx, `SELECT lower(name) FROM theme WHERE owner_id = current_actor_id()`)
		if err != nil {
			return err
		}
		names, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		taken := map[string]bool{}
		for _, n := range names {
			taken[n] = true
		}
		for n := 1; n <= maxNameTries && made.Name == ""; n++ {
			attempt := name
			if n > 1 {
				attempt = fmt.Sprintf("%s (%d)", name, n)
			}
			if !taken[strings.ToLower(attempt)] {
				made.Name = attempt
			}
		}
		if made.Name == "" {
			return ErrDuplicateName
		}
		if err := q.QueryRow(ctx, `INSERT INTO theme (owner_id, name, spec) VALUES (current_actor_id(), $1, '{}') RETURNING id`,
			made.Name).Scan(&made.ID); err != nil {
			return duplicate(err)
		}

		renamed := map[uuid.UUID]uuid.UUID{}
		own := map[uuid.UUID]bool{}
		for i, a := range pkg.Assets {
			var id uuid.UUID
			if err := q.QueryRow(ctx, `INSERT INTO theme_asset (theme_id, name, content_type, data) VALUES ($1, $2, $3, $4) RETURNING id`,
				made.ID, cleanFileName(a.Name), types[i], a.Data).Scan(&id); err != nil {
				return err
			}
			renamed[a.ID] = id
			own[id] = true
		}

		spec := pkg.Spec
		if err := rename(&spec, made.ID, renamed); err != nil {
			return err
		}
		if err := Validate(&spec, made.ID, own); err != nil {
			return Invalid{err}
		}
		encoded, err := json.Marshal(spec)
		if err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `UPDATE theme SET spec = $2 WHERE id = $1`, made.ID, encoded); err != nil {
			return err
		}
		made, err = readOne(ctx, q, made.ID)
		return err
	})
	if err != nil {
		return Theme{}, 0, err
	}
	return made, lsn, nil
}

// rename points the spec at the files' new ids. A spec naming a file the package does not carry is not a theme file.
func rename(spec *Spec, themeID uuid.UUID, renamed map[uuid.UUID]uuid.UUID) error {
	spec.normalise()
	swap := func(id *uuid.UUID) error {
		if id == nil {
			return nil
		}
		next, ok := renamed[*id]
		if !ok {
			return Invalid{fmt.Errorf("%w: the theme names a file it does not carry", ErrNotAThemeFile)}
		}
		*id = next
		return nil
	}
	for _, font := range []*Font{spec.Fonts.Sans, spec.Fonts.Mono} {
		if font != nil {
			if err := swap(font.AssetID); err != nil {
				return err
			}
		}
	}
	for kind, cursor := range spec.Cursors {
		if err := swap(&cursor.AssetID); err != nil {
			return err
		}
		spec.Cursors[kind] = cursor
	}
	for name, icon := range spec.Icons {
		if err := swap(icon.AssetID); err != nil {
			return err
		}
		spec.Icons[name] = icon
	}
	if spec.Backdrop != nil {
		if err := swap(&spec.Backdrop.AssetID); err != nil {
			return err
		}
	}
	spec.CSS = ownAssetURL.ReplaceAllStringFunc(spec.CSS, func(match string) string {
		parts := ownAssetURL.FindStringSubmatch(match)
		old, err := uuid.Parse(parts[2])
		if err != nil {
			return match
		}
		next, ok := renamed[old]
		if !ok {
			return match
		}
		return "/api/v1/themes/" + themeID.String() + "/assets/" + next.String()
	})
	return nil
}

// Export packs a theme the reader may see, files included.
func (s *Service) Export(ctx context.Context, id uuid.UUID) (*Package, error) {
	var out *Package
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		t, err := readOne(ctx, q, id)
		if err != nil {
			return err
		}
		out = &Package{Format: PackageFormat, Name: t.Name, Spec: t.Spec, Assets: []PackagedAsset{}}
		rows, err := q.Query(ctx, `SELECT id, name, content_type, data FROM theme_asset WHERE theme_id = $1 ORDER BY name, id`, id)
		if err != nil {
			return err
		}
		out.Assets, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (PackagedAsset, error) {
			var a PackagedAsset
			return a, r.Scan(&a.ID, &a.Name, &a.ContentType, &a.Data)
		})
		return err
	})
	return out, err
}

// File reads one of the files of a theme the reader may see.
func (s *Service) File(ctx context.Context, themeID, assetID uuid.UUID) (Asset, error) {
	var a Asset
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		err := q.QueryRow(ctx, `SELECT id, name, content_type, data FROM theme_asset WHERE theme_id = $1 AND id = $2`, themeID, assetID).
			Scan(&a.ID, &a.Name, &a.ContentType, &a.Data)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	})
	a.Size = len(a.Data)
	return a, err
}

func duplicate(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrDuplicateName
	}
	return err
}
