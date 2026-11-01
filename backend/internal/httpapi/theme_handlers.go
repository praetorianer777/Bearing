package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/theme"
)

// Themes are their makers', shared with everybody when they say so, as in Exciter and Stator. Changing one is the
// owner's, or an administrator's once shared; each person's choice follows them between devices.

type themeAssetResponse struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	ContentType string    `json:"contentType"`
	Size        int       `json:"size"`
}

type themeResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	OwnerID   uuid.UUID `json:"ownerId"`
	OwnerName string    `json:"ownerName"`
	Shared    bool      `json:"shared"`
	// Default is the theme everybody sees until they choose another; active says the caller chose it.
	Default bool `json:"default"`
	Active  bool `json:"active"`
	// InUse counts the people who chose it.
	InUse     int                  `json:"inUse"`
	Spec      theme.Spec           `json:"spec"`
	Assets    []themeAssetResponse `json:"assets"`
	UpdatedAt time.Time            `json:"updatedAt"`
}

type activeThemeResponse struct {
	// Theme is what the caller sees; null is the built-in look.
	Theme *themeResponse `json:"theme"`
	// Source is where it comes from: "chosen", "everyone" for the installation's default, or "" for the built-in look.
	Source string `json:"source"`
}

type themeExampleResponse struct {
	Key         string     `json:"key"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Spec        theme.Spec `json:"spec"`
}

type defaultThemeRequest struct {
	// ThemeID is the shared theme everybody sees; null is the built-in look.
	ThemeID *uuid.UUID `json:"themeId"`
}

type chooseThemeRequest struct {
	// ThemeID is the theme to use; null returns to the installation's default.
	ThemeID *uuid.UUID `json:"themeId"`
	// BuiltIn with no theme keeps the built-in look over the installation's default.
	BuiltIn bool `json:"builtIn,omitempty"`
}

type themeAssetRequest struct {
	Name string `json:"name"`
	// Data is the file's bytes, base64 as JSON carries them.
	Data []byte `json:"data"`
}

func toTheme(t theme.Theme) themeResponse {
	out := themeResponse{ID: t.ID, Name: t.Name, OwnerID: t.OwnerID, OwnerName: t.OwnerName, Shared: t.Shared, Default: t.Default,
		Active: t.Active, InUse: t.InUse, Spec: t.Spec, Assets: make([]themeAssetResponse, len(t.Assets)), UpdatedAt: t.UpdatedAt}
	for i, a := range t.Assets {
		out.Assets[i] = toThemeAsset(a)
	}
	return out
}

func toThemeAsset(a theme.Asset) themeAssetResponse {
	return themeAssetResponse{ID: a.ID, Name: a.Name, ContentType: a.ContentType, Size: a.Size}
}

func toThemePointer(t *theme.Theme) *themeResponse {
	if t == nil {
		return nil
	}
	out := toTheme(*t)
	return &out
}

// themeError says a theme refusal in the API's terms.
func themeError(err error) error {
	var invalid theme.Invalid
	switch {
	case errors.Is(err, theme.ErrNotFound):
		return ErrNotFound()
	case errors.Is(err, theme.ErrDuplicateName):
		return ErrConflict("duplicate_name", sentence(err.Error()))
	case errors.Is(err, theme.ErrNotYours):
		return ErrForbidden(sentence(err.Error()))
	case errors.Is(err, theme.ErrAssetInUse):
		return ErrConflict("asset_in_use", sentence(err.Error()))
	case errors.Is(err, theme.ErrNoName):
		return ErrValidation(map[string]string{"name": sentence(err.Error())})
	case errors.Is(err, theme.ErrDefaultNotShared):
		return &APIError{Status: http.StatusUnprocessableEntity, Code: "not_shared", Message: sentence(err.Error())}
	case errors.As(err, &invalid):
		return &APIError{Status: http.StatusUnprocessableEntity, Code: "invalid_theme", Message: sentence(invalid.Error())}
	case errors.Is(err, theme.ErrNotAThemeFile):
		return &APIError{Status: http.StatusUnprocessableEntity, Code: "not_a_theme_file", Message: sentence(err.Error())}
	}
	return err
}

// sentence makes a refusal from below read as one: a capital first, a full stop last.
func sentence(s string) string {
	if s == "" {
		return s
	}
	s = strings.ToUpper(s[:1]) + s[1:]
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}

func (s *Server) themeID(w http.ResponseWriter, r *http.Request, param string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, param))
	if err != nil {
		respondError(w, r, s.Log, ErrNotFound())
		return uuid.Nil, false
	}
	return id, true
}

func (s *Server) handleThemes(w http.ResponseWriter, r *http.Request) {
	themes, err := s.Themes.List(r.Context())
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	out := make([]themeResponse, len(themes))
	for i, t := range themes {
		out[i] = toTheme(t)
	}
	respondJSON(w, http.StatusOK, env{"themes": out})
}

func (s *Server) handleThemeExamples(w http.ResponseWriter, _ *http.Request) {
	examples := theme.Examples()
	out := make([]themeExampleResponse, len(examples))
	for i, e := range examples {
		out[i] = themeExampleResponse{Key: e.Key, Name: e.Name, Description: e.Description, Spec: e.Spec}
	}
	respondJSON(w, http.StatusOK, env{"examples": out})
}

func (s *Server) handleCreateTheme(w http.ResponseWriter, r *http.Request) {
	var in theme.Input
	if err := decodeUpTo(r, &in, theme.MaxSpecBytes*2); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	made, lsn, err := s.Themes.Create(r.Context(), in)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, themeError(err))
		return
	}
	respondJSON(w, http.StatusCreated, env{"theme": toTheme(made)})
}

func (s *Server) handleTheme(w http.ResponseWriter, r *http.Request) {
	id, ok := s.themeID(w, r, "themeID")
	if !ok {
		return
	}
	got, err := s.Themes.Get(r.Context(), id)
	if err != nil {
		respondError(w, r, s.Log, themeError(err))
		return
	}
	respondJSON(w, http.StatusOK, env{"theme": toTheme(got)})
}

func (s *Server) handleUpdateTheme(w http.ResponseWriter, r *http.Request) {
	id, ok := s.themeID(w, r, "themeID")
	if !ok {
		return
	}
	var in theme.Input
	if err := decodeUpTo(r, &in, theme.MaxSpecBytes*2); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	updated, lsn, err := s.Themes.Update(r.Context(), id, in)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, themeError(err))
		return
	}
	respondJSON(w, http.StatusOK, env{"theme": toTheme(updated)})
}

func (s *Server) handleActiveTheme(w http.ResponseWriter, r *http.Request) {
	active, source, err := s.Themes.Active(r.Context())
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	respondJSON(w, http.StatusOK, activeThemeResponse{Theme: toThemePointer(active), Source: string(source)})
}

func (s *Server) handleChooseTheme(w http.ResponseWriter, r *http.Request) {
	var req chooseThemeRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	chosen, lsn, err := s.Themes.Choose(r.Context(), req.ThemeID, req.BuiltIn)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, themeError(err))
		return
	}
	respondJSON(w, http.StatusOK, env{"theme": toThemePointer(chosen)})
}

func (s *Server) handleDefaultTheme(w http.ResponseWriter, r *http.Request) {
	got, err := s.Themes.Default(r.Context())
	if err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	respondJSON(w, http.StatusOK, env{"theme": toThemePointer(got)})
}

func (s *Server) handleImportTheme(w http.ResponseWriter, r *http.Request) {
	var pkg theme.Package
	if err := decodeUpTo(r, &pkg, theme.MaxPackageBytes); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	made, lsn, err := s.Themes.Import(r.Context(), &pkg)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, themeError(err))
		return
	}
	respondJSON(w, http.StatusCreated, env{"theme": toTheme(made)})
}

func (s *Server) handleExportTheme(w http.ResponseWriter, r *http.Request) {
	id, ok := s.themeID(w, r, "themeID")
	if !ok {
		return
	}
	pkg, err := s.Themes.Export(r.Context(), id)
	if err != nil {
		respondError(w, r, s.Log, themeError(err))
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+fileName(pkg.Name)+`.armature-theme.json"`)
	respondJSON(w, http.StatusOK, pkg)
}

// fileName keeps the letters, digits and dashes of a theme's name, so it is safe in a header and on any disk.
func fileName(name string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(name) {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b.WriteRune(c)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteByte('-')
		}
	}
	if out := strings.Trim(b.String(), "-"); out != "" {
		return out
	}
	return "theme"
}

func (s *Server) handleAddThemeAsset(w http.ResponseWriter, r *http.Request) {
	id, ok := s.themeID(w, r, "themeID")
	if !ok {
		return
	}
	var req themeAssetRequest
	if err := decodeUpTo(r, &req, theme.MaxAssetBytes*4/3+64<<10); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	made, lsn, err := s.Themes.AddAsset(r.Context(), id, req.Name, req.Data)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, themeError(err))
		return
	}
	respondJSON(w, http.StatusCreated, env{"asset": toThemeAsset(made)})
}

func (s *Server) handleRemoveThemeAsset(w http.ResponseWriter, r *http.Request) {
	id, ok := s.themeID(w, r, "themeID")
	if !ok {
		return
	}
	assetID, ok := s.themeID(w, r, "assetID")
	if !ok {
		return
	}
	lsn, err := s.Themes.RemoveAsset(r.Context(), id, assetID)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, themeError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleThemeFile serves a file as a download, so an SVG never renders as a page of ours; its id never changes, so
// the browser may keep it.
func (s *Server) handleThemeFile(w http.ResponseWriter, r *http.Request) {
	id, ok := s.themeID(w, r, "themeID")
	if !ok {
		return
	}
	assetID, ok := s.themeID(w, r, "assetID")
	if !ok {
		return
	}
	a, err := s.Themes.File(r.Context(), id, assetID)
	if err != nil {
		respondError(w, r, s.Log, themeError(err))
		return
	}
	h := w.Header()
	h.Set("Content-Type", a.ContentType)
	h.Set("Content-Length", strconv.Itoa(a.Size))
	h.Set("Content-Disposition", `attachment; filename="`+fileName(a.Name)+`"`)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	h.Set("Cache-Control", "private, max-age=31536000, immutable")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(a.Data)
}

func (s *Server) handleDeleteTheme(w http.ResponseWriter, r *http.Request) {
	id, ok := s.themeID(w, r, "themeID")
	if !ok {
		return
	}
	lsn, err := s.Themes.Delete(r.Context(), id)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, themeError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSetDefaultTheme(w http.ResponseWriter, r *http.Request) {
	var req defaultThemeRequest
	if err := decode(r, &req); err != nil {
		respondError(w, r, s.Log, err)
		return
	}
	lsn, err := s.Themes.SetDefault(r.Context(), req.ThemeID)
	noteWrite(r.Context(), lsn)
	if err != nil {
		respondError(w, r, s.Log, themeError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
