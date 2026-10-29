package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/praetorianer777/bearing/backend/internal/theme"
)

type themeAssetResponse struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	ContentType string    `json:"contentType"`
	Size        int       `json:"size"`
}

type themeResponse struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	// Default is the theme everybody starts from until they choose another.
	Default bool                 `json:"default"`
	Spec    theme.Spec           `json:"spec"`
	Assets  []themeAssetResponse `json:"assets"`
}

type defaultThemeRequest struct {
	// ThemeID is the theme everybody starts from; null is the built-in look.
	ThemeID *uuid.UUID `json:"themeId"`
}

func toTheme(t theme.Theme) themeResponse {
	out := themeResponse{ID: t.ID, Name: t.Name, Default: t.Default, Spec: t.Spec, Assets: make([]themeAssetResponse, len(t.Assets))}
	for i, a := range t.Assets {
		out.Assets[i] = themeAssetResponse{ID: a.ID, Name: a.Name, ContentType: a.ContentType, Size: a.Size}
	}
	return out
}

// themeError says a theme refusal in the API's terms.
func themeError(err error) error {
	var invalid theme.Invalid
	switch {
	case errors.Is(err, theme.ErrNotFound):
		return ErrNotFound()
	case errors.Is(err, theme.ErrDuplicateName):
		return ErrConflict("duplicate_name", "Another theme has that name.")
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
