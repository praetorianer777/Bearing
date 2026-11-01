//go:build integration

package test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/db"
	"github.com/praetorianer777/bearing/backend/internal/theme"
)

type themeJSON struct {
	ID, Name, OwnerID, OwnerName string
	Shared, Default, Active      bool
	InUse                        int
	Spec                         theme.Spec
	Assets                       []struct{ ID, Name, ContentType string }
}

type themeBody struct{ Theme themeJSON }

type activeBody struct {
	Theme  *themeJSON
	Source string
}

// siblingTheme is a theme as a sibling exports it: harbour from Armature, exciter from Exciter, meadow from Stator.
func siblingTheme(t *testing.T, name string) theme.Package {
	t.Helper()
	raw, err := os.ReadFile("../internal/theme/testdata/" + name + ".armature-theme.json")
	if err != nil {
		t.Fatal(err)
	}
	var pkg theme.Package
	if err := json.Unmarshal(raw, &pkg); err != nil {
		t.Fatal(err)
	}
	return pkg
}

func importTheme(t *testing.T, b *browser, pkg theme.Package) themeBody {
	t.Helper()
	var made struct {
		themeBody
		Error struct{ Message string }
	}
	if status := b.do(http.MethodPost, "/themes/import", pkg, &made); status != http.StatusCreated {
		t.Fatalf("importing answered %d: %s", status, made.Error.Message)
	}
	t.Cleanup(func() { b.do(http.MethodDelete, "/themes/"+made.Theme.ID, nil, nil) })
	return made.themeBody
}

func makeTheme(t *testing.T, b *browser, body map[string]any) themeJSON {
	t.Helper()
	var made struct {
		themeBody
		Error struct{ Message string }
	}
	if status := b.do(http.MethodPost, "/themes", body, &made); status != http.StatusCreated {
		t.Fatalf("making a theme answered %d: %s", status, made.Error.Message)
	}
	t.Cleanup(func() { b.do(http.MethodDelete, "/themes/"+made.Theme.ID, nil, nil) })
	return made.Theme
}

func listed(t *testing.T, b *browser, id string) *themeJSON {
	t.Helper()
	var list struct{ Themes []themeJSON }
	if status := b.do(http.MethodGet, "/themes", nil, &list); status != http.StatusOK {
		t.Fatalf("listing answered %d", status)
	}
	for _, th := range list.Themes {
		if th.ID == id {
			return &th
		}
	}
	return nil
}

func activeOf(t *testing.T, b *browser) activeBody {
	t.Helper()
	var got activeBody
	if status := b.do(http.MethodGet, "/themes/active", nil, &got); status != http.StatusOK {
		t.Fatalf("the active theme answered %d", status)
	}
	return got
}

// A theme file exported by a sibling comes back out of Bearing as it went in, but for the ids its files were given.
func TestASiblingsThemeRoundTrips(t *testing.T) {
	member, _ := withRole(t, signedIn(t, adminEmail, adminPassword), "member")
	for _, sibling := range []string{"harbour", "exciter", "meadow"} {
		t.Run(sibling, func(t *testing.T) {
			in := siblingTheme(t, sibling)
			made := importTheme(t, member, in)
			if made.Theme.Name != in.Name || len(made.Theme.Assets) != len(in.Assets) || made.Theme.Shared {
				t.Fatalf("imported as %+v", made.Theme)
			}

			var out theme.Package
			if status := member.do(http.MethodGet, "/themes/"+made.Theme.ID+"/export", nil, &out); status != http.StatusOK {
				t.Fatalf("exporting answered %d", status)
			}
			if out.Format != "armature-theme/1" || out.Name != in.Name || len(out.Assets) != len(in.Assets) {
				t.Fatalf("exported %s %q with %d files", out.Format, out.Name, len(out.Assets))
			}
			renamed := map[uuid.UUID]uuid.UUID{}
			for i, a := range in.Assets {
				got := out.Assets[i]
				if got.Name != a.Name || got.ContentType != a.ContentType || !bytes.Equal(got.Data, a.Data) {
					t.Errorf("file %s came back as %s %s, %d bytes", a.Name, got.Name, got.ContentType, len(got.Data))
				}
				if got.ID == a.ID {
					t.Errorf("file %s kept its id, which belongs to the installation it came from", a.Name)
				}
				renamed[got.ID] = a.ID
			}
			// The spec's maps are out's own, so the comparison works on a copy and out still imports as it was exported.
			var back theme.Spec
			raw, _ := json.Marshal(out.Spec)
			_ = json.Unmarshal(raw, &back)
			for kind, cursor := range back.Cursors {
				cursor.AssetID = renamed[cursor.AssetID]
				back.Cursors[kind] = cursor
			}
			if !reflect.DeepEqual(back, in.Spec) {
				t.Errorf("the spec changed on the way:\n in %+v\nout %+v", in.Spec, back)
			}

			// The exported file imports again, as a theme of its own with the name numbered.
			again := importTheme(t, member, out)
			if again.Theme.Name != in.Name+" (2)" {
				t.Errorf("importing again named it %q", again.Theme.Name)
			}
		})
	}
}

func TestAThemeIsItsMakersUntilShared(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	maker, makerID := withRole(t, admin, "member")
	other, otherID := withRole(t, admin, "reader")
	made := makeTheme(t, maker, map[string]any{"name": "Night", "spec": map[string]any{"colors": map[string]any{"light": map[string]string{"accent": "#225588"}}}})
	if made.OwnerID != makerID.String() || made.OwnerName != "member" || made.Shared || made.Spec.Colors.Light["accent"] != "#225588" {
		t.Fatalf("made %+v", made)
	}
	// Names are the owner's: another person may call theirs the same, the owner may not twice.
	makeTheme(t, other, map[string]any{"name": "Night"})
	if status := maker.do(http.MethodPost, "/themes", map[string]any{"name": "night"}, nil); status != http.StatusConflict {
		t.Errorf("a second theme of the same name answered %d", status)
	}
	if status := maker.do(http.MethodPost, "/themes", map[string]any{"name": " "}, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("a theme without a name answered %d", status)
	}

	replicated(t)
	if listed(t, other, made.ID) != nil {
		t.Error("somebody else sees a private theme")
	}
	if status := other.do(http.MethodGet, "/themes/"+made.ID, nil, nil); status != http.StatusNotFound {
		t.Errorf("somebody else read a private theme: %d", status)
	}
	if status := admin.do(http.MethodPatch, "/themes/"+made.ID, map[string]any{"name": "Taken over"}, nil); status != http.StatusNotFound {
		t.Errorf("an administrator changed a private theme: %d", status)
	}

	if status := maker.do(http.MethodPatch, "/themes/"+made.ID, map[string]any{"shared": true}, nil); status != http.StatusOK {
		t.Fatalf("sharing answered %d", status)
	}
	replicated(t)
	seen := listed(t, other, made.ID)
	if seen == nil || !seen.Shared || seen.Spec.Colors.Light["accent"] != "#225588" {
		t.Fatalf("somebody else sees the shared theme as %+v", seen)
	}
	if status := other.do(http.MethodPatch, "/themes/"+made.ID, map[string]any{"name": "Mine now"}, nil); status != http.StatusForbidden {
		t.Errorf("somebody else changed a shared theme: %d", status)
	}
	if status := other.do(http.MethodDelete, "/themes/"+made.ID, nil, nil); status != http.StatusForbidden {
		t.Errorf("somebody else deleted a shared theme: %d", status)
	}
	var tidied themeBody
	if status := admin.do(http.MethodPatch, "/themes/"+made.ID, map[string]any{"name": "Night, tidied"}, &tidied); status != http.StatusOK ||
		tidied.Theme.Name != "Night, tidied" || tidied.Theme.OwnerID != makerID.String() {
		t.Errorf("an administrator tidying a shared theme answered %d %+v", status, tidied.Theme)
	}

	// The database refuses what the service would have, whatever the api says.
	cluster, err := db.Open(context.Background(), loadConfig(t).DB, quietLog())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cluster.Close)
	_, err = cluster.Write(db.WithUser(context.Background(), otherID), func(ctx context.Context, q db.DBTX) error {
		_, err := q.Exec(ctx, `INSERT INTO theme (owner_id, name, spec) VALUES ($1, 'sneaked in', '{}')`, makerID)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "row-level security") {
		t.Errorf("somebody made a theme in another's name: %v", err)
	}
	_, err = cluster.Write(db.WithUser(context.Background(), otherID), func(ctx context.Context, q db.DBTX) error {
		tag, err := q.Exec(ctx, `UPDATE theme SET name = 'changed' WHERE id = $1`, made.ID)
		if err == nil && tag.RowsAffected() > 0 {
			t.Error("somebody changed another's shared theme in the database")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = cluster.Write(db.WithUser(context.Background(), makerID), func(ctx context.Context, q db.DBTX) error {
		_, err := q.Exec(ctx, `UPDATE theme SET is_default = true WHERE id = $1`, made.ID)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "only an administrator") {
		t.Errorf("a member named the theme everybody sees: %v", err)
	}
}

func TestTheChoiceFollowsThePersonAndFallsBackToTheDefault(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	email := uniqueEmail("chooser")
	if status := admin.do(http.MethodPost, "/accounts", map[string]string{"name": "Chooser", "email": email, "role": "member", "password": "chooser long password"}, nil); status != http.StatusCreated {
		t.Fatalf("making the chooser answered %d", status)
	}
	laptop := signedIn(t, email, "chooser long password")
	phone := signedIn(t, email, "chooser long password")
	maker, _ := withRole(t, admin, "editor")

	everybodys := makeTheme(t, admin, map[string]any{"name": "Everybody's " + uuid.NewString()[:8], "shared": true})
	private := makeTheme(t, admin, map[string]any{"name": "Private " + uuid.NewString()[:8]})
	var refusal struct{ Error struct{ Code string } }
	if status := admin.do(http.MethodPut, "/themes/default", map[string]any{"themeId": private.ID}, &refusal); status != http.StatusUnprocessableEntity || refusal.Error.Code != "not_shared" {
		t.Errorf("a private default answered %d %+v", status, refusal)
	}
	if status := admin.do(http.MethodPut, "/themes/default", map[string]any{"themeId": everybodys.ID}, nil); status != http.StatusNoContent {
		t.Fatalf("naming the default answered %d", status)
	}
	t.Cleanup(func() { admin.do(http.MethodPut, "/themes/default", map[string]any{"themeId": nil}, nil) })
	if status := laptop.do(http.MethodPut, "/themes/default", map[string]any{"themeId": nil}, nil); status != http.StatusForbidden {
		t.Errorf("a member named the default: %d", status)
	}

	replicated(t)
	if got := activeOf(t, laptop); got.Source != "everyone" || got.Theme == nil || got.Theme.ID != everybodys.ID || !got.Theme.Default {
		t.Errorf("before choosing, the chooser sees %+v", got)
	}

	theirs := makeTheme(t, maker, map[string]any{"name": "Theirs", "shared": true})
	replicated(t)
	var chosen struct{ Theme *themeJSON }
	if status := laptop.do(http.MethodPut, "/themes/active", map[string]any{"themeId": theirs.ID}, &chosen); status != http.StatusOK ||
		chosen.Theme == nil || !chosen.Theme.Active || chosen.Theme.InUse != 1 {
		t.Fatalf("choosing answered %d %+v", status, chosen.Theme)
	}
	replicated(t)
	if got := activeOf(t, phone); got.Source != "chosen" || got.Theme == nil || got.Theme.ID != theirs.ID {
		t.Errorf("on the other device the chooser sees %+v", got)
	}
	replicated(t)
	if seen := listed(t, maker, theirs.ID); seen == nil || seen.InUse != 1 || seen.Active {
		t.Errorf("the maker sees their theme as %+v", seen)
	}
	if status := laptop.do(http.MethodPut, "/themes/active", map[string]any{"themeId": private.ID}, nil); status != http.StatusNotFound {
		t.Errorf("choosing a theme out of sight answered %d", status)
	}

	// Deleting a theme in use returns its people to the default.
	if status := maker.do(http.MethodDelete, "/themes/"+theirs.ID, nil, nil); status != http.StatusNoContent {
		t.Fatalf("deleting answered %d", status)
	}
	replicated(t)
	if got := activeOf(t, laptop); got.Source != "everyone" || got.Theme == nil || got.Theme.ID != everybodys.ID {
		t.Errorf("after the theme went, the chooser sees %+v", got)
	}

	if status := laptop.do(http.MethodPut, "/themes/active", map[string]any{"themeId": nil, "builtIn": true}, nil); status != http.StatusOK {
		t.Fatalf("choosing the built-in look answered %d", status)
	}
	if got := activeOf(t, laptop); got.Source != "" || got.Theme != nil {
		t.Errorf("with the built-in look chosen, the chooser sees %+v", got)
	}
	if status := laptop.do(http.MethodPut, "/themes/active", map[string]any{"themeId": nil}, nil); status != http.StatusOK {
		t.Fatalf("returning to the default answered %d", status)
	}
	if got := activeOf(t, laptop); got.Source != "everyone" {
		t.Errorf("back on the default, the chooser sees %+v", got)
	}

	// A default taken private stops being anybody's.
	if status := admin.do(http.MethodPatch, "/themes/"+everybodys.ID, map[string]any{"shared": false}, nil); status != http.StatusOK {
		t.Fatalf("unsharing answered %d", status)
	}
	replicated(t)
	var fallback struct{ Theme *themeJSON }
	if status := laptop.do(http.MethodGet, "/themes/default", nil, &fallback); status != http.StatusOK || fallback.Theme != nil {
		t.Errorf("after unsharing, the default is %d %+v", status, fallback.Theme)
	}
}

const pictureSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 8 8"><rect width="8" height="8" fill="#123"/></svg>`

func TestAThemesFilesAreAddedByWhoeverMayChangeIt(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	maker, _ := withRole(t, admin, "member")
	other, _ := withRole(t, admin, "member")
	made := makeTheme(t, maker, map[string]any{"name": "Pictured", "shared": true})

	var added struct {
		Asset struct{ ID, Name, ContentType string }
		Error struct{ Message string }
	}
	if status := maker.do(http.MethodPost, "/themes/"+made.ID+"/assets", map[string]any{"name": "folder/back.svg", "data": []byte(pictureSVG)}, &added); status != http.StatusCreated ||
		added.Asset.Name != "back.svg" || added.Asset.ContentType != "image/svg+xml" {
		t.Fatalf("adding a file answered %d %+v", status, added)
	}
	if status := maker.do(http.MethodPost, "/themes/"+made.ID+"/assets", map[string]any{"name": "note.txt", "data": []byte("just words")}, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("a file a theme cannot use answered %d", status)
	}
	replicated(t)
	if status := other.do(http.MethodPost, "/themes/"+made.ID+"/assets", map[string]any{"name": "mine.svg", "data": []byte(pictureSVG)}, nil); status != http.StatusForbidden {
		t.Errorf("somebody else added a file: %d", status)
	}

	spec := map[string]any{"backdrop": map[string]any{"assetId": added.Asset.ID, "fit": "tile"}}
	if status := maker.do(http.MethodPatch, "/themes/"+made.ID, map[string]any{"spec": spec}, nil); status != http.StatusOK {
		t.Fatalf("using the file answered %d", status)
	}
	replicated(t)
	resp, err := other.client.Get(other.base + "/themes/" + made.ID + "/assets/" + added.Asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/svg+xml" || string(body) != pictureSVG ||
		resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("the file answered %d %s", resp.StatusCode, resp.Header)
	}
	if status := newBrowser(t).do(http.MethodGet, "/themes/"+made.ID+"/assets/"+added.Asset.ID, nil, nil); status != http.StatusUnauthorized {
		t.Errorf("somebody signed out read a theme's file: %d", status)
	}

	if status := maker.do(http.MethodDelete, "/themes/"+made.ID+"/assets/"+added.Asset.ID, nil, nil); status != http.StatusConflict {
		t.Errorf("removing a file in use answered %d", status)
	}
	if status := maker.do(http.MethodPatch, "/themes/"+made.ID, map[string]any{"spec": map[string]any{}}, nil); status != http.StatusOK {
		t.Fatalf("taking the file out answered %d", status)
	}
	if status := maker.do(http.MethodDelete, "/themes/"+made.ID+"/assets/"+added.Asset.ID, nil, nil); status != http.StatusNoContent {
		t.Errorf("removing a file answered %d", status)
	}
	if status := maker.do(http.MethodDelete, "/themes/"+made.ID+"/assets/"+added.Asset.ID, nil, nil); status != http.StatusNotFound {
		t.Errorf("removing it again answered %d", status)
	}
}

func TestTheExamplesAreThereToStartFrom(t *testing.T) {
	reader, _ := withRole(t, signedIn(t, adminEmail, adminPassword), "reader")
	var got struct {
		Examples []struct {
			Key, Name string
			Spec      theme.Spec
		}
	}
	if status := reader.do(http.MethodGet, "/themes/examples", nil, &got); status != http.StatusOK || len(got.Examples) != 2 {
		t.Fatalf("the examples answered %d %+v", status, got)
	}
	for _, e := range got.Examples {
		made := makeTheme(t, reader, map[string]any{"name": e.Name, "spec": e.Spec})
		if made.Spec.Effect != e.Spec.Effect {
			t.Errorf("%s saved its effect as %q", e.Name, made.Spec.Effect)
		}
	}
}

func TestAThemeFileThatIsNotOneIsRefused(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	var refusal struct {
		Error struct{ Code, Message string }
	}
	if status := admin.do(http.MethodPost, "/themes/import", map[string]any{"format": "something/1", "name": "x"}, &refusal); status != http.StatusUnprocessableEntity || refusal.Error.Code != "not_a_theme_file" {
		t.Errorf("another format answered %d %+v", status, refusal)
	}
	pkg := siblingTheme(t, "harbour")
	pkg.Name = "Phoning home " + uuid.NewString()[:8]
	pkg.Spec.CSS = `body { background: url(https://example.com/beacon.png) }`
	if status := admin.do(http.MethodPost, "/themes/import", pkg, &refusal); status != http.StatusUnprocessableEntity || refusal.Error.Code != "invalid_theme" {
		t.Errorf("a theme loading from elsewhere answered %d %+v", status, refusal)
	}
	var list struct{ Themes []struct{ Name string } }
	admin.do(http.MethodGet, "/themes", nil, &list)
	for _, th := range list.Themes {
		if th.Name == pkg.Name {
			t.Error("the refused theme was kept")
		}
	}
}
