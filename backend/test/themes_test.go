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
	"time"

	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/db"
	"github.com/praetorianer777/bearing/backend/internal/theme"
)

type themeBody struct {
	Theme struct {
		ID      string
		Name    string
		Default bool
		Spec    theme.Spec
		Assets  []struct{ ID, Name, ContentType string }
	}
}

// armatureTheme is a theme as Armature exports it.
func armatureTheme(t *testing.T) theme.Package {
	t.Helper()
	raw, err := os.ReadFile("../internal/theme/testdata/harbour.armature-theme.json")
	if err != nil {
		t.Fatal(err)
	}
	var pkg theme.Package
	if err := json.Unmarshal(raw, &pkg); err != nil {
		t.Fatal(err)
	}
	return pkg
}

func importTheme(t *testing.T, admin *browser, pkg theme.Package) themeBody {
	t.Helper()
	var made struct {
		themeBody
		Error struct{ Message string }
	}
	if status := admin.do(http.MethodPost, "/themes/import", pkg, &made); status != http.StatusCreated {
		t.Fatalf("importing answered %d: %s", status, made.Error.Message)
	}
	t.Cleanup(func() { admin.do(http.MethodDelete, "/themes/"+made.Theme.ID, nil, nil) })
	return made.themeBody
}

// A theme file exported by Armature comes back out of Bearing as it went in, but for the ids its files were given.
func TestAnArmatureThemeRoundTrips(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	in := armatureTheme(t)
	in.Name = "Harbour " + uuid.NewString()[:8]
	made := importTheme(t, admin, in)
	if made.Theme.Name != in.Name || len(made.Theme.Assets) != len(in.Assets) {
		t.Fatalf("imported as %+v", made.Theme)
	}

	var out theme.Package
	if status := admin.do(http.MethodGet, "/themes/"+made.Theme.ID+"/export", nil, &out); status != http.StatusOK {
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
	again := importTheme(t, admin, out)
	if again.Theme.Name != in.Name+" (2)" {
		t.Errorf("importing again named it %q", again.Theme.Name)
	}
}

func TestEverybodyReadsThemesAndOnlyAdministratorsChangeThem(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	pkg := armatureTheme(t)
	pkg.Name = "Shared " + uuid.NewString()[:8]
	made := importTheme(t, admin, pkg)
	reader, readerID := withRole(t, admin, "reader")

	var list struct{ Themes []struct{ ID string } }
	found := false
	for range 50 {
		reader.do(http.MethodGet, "/themes", nil, &list)
		for _, th := range list.Themes {
			found = found || th.ID == made.Theme.ID
		}
		if found {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !found {
		t.Fatal("a reader does not see the theme")
	}
	resp, err := reader.client.Get(reader.base + "/themes/" + made.Theme.ID + "/assets/" + made.Theme.Assets[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/svg+xml" || !strings.Contains(string(body), "<svg") ||
		resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("the file answered %d %s", resp.StatusCode, resp.Header)
	}
	if status := newBrowser(t).do(http.MethodGet, "/themes/"+made.Theme.ID+"/assets/"+made.Theme.Assets[0].ID, nil, nil); status != http.StatusUnauthorized {
		t.Errorf("somebody signed out read a theme's file: %d", status)
	}

	if status := reader.do(http.MethodPost, "/themes/import", pkg, nil); status != http.StatusForbidden {
		t.Errorf("a reader imported a theme: %d", status)
	}
	if status := reader.do(http.MethodDelete, "/themes/"+made.Theme.ID, nil, nil); status != http.StatusForbidden {
		t.Errorf("a reader removed a theme: %d", status)
	}

	if status := admin.do(http.MethodPut, "/themes/default", map[string]any{"themeId": made.Theme.ID}, nil); status != http.StatusNoContent {
		t.Fatalf("naming the default answered %d", status)
	}
	t.Cleanup(func() { admin.do(http.MethodPut, "/themes/default", map[string]any{"themeId": nil}, nil) })
	if status := admin.do(http.MethodPut, "/themes/default", map[string]any{"themeId": uuid.NewString()}, nil); status != http.StatusNotFound {
		t.Errorf("an unknown default answered %d", status)
	}

	// The database refuses a reader's write whatever the api would have said.
	cluster, err := db.Open(context.Background(), loadConfig(t).DB, quietLog())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cluster.Close)
	_, err = cluster.Write(db.WithUser(context.Background(), readerID), func(ctx context.Context, q db.DBTX) error {
		_, err := q.Exec(ctx, `INSERT INTO theme (name, spec) VALUES ('sneaked in', '{}')`)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "row-level security") {
		t.Errorf("a reader wrote a theme: %v", err)
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
	pkg := armatureTheme(t)
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
