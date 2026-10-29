package theme

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func load(t *testing.T, name string) *Package {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var pkg Package
	if err := json.Unmarshal(raw, &pkg); err != nil {
		t.Fatal(err)
	}
	return &pkg
}

// The suite's files, as Armature and Exciter export them, take here: their files sniff as what they claim, and
// their specs, pointed at new ids, validate.
func TestTheSuitesThemeFilesTake(t *testing.T) {
	files, _ := filepath.Glob("testdata/*.armature-theme.json")
	if len(files) == 0 {
		t.Fatal("no theme files in testdata")
	}
	for _, file := range files {
		pkg := load(t, filepath.Base(file))
		if pkg.Format != PackageFormat {
			t.Errorf("%s is %q", file, pkg.Format)
		}
		themeID := uuid.New()
		renamed, own := map[uuid.UUID]uuid.UUID{}, map[uuid.UUID]bool{}
		for _, a := range pkg.Assets {
			kind, err := sniff(a.Name, a.Data)
			if err != nil || kind != a.ContentType {
				t.Errorf("%s: %s sniffs as %q, %v; claimed %q", file, a.Name, kind, err, a.ContentType)
			}
			renamed[a.ID] = uuid.New()
			own[renamed[a.ID]] = true
		}
		spec := pkg.Spec
		if err := rename(&spec, themeID, renamed); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		if err := Validate(&spec, themeID, own); err != nil {
			t.Errorf("%s: %v", file, err)
		}
		for _, id := range spec.AssetIDs() {
			if !own[id] {
				t.Errorf("%s still names %s after renaming", file, id)
			}
		}
	}
}

func TestASpecNamingAFileItDoesNotCarryIsNoThemeFile(t *testing.T) {
	pkg := load(t, "harbour.armature-theme.json")
	spec := pkg.Spec
	if err := rename(&spec, uuid.New(), map[uuid.UUID]uuid.UUID{}); !errors.Is(err, ErrNotAThemeFile) {
		t.Errorf("renaming with no files = %v", err)
	}
}

func TestOwnFilesInTheCSSFollowTheirNewIDs(t *testing.T) {
	oldTheme, oldFile, newTheme, newFile := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	spec := Spec{CSS: `.x { background: url("/api/v1/themes/` + oldTheme.String() + `/assets/` + oldFile.String() + `") }`}
	if err := rename(&spec, newTheme, map[uuid.UUID]uuid.UUID{oldFile: newFile}); err != nil {
		t.Fatal(err)
	}
	if want := "/api/v1/themes/" + newTheme.String() + "/assets/" + newFile.String(); !strings.Contains(spec.CSS, want) {
		t.Errorf("css is %s", spec.CSS)
	}
	if err := Validate(&spec, newTheme, map[uuid.UUID]bool{newFile: true}); err != nil {
		t.Error(err)
	}
}

func TestAThemeLoadsNothingFromElsewhere(t *testing.T) {
	for _, css := range []string{
		`.x { background: url(https://example.com/track.png) }`,
		`@import "https://example.com/x.css";`,
		`.x { background: url(/api/v1/themes/` + uuid.NewString() + `/assets/` + uuid.NewString() + `) }`,
	} {
		spec := Spec{CSS: css}
		if err := Validate(&spec, uuid.New(), map[uuid.UUID]bool{}); err == nil {
			t.Errorf("took %s", css)
		}
	}
	spec := Spec{Colors: Palette{Light: map[string]string{"canvas": "red; } body { display:none"}}}
	if err := Validate(&spec, uuid.New(), nil); err == nil {
		t.Error("took a colour that breaks out of its declaration")
	}
}

func TestAFileIsWhatItsBytesSay(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	if kind, err := sniff("picture.svg", png); err != nil || kind != "image/png" {
		t.Errorf("a PNG named .svg sniffs as %q, %v", kind, err)
	}
	if _, err := sniff("x.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)); !errors.Is(err, ErrUnsafeSVG) {
		t.Errorf("an SVG with script = %v", err)
	}
	if _, err := sniff("x.html", []byte(`<html><body>hi</body></html>`)); !errors.Is(err, ErrBadAssetType) {
		t.Errorf("a page = %v", err)
	}
	if _, err := sniff("big.png", make([]byte, MaxAssetBytes+1)); !errors.Is(err, ErrAssetTooLarge) {
		t.Errorf("an oversized file = %v", err)
	}
}

func TestImportRefusesWhatIsNoThemeFile(t *testing.T) {
	s := &Service{}
	for _, pkg := range []*Package{nil, {Format: "armature-theme/2", Name: "x"}, {Format: PackageFormat, Name: "  "}} {
		if _, _, err := s.Import(t.Context(), pkg); !errors.Is(err, ErrNotAThemeFile) {
			t.Errorf("%+v = %v", pkg, err)
		}
	}
	many := &Package{Format: PackageFormat, Name: "x", Assets: make([]PackagedAsset, MaxAssetsPerSpec+1)}
	if _, _, err := s.Import(t.Context(), many); !errors.Is(err, ErrTooManyAssets) {
		t.Errorf("too many files = %v", err)
	}
	bad := &Package{Format: PackageFormat, Name: "x", Assets: []PackagedAsset{{Name: "x.html", Data: []byte("<html></html>")}}}
	var invalid Invalid
	if _, _, err := s.Import(t.Context(), bad); !errors.As(err, &invalid) {
		t.Errorf("a page as a file = %v", err)
	}
}

func TestASpecIsHeldToItsLimits(t *testing.T) {
	own := uuid.New()
	files := map[uuid.UUID]bool{own: true}
	elsewhere := uuid.New()
	radius := func(n int) *int { return &n }
	for name, spec := range map[string]Spec{
		"a token name":        {Colors: Palette{Dark: map[string]string{"Canvas!": "#fff"}}},
		"a font family":       {Fonts: Fonts{Sans: &Font{Family: "Comic; Sans"}}},
		"a font's file":       {Fonts: Fonts{Mono: &Font{Family: "Mono", AssetID: &elsewhere}}},
		"a radius":            {Shape: Shape{RadiusControl: radius(MaxRadius + 1)}},
		"a shadow's key":      {Shadows: map[string]string{"4": "none"}},
		"a shadow's value":    {Shadows: map[string]string{"1": "url(x)"}},
		"a cursor's kind":     {Cursors: map[string]Cursor{"crosshair": {AssetID: own}}},
		"a cursor's file":     {Cursors: map[string]Cursor{"default": {AssetID: elsewhere}}},
		"a cursor's hotspot":  {Cursors: map[string]Cursor{"default": {AssetID: own, HotspotX: MaxHotspot + 1}}},
		"an icon's name":      {Icons: map[string]Icon{"Gear": {Paths: []string{"M1 1"}}}},
		"an icon with naught": {Icons: map[string]Icon{"gear": {}}},
		"an icon's path":      {Icons: map[string]Icon{"gear": {Paths: []string{"<script>"}}}},
		"a backdrop's file":   {Backdrop: &Backdrop{AssetID: elsewhere, Fit: "cover"}},
		"a backdrop's fit":    {Backdrop: &Backdrop{AssetID: own, Fit: "stretch"}},
		"an effect":           {Effect: "fireworks"},
		"the CSS's size":      {CSS: strings.Repeat("a", MaxCSSBytes+1)},
	} {
		if err := Validate(&spec, uuid.New(), files); err == nil {
			t.Errorf("%s out of bounds was taken", name)
		}
	}
	good := Spec{
		Colors:   Palette{Light: map[string]string{"canvas": "oklch(0.98 0.01 90)", "surface": "transparent"}},
		Fonts:    Fonts{Sans: &Font{Family: "Atkinson Hyperlegible", AssetID: &own}},
		Shape:    Shape{RadiusOverlay: radius(12)},
		Shadows:  map[string]string{"2": "0 4px 8px rgba(0, 0, 0, 0.2)"},
		Icons:    map[string]Icon{"gear": {AssetID: &own}, "search": {Paths: []string{"M1 1L15 15"}}},
		Backdrop: &Backdrop{AssetID: own, Fit: "tile"},
		Effect:   "confetti",
		CSS:      `.x { background: url(data:image/png;base64,AAAA) }`,
	}
	if err := Validate(&good, uuid.New(), files); err != nil {
		t.Errorf("a theme within bounds was refused: %v", err)
	}
	if ids := good.AssetIDs(); len(ids) != 3 {
		t.Errorf("the theme names %d files, want the font, the icon and the backdrop", len(ids))
	}
	if err := Validate(nil, uuid.New(), files); err == nil {
		t.Error("no spec was taken")
	}
}

func TestEveryKindOfFileTheSpecNamesIsRenamed(t *testing.T) {
	font, icon, backdrop := uuid.New(), uuid.New(), uuid.New()
	renamed := map[uuid.UUID]uuid.UUID{font: uuid.New(), icon: uuid.New(), backdrop: uuid.New()}
	wantFont, wantIcon, wantBackdrop := renamed[font], renamed[icon], renamed[backdrop]
	// The spec gets pointers of its own, which renaming rewrites in place.
	fontRef, iconRef := font, icon
	spec := Spec{
		Fonts:    Fonts{Sans: &Font{Family: "Sans", AssetID: &fontRef}},
		Icons:    map[string]Icon{"gear": {AssetID: &iconRef}},
		Backdrop: &Backdrop{AssetID: backdrop, Fit: "cover"},
	}
	if err := rename(&spec, uuid.New(), renamed); err != nil {
		t.Fatal(err)
	}
	if *spec.Fonts.Sans.AssetID != wantFont || *spec.Icons["gear"].AssetID != wantIcon || spec.Backdrop.AssetID != wantBackdrop {
		t.Errorf("not every file followed: %+v", spec)
	}
}
