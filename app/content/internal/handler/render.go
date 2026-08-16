package handler

import (
	"database/sql"
	"errors"
	"fmt"
	nethtml "html"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/quillit/contentengine/filter"
	"github.com/quillit/contentengine/parse"
	"github.com/quillit/contentengine/render"

	"github.com/quillit/content-svc/internal/authz"
	contentresolver "github.com/quillit/content-svc/internal/resolver"
)

// RenderHandler serves docs/web-refactor-spec.md §6.4's render endpoint —
// "the only way player-facing HTML is ever produced" (golden rule 5).
// Nothing else in the web app may re-derive a player or card view
// client-side; everything routes through here.
type RenderHandler struct {
	db        *sql.DB
	blobs     BlobStore
	jwtSecret []byte
	checker   authz.Checker
}

func NewRender(db *sql.DB, blobs BlobStore, jwtSecret string, checker authz.Checker) *RenderHandler {
	return &RenderHandler{db: db, blobs: blobs, jwtSecret: []byte(jwtSecret), checker: checker}
}

// Render godoc
// @Summary      Render an entry to an HTML fragment
// @Description  view and card are mutually exclusive; card implies DM audience. quick=1 composes with either.
// @Tags         render
// @Produce      html
// @Param        id     path   string  true   "Entry ID"
// @Param        view   query  string  false  "dm (default) or player"
// @Param        card   query  string  false  "facet name"
// @Param        quick  query  string  false  "1 to truncate to the first surviving block"
// @Success      200    {string}  string
// @Failure      400    {object}  ErrorResponse
// @Failure      404    {object}  ErrorResponse
// @Failure      422    {object}  ErrorResponse
// @Router       /content/entries/{id}/render [get]
func (h *RenderHandler) Render(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	m, err := scanEntryMeta(h.db.QueryRowContext(r.Context(), entryMetaSelect+" WHERE id = ?", id))
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if _, ok := requireProjectMember(w, r, h.jwtSecret, h.checker, m.ProjectID); !ok {
		return
	}

	if h.blobs == nil {
		writeError(w, http.StatusServiceUnavailable, "blob storage not configured")
		return
	}
	data, err := h.blobs.Get(r.Context(), bodyKey(id))
	if err != nil {
		writeError(w, http.StatusNotFound, "entry body not found")
		return
	}

	parsed, err := parse.Parse(data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	view, ok := parseView(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "view and card are mutually exclusive")
		return
	}

	_, sortedVocab, err := effectiveFacetVocabulary(r.Context(), h.db, m.ProjectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	filtered, err := filter.Filter(parsed, view, sortedVocab)
	if err != nil {
		var uf filter.UnknownFacet
		if errors.As(err, &uf) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"error":      "unknown facet",
				"facet":      uf.Name,
				"vocabulary": uf.Vocabulary,
			})
			return
		}
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	resolver := &contentresolver.SQLite{DB: h.db, Blobs: h.blobs, ProjectID: m.ProjectID, Ctx: r.Context()}
	fragment, err := render.Render(filtered, view, resolver, webLinkRenderer)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "render error")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(fragment))
}

// parseView reads view/card/quick query params into a filter.View. ok is
// false when view and card are both set (docs/cli-spec.md §7 "render":
// mutually exclusive).
func parseView(r *http.Request) (filter.View, bool) {
	q := r.URL.Query()
	viewParam := q.Get("view")
	cardParam := q.Get("card")
	quick := q.Get("quick") == "1"

	if viewParam != "" && cardParam != "" {
		return filter.View{}, false
	}
	if cardParam != "" {
		return filter.View{Kind: filter.ViewCard, Facet: cardParam, Quick: quick}, true
	}
	if viewParam == "player" {
		return filter.View{Kind: filter.ViewPlayer, Quick: quick}, true
	}
	// "" and "dm" both mean the default DM view.
	return filter.View{Kind: filter.ViewDM, Quick: quick}, true
}

// webLinkRenderer is this service's LinkRenderer (render.LinkRenderer):
// non-expanded/depth-exceeded links become in-app navigation references —
// an <a> the frontend's router intercepts via data-quillit-link — never the
// CLI's clipboard-link presentation, which is meaningless outside its
// static-HTML-scaffolding context (docs/web-refactor-spec.md §7.1
// requirement 3). Player view never calls this at all — render.Render's own
// structural guarantee, not something this callback needs to enforce.
func webLinkRenderer(info render.LinkInfo) string {
	class := "wikilink"
	if info.NoMatchingCard {
		class += " wikilink--no-card"
	}
	href := "/entries/" + info.Path
	return fmt.Sprintf(
		`<a href="%s" class="%s" data-quillit-link="%s">%s</a>`,
		nethtml.EscapeString(href),
		class,
		nethtml.EscapeString(info.Path),
		nethtml.EscapeString(info.Label),
	)
}
