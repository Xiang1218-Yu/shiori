package api_v1

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-shiori/shiori/internal/http/middleware"
	"github.com/go-shiori/shiori/internal/http/response"
	"github.com/go-shiori/shiori/internal/model"
)

type bookmarkAliasPayload struct {
	URL string `json:"url"`
}

func (p *bookmarkAliasPayload) IsValid() error {
	p.URL = strings.TrimSpace(p.URL)
	if p.URL == "" {
		return errors.New("url should not be empty")
	}
	return nil
}

type bookmarkCapabilitiesResponse struct {
	HistoryURLsEnabled bool `json:"historyUrlsEnabled"`
}

// HandleBookmarkCapabilities reports which optional bookmark features are
// enabled on the server.
//
//	@Summary	Get bookmark feature capabilities.
//	@Tags		Bookmarks
//	@Security	ApiKeyAuth
//	@Produce	json
//	@Success	200	{object}	bookmarkCapabilitiesResponse
//	@Failure	403	{object}	nil	"Token not provided/invalid"
//	@Router		/api/v1/bookmarks/capabilities [get]
func HandleBookmarkCapabilities(deps model.Dependencies, c model.WebContext) {
	if err := middleware.RequireLoggedInUser(deps, c); err != nil {
		return
	}

	response.SendJSON(c, http.StatusOK, bookmarkCapabilitiesResponse{
		HistoryURLsEnabled: deps.Domains().Bookmarks().HistoryURLsEnabled(),
	})
}

// HandleResolveBookmark resolves a current or history URL to the stable
// bookmark identity behind it.
//
//	@Summary	Resolve a bookmark by its current or history URL.
//	@Tags		Bookmarks
//	@Security	ApiKeyAuth
//	@Param		url	query	string	true	"Current or history URL of the bookmark"
//	@Produce	json
//	@Success	200	{object}	model.BookmarkDTO
//	@Failure	400	{object}	nil	"Missing url query parameter"
//	@Failure	403	{object}	nil	"Token not provided/invalid"
//	@Failure	404	{object}	nil	"No bookmark matches the URL"
//	@Router		/api/v1/bookmarks/resolve [get]
func HandleResolveBookmark(deps model.Dependencies, c model.WebContext) {
	if err := middleware.RequireLoggedInUser(deps, c); err != nil {
		return
	}

	url := strings.TrimSpace(c.Request().URL.Query().Get("url"))
	if url == "" {
		response.SendError(c, http.StatusBadRequest, "Query parameter \"url\" is required")
		return
	}

	bookmark, err := deps.Domains().Bookmarks().ResolveBookmarkByURL(c.Request().Context(), url)
	if err != nil {
		if errors.Is(err, model.ErrBookmarkNotFound) {
			response.SendError(c, http.StatusNotFound, "No bookmark matches the given URL")
			return
		}
		deps.Logger().WithError(err).Error("failed to resolve bookmark by URL")
		response.SendInternalServerError(c)
		return
	}

	response.SendJSON(c, http.StatusOK, bookmark)
}

// HandleAddBookmarkAlias associates a history URL with a bookmark.
//
//	@Summary	Add a history URL to a bookmark.
//	@Tags		Bookmarks
//	@Security	ApiKeyAuth
//	@Param		id		path	int						true	"Bookmark ID"
//	@Param		payload	body	bookmarkAliasPayload	true	"History URL payload"
//	@Produce	json
//	@Success	201	{object}	nil
//	@Failure	400	{object}	nil	"Invalid payload or history URLs disabled"
//	@Failure	403	{object}	nil	"Token not provided/invalid"
//	@Failure	404	{object}	nil	"Bookmark not found"
//	@Failure	409	{object}	nil	"URL already used by another bookmark"
//	@Router		/api/v1/bookmarks/{id}/aliases [post]
func HandleAddBookmarkAlias(deps model.Dependencies, c model.WebContext) {
	if err := middleware.RequireLoggedInAdmin(deps, c); err != nil {
		return
	}

	bookmarkID, err := strconv.Atoi(c.Request().PathValue("id"))
	if err != nil {
		response.SendError(c, http.StatusBadRequest, "Invalid bookmark ID")
		return
	}

	var payload bookmarkAliasPayload
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		response.SendError(c, http.StatusBadRequest, "Invalid request payload")
		return
	}

	if err := payload.IsValid(); err != nil {
		response.SendError(c, http.StatusBadRequest, err.Error())
		return
	}

	err = deps.Domains().Bookmarks().AddBookmarkAlias(c.Request().Context(), bookmarkID, payload.URL)
	if err != nil {
		handleBookmarkAliasError(deps, c, err)
		return
	}

	response.SendJSON(c, http.StatusCreated, nil)
}

// HandleDeleteBookmarkAlias removes a history URL from a bookmark.
//
//	@Summary	Remove a history URL from a bookmark.
//	@Tags		Bookmarks
//	@Security	ApiKeyAuth
//	@Param		id		path	int						true	"Bookmark ID"
//	@Param		payload	body	bookmarkAliasPayload	true	"History URL payload"
//	@Produce	json
//	@Success	200	{object}	nil
//	@Failure	400	{object}	nil	"Invalid payload or history URLs disabled"
//	@Failure	403	{object}	nil	"Token not provided/invalid"
//	@Failure	404	{object}	nil	"Bookmark not found"
//	@Router		/api/v1/bookmarks/{id}/aliases [delete]
func HandleDeleteBookmarkAlias(deps model.Dependencies, c model.WebContext) {
	if err := middleware.RequireLoggedInAdmin(deps, c); err != nil {
		return
	}

	bookmarkID, err := strconv.Atoi(c.Request().PathValue("id"))
	if err != nil {
		response.SendError(c, http.StatusBadRequest, "Invalid bookmark ID")
		return
	}

	var payload bookmarkAliasPayload
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		response.SendError(c, http.StatusBadRequest, "Invalid request payload")
		return
	}

	if err := payload.IsValid(); err != nil {
		response.SendError(c, http.StatusBadRequest, err.Error())
		return
	}

	err = deps.Domains().Bookmarks().DeleteBookmarkAlias(c.Request().Context(), bookmarkID, payload.URL)
	if err != nil {
		handleBookmarkAliasError(deps, c, err)
		return
	}

	response.SendJSON(c, http.StatusOK, nil)
}

// handleBookmarkAliasError maps domain errors onto explicit HTTP status
// codes so conflicts and disabled features are never silent.
func handleBookmarkAliasError(deps model.Dependencies, c model.WebContext, err error) {
	switch {
	case errors.Is(err, model.ErrBookmarkHistoryDisabled):
		response.SendError(c, http.StatusBadRequest, "Bookmark history URLs are disabled on this server")
	case errors.Is(err, model.ErrBookmarkNotFound):
		response.SendError(c, http.StatusNotFound, "Bookmark not found")
	case errors.Is(err, model.ErrBookmarkAliasConflict):
		response.SendError(c, http.StatusConflict, "The URL is already used by another bookmark")
	default:
		deps.Logger().WithError(err).Error("failed to manage bookmark alias")
		response.SendInternalServerError(c)
	}
}
