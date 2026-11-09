package api_v1

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/go-shiori/shiori/internal/testutil"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestHandleBookmarkCapabilities(t *testing.T) {
	logger := logrus.New()
	ctx := context.Background()

	t.Run("requires authentication", func(t *testing.T) {
		_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)
		w := testutil.PerformRequest(deps, HandleBookmarkCapabilities, http.MethodGet,
			"/api/v1/bookmarks/capabilities")
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("disabled by default", func(t *testing.T) {
		_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)
		w := testutil.PerformRequest(deps, HandleBookmarkCapabilities, http.MethodGet,
			"/api/v1/bookmarks/capabilities", testutil.WithFakeUser())
		require.Equal(t, http.StatusOK, w.Code)

		response := testutil.NewTestResponseFromRecorder(w)
		response.AssertOk(t)
		response.AssertMessageJSONKeyValue(t, "historyUrlsEnabled", func(t *testing.T, value any) {
			require.Equal(t, false, value)
		})
	})

	t.Run("reflects enabled flag", func(t *testing.T) {
		_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)
		deps.Database().SetHistoryURLsEnabled(true)

		w := testutil.PerformRequest(deps, HandleBookmarkCapabilities, http.MethodGet,
			"/api/v1/bookmarks/capabilities", testutil.WithFakeUser())
		require.Equal(t, http.StatusOK, w.Code)

		response := testutil.NewTestResponseFromRecorder(w)
		response.AssertMessageJSONKeyValue(t, "historyUrlsEnabled", func(t *testing.T, value any) {
			require.Equal(t, true, value)
		})
	})
}

func TestHandleResolveBookmark(t *testing.T) {
	logger := logrus.New()
	ctx := context.Background()

	t.Run("requires authentication", func(t *testing.T) {
		_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)
		w := testutil.PerformRequest(deps, HandleResolveBookmark, http.MethodGet,
			"/api/v1/bookmarks/resolve")
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("missing url parameter", func(t *testing.T) {
		_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)
		w := testutil.PerformRequest(deps, HandleResolveBookmark, http.MethodGet,
			"/api/v1/bookmarks/resolve", testutil.WithFakeUser())
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("unknown url returns 404", func(t *testing.T) {
		_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)
		w := testutil.PerformRequest(deps, HandleResolveBookmark, http.MethodGet,
			"/api/v1/bookmarks/resolve",
			testutil.WithFakeUser(),
			testutil.WithRequestQueryParam("url", "https://unknown.example.com/page"))
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("current and history urls resolve to the same bookmark", func(t *testing.T) {
		_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)
		deps.Database().SetHistoryURLsEnabled(true)

		bookmark := testutil.GetValidBookmark()
		saved, err := deps.Database().SaveBookmarks(ctx, true, *bookmark)
		require.NoError(t, err)
		require.Len(t, saved, 1)

		oldURL := "https://old.example.com/page"
		require.NoError(t, deps.Domains().Bookmarks().AddBookmarkAlias(ctx, saved[0].ID, oldURL))

		// The current URL resolves.
		w := testutil.PerformRequest(deps, HandleResolveBookmark, http.MethodGet,
			"/api/v1/bookmarks/resolve",
			testutil.WithFakeUser(),
			testutil.WithRequestQueryParam("url", bookmark.URL))
		require.Equal(t, http.StatusOK, w.Code)

		// The history URL resolves to the exact same bookmark identity.
		w = testutil.PerformRequest(deps, HandleResolveBookmark, http.MethodGet,
			"/api/v1/bookmarks/resolve",
			testutil.WithFakeUser(),
			testutil.WithRequestQueryParam("url", oldURL))
		require.Equal(t, http.StatusOK, w.Code)

		response := testutil.NewTestResponseFromRecorder(w)
		response.AssertMessageJSONKeyValue(t, "ID", func(t *testing.T, value any) {
			require.EqualValues(t, saved[0].ID, value)
		})
	})
}

func TestHandleBookmarkAliasEndpoints(t *testing.T) {
	logger := logrus.New()
	ctx := context.Background()

	t.Run("add requires admin", func(t *testing.T) {
		_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)
		w := testutil.PerformRequest(deps, HandleAddBookmarkAlias, http.MethodPost,
			"/api/v1/bookmarks/1/aliases",
			testutil.WithFakeUser(),
			testutil.WithRequestPathValue("id", "1"),
			testutil.WithBody(`{"url":"https://old.example.com"}`))
		require.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("add rejected while feature disabled", func(t *testing.T) {
		_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)
		w := testutil.PerformRequest(deps, HandleAddBookmarkAlias, http.MethodPost,
			"/api/v1/bookmarks/1/aliases",
			testutil.WithFakeAdmin(),
			testutil.WithRequestPathValue("id", "1"),
			testutil.WithBody(`{"url":"https://old.example.com"}`))
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("add, duplicate submission and conflict", func(t *testing.T) {
		_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)
		deps.Database().SetHistoryURLsEnabled(true)

		first := testutil.GetValidBookmark()
		savedFirst, err := deps.Database().SaveBookmarks(ctx, true, *first)
		require.NoError(t, err)

		second := testutil.GetValidBookmark()
		second.URL = "https://conflict.example.com/other"
		savedSecond, err := deps.Database().SaveBookmarks(ctx, true, *second)
		require.NoError(t, err)

		aliasURL := "https://old.example.com/page"

		// First association succeeds.
		w := testutil.PerformRequest(deps, HandleAddBookmarkAlias, http.MethodPost,
			"/api/v1/bookmarks/"+itoa(savedFirst[0].ID)+"/aliases",
			testutil.WithFakeAdmin(),
			testutil.WithRequestPathValue("id", itoa(savedFirst[0].ID)),
			testutil.WithBody(`{"url":"`+aliasURL+`"}`))
		require.Equal(t, http.StatusCreated, w.Code)

		// Repeated submission stays successful and creates no duplicate.
		w = testutil.PerformRequest(deps, HandleAddBookmarkAlias, http.MethodPost,
			"/api/v1/bookmarks/"+itoa(savedFirst[0].ID)+"/aliases",
			testutil.WithFakeAdmin(),
			testutil.WithRequestPathValue("id", itoa(savedFirst[0].ID)),
			testutil.WithBody(`{"url":"`+aliasURL+`"}`))
		require.Equal(t, http.StatusCreated, w.Code)

		aliases, err := deps.Database().GetBookmarkAliases(ctx, savedFirst[0].ID)
		require.NoError(t, err)
		require.Len(t, aliases[savedFirst[0].ID], 1)

		// Stealing an address owned by another bookmark is a 409.
		w = testutil.PerformRequest(deps, HandleAddBookmarkAlias, http.MethodPost,
			"/api/v1/bookmarks/"+itoa(savedSecond[0].ID)+"/aliases",
			testutil.WithFakeAdmin(),
			testutil.WithRequestPathValue("id", itoa(savedSecond[0].ID)),
			testutil.WithBody(`{"url":"`+aliasURL+`"}`))
		require.Equal(t, http.StatusConflict, w.Code)

		// An invalid payload is a 400.
		w = testutil.PerformRequest(deps, HandleAddBookmarkAlias, http.MethodPost,
			"/api/v1/bookmarks/"+itoa(savedFirst[0].ID)+"/aliases",
			testutil.WithFakeAdmin(),
			testutil.WithRequestPathValue("id", itoa(savedFirst[0].ID)),
			testutil.WithBody(`{"url":""}`))
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("delete removes the alias", func(t *testing.T) {
		_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)
		deps.Database().SetHistoryURLsEnabled(true)

		bookmark := testutil.GetValidBookmark()
		saved, err := deps.Database().SaveBookmarks(ctx, true, *bookmark)
		require.NoError(t, err)

		aliasURL := "https://old.example.com/page"
		require.NoError(t, deps.Domains().Bookmarks().AddBookmarkAlias(ctx, saved[0].ID, aliasURL))

		w := testutil.PerformRequest(deps, HandleDeleteBookmarkAlias, http.MethodDelete,
			"/api/v1/bookmarks/"+itoa(saved[0].ID)+"/aliases",
			testutil.WithFakeAdmin(),
			testutil.WithRequestPathValue("id", itoa(saved[0].ID)),
			testutil.WithBody(`{"url":"`+aliasURL+`"}`))
		require.Equal(t, http.StatusOK, w.Code)

		aliases, err := deps.Database().GetBookmarkAliases(ctx, saved[0].ID)
		require.NoError(t, err)
		require.Empty(t, aliases[saved[0].ID])
	})
}

func itoa(id int) string {
	return strconv.Itoa(id)
}
