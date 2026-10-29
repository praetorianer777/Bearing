package theme

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

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

// Theme is one of the installation's themes.
type Theme struct {
	ID      uuid.UUID
	Name    string
	Default bool
	Spec    Spec
	Assets  []Asset
}

// Asset is a file a theme draws from; Data is filled only when the file itself is asked for.
type Asset struct {
	ID          uuid.UUID
	Name        string
	ContentType string
	Size        int
	Data        []byte
}

var (
	// ErrNotAThemeFile is returned for a file that is not an exported theme.
	ErrNotAThemeFile = errors.New("that is not a theme file: export one from Armature, Exciter, Stator or Bearing to see the shape")
	// ErrNotFound is a theme or a file that is not there.
	ErrNotFound = errors.New("there is no such theme")
	// ErrDuplicateName is a name another theme has.
	ErrDuplicateName = errors.New("another theme has that name")
)

// Invalid is a theme whose spec or files this installation will not take, saying why.
type Invalid struct{ Err error }

func (e Invalid) Error() string { return e.Err.Error() }
func (e Invalid) Unwrap() error { return e.Err }

// Service keeps the installation's themes.
type Service struct {
	DB *db.Cluster
}

// List is every theme, the files named but not read.
func (s *Service) List(ctx context.Context) ([]Theme, error) {
	var out []Theme
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		rows, err := q.Query(ctx, `SELECT id, name, is_default, spec FROM theme ORDER BY lower(name)`)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, scanTheme)
		if err != nil {
			return err
		}
		byID := map[uuid.UUID]*Theme{}
		for i := range out {
			byID[out[i].ID] = &out[i]
		}
		rows, err = q.Query(ctx, `SELECT theme_id, id, name, content_type, octet_length(data) FROM theme_asset ORDER BY theme_id, name`)
		if err != nil {
			return err
		}
		var themeID uuid.UUID
		var a Asset
		_, err = pgx.ForEachRow(rows, []any{&themeID, &a.ID, &a.Name, &a.ContentType, &a.Size}, func() error {
			if t := byID[themeID]; t != nil {
				t.Assets = append(t.Assets, a)
			}
			return nil
		})
		return err
	})
	return out, err
}

func scanTheme(row pgx.CollectableRow) (Theme, error) {
	var t Theme
	var spec []byte
	if err := row.Scan(&t.ID, &t.Name, &t.Default, &spec); err != nil {
		return t, err
	}
	t.Assets = []Asset{}
	if err := json.Unmarshal(spec, &t.Spec); err != nil {
		return t, fmt.Errorf("theme %s: %w", t.ID, err)
	}
	t.Spec.normalise()
	return t, nil
}

// maxNameTries is how far an import numbers a taken name, "Name (2)" on, before it says the name is taken.
const maxNameTries = 50

var ownAssetURL = regexp.MustCompile(`/api/v1/themes/([0-9a-f-]{36})/assets/([0-9a-f-]{36})`)

// Import makes the package one of the installation's themes, as an administrator. Its files get new ids, which the
// spec and the CSS are rewritten to name; what each file is comes from its bytes. A taken name gets a number.
func (s *Service) Import(ctx context.Context, pkg *Package) (Theme, db.LSN, error) {
	if pkg == nil || pkg.Format != PackageFormat {
		return Theme{}, 0, ErrNotAThemeFile
	}
	name := strings.TrimSpace(pkg.Name)
	if name == "" || len([]rune(name)) > 100 {
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
		taken := map[string]bool{}
		rows, err := q.Query(ctx, `SELECT lower(name) FROM theme`)
		if err != nil {
			return err
		}
		names, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
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
		actor, _ := db.UserFrom(ctx)
		if err := q.QueryRow(ctx, `INSERT INTO theme (name, spec, created_by) VALUES ($1, '{}', $2) RETURNING id`,
			made.Name, actor).Scan(&made.ID); err != nil {
			return duplicate(err)
		}

		renamed := map[uuid.UUID]uuid.UUID{}
		own := map[uuid.UUID]bool{}
		made.Assets = []Asset{}
		for i, a := range pkg.Assets {
			asset := Asset{Name: strings.TrimSpace(a.Name), ContentType: types[i], Size: len(a.Data)}
			if asset.Name == "" {
				asset.Name = "file"
			}
			if err := q.QueryRow(ctx, `INSERT INTO theme_asset (theme_id, name, content_type, data) VALUES ($1, $2, $3, $4) RETURNING id`,
				made.ID, asset.Name, asset.ContentType, a.Data).Scan(&asset.ID); err != nil {
				return err
			}
			renamed[a.ID] = asset.ID
			own[asset.ID] = true
			made.Assets = append(made.Assets, asset)
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
		made.Spec = spec
		_, err = q.Exec(ctx, `UPDATE theme SET spec = $2 WHERE id = $1`, made.ID, encoded)
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

// Export packs a theme, files included.
func (s *Service) Export(ctx context.Context, id uuid.UUID) (*Package, error) {
	var out *Package
	err := s.DB.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		rows, err := q.Query(ctx, `SELECT id, name, is_default, spec FROM theme WHERE id = $1`, id)
		if err != nil {
			return err
		}
		t, err := pgx.CollectExactlyOneRow(rows, scanTheme)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		out = &Package{Format: PackageFormat, Name: t.Name, Spec: t.Spec, Assets: []PackagedAsset{}}
		rows, err = q.Query(ctx, `SELECT id, name, content_type, data FROM theme_asset WHERE theme_id = $1 ORDER BY name, id`, id)
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

// File reads one of a theme's files.
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

// Delete removes a theme and its files, as an administrator. Whoever had chosen it falls back to the installation's.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) (db.LSN, error) {
	return s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		tag, err := q.Exec(ctx, `DELETE FROM theme WHERE id = $1`, id)
		if err == nil && tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return err
	})
}

// SetDefault names the theme everybody starts from, as an administrator; nil leaves everybody on the built-in look.
func (s *Service) SetDefault(ctx context.Context, id *uuid.UUID) (db.LSN, error) {
	return s.DB.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		if _, err := q.Exec(ctx, `UPDATE theme SET is_default = false WHERE is_default`); err != nil {
			return err
		}
		if id == nil {
			return nil
		}
		tag, err := q.Exec(ctx, `UPDATE theme SET is_default = true WHERE id = $1`, *id)
		if err == nil && tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return err
	})
}

func duplicate(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrDuplicateName
	}
	return err
}
