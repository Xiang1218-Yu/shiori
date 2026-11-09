package database

import (
	"context"
	"testing"

	"github.com/go-shiori/shiori/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createBookmarkForAliasTest saves a bookmark with the given URL and returns
// its stable ID.
func createBookmarkForAliasTest(t *testing.T, ctx context.Context, db model.DB, url string) model.BookmarkDTO {
	t.Helper()

	result, err := db.SaveBookmarks(ctx, true, model.BookmarkDTO{
		Bookmark: model.Bookmark{
			URL:   url,
			Title: url,
		},
	})
	require.NoError(t, err, "Save bookmarks must not fail")
	require.Len(t, result, 1)
	return result[0]
}

// testHistoryURLsDisabledByDefault guarantees the legacy URL behavior is
// untouched while the feature flag stays off.
func testHistoryURLsDisabledByDefault(t *testing.T, db model.DB) {
	ctx := context.TODO()

	assert.False(t, db.HistoryURLsEnabled(), "history URLs must be disabled by default")

	book := createBookmarkForAliasTest(t, ctx, db, "https://example.com/old")

	// Changing the URL must not record any history while disabled.
	book.URL = "https://example.com/new"
	_, err := db.SaveBookmarks(ctx, false, book)
	require.NoError(t, err)

	_, found, err := db.ResolveBookmarkIDByURL(ctx, "https://example.com/old")
	require.NoError(t, err)
	assert.False(t, found, "old URL must not resolve while history URLs are disabled")

	_, exists, err := db.GetBookmark(ctx, 0, "https://example.com/old")
	require.NoError(t, err)
	assert.False(t, exists, "GetBookmark must keep matching current URLs only while disabled")

	aliases, err := db.GetBookmarkAliases(ctx, book.ID)
	require.NoError(t, err)
	assert.Empty(t, aliases[book.ID], "no alias rows must be written while disabled")

	err = db.AddBookmarkAlias(ctx, book.ID, "https://example.com/another")
	assert.ErrorIs(t, err, model.ErrBookmarkHistoryDisabled)
}

// testBookmarkAliasRecordsOldURLOnUpdate covers the core lifecycle: an old
// URL keeps resolving to the same stable bookmark identity after the URL
// changes.
func testBookmarkAliasRecordsOldURLOnUpdate(t *testing.T, db model.DB) {
	ctx := context.TODO()
	db.SetHistoryURLsEnabled(true)

	book := createBookmarkForAliasTest(t, ctx, db, "https://example.com/v1/page")

	book.URL = "https://example.com/v2/page"
	_, err := db.SaveBookmarks(ctx, false, book)
	require.NoError(t, err)

	// The old URL resolves to the exact same bookmark ID.
	id, found, err := db.ResolveBookmarkIDByURL(ctx, "https://example.com/v1/page")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, book.ID, id)

	// GetBookmark by old URL returns the same bookmark, now exposing the
	// history URL in its DTO.
	resolved, exists, err := db.GetBookmark(ctx, 0, "https://example.com/v1/page")
	require.NoError(t, err)
	require.True(t, exists)
	assert.Equal(t, book.ID, resolved.ID)
	assert.Equal(t, "https://example.com/v2/page", resolved.URL)
	assert.Equal(t, []string{"https://example.com/v1/page"}, resolved.HistoryURLs)

	// Current URL still takes precedence.
	current, exists, err := db.GetBookmark(ctx, 0, "https://example.com/v2/page")
	require.NoError(t, err)
	require.True(t, exists)
	assert.Equal(t, book.ID, current.ID)

	// Keyword search finds the bookmark through its history URL.
	results, err := db.GetBookmarks(ctx, model.DBGetBookmarksOptions{Keyword: "v1/page"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, book.ID, results[0].ID)

	count, err := db.GetBookmarksCount(ctx, model.DBGetBookmarksOptions{Keyword: "v1/page"})
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Listing aliases returns the old URL.
	aliases, err := db.GetBookmarkAliases(ctx, book.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"https://example.com/v1/page"}, aliases[book.ID])
}

// testBookmarkAliasIdempotent ensures repeated submissions never create
// duplicate associations.
func testBookmarkAliasIdempotent(t *testing.T, db model.DB) {
	ctx := context.TODO()
	db.SetHistoryURLsEnabled(true)

	book := createBookmarkForAliasTest(t, ctx, db, "https://example.com/current")
	alias := "https://example.com/previous"

	require.NoError(t, db.AddBookmarkAlias(ctx, book.ID, alias))
	// Submitting the exact same address again must be a no-op.
	require.NoError(t, db.AddBookmarkAlias(ctx, book.ID, alias))
	// Trimming is applied consistently.
	require.NoError(t, db.AddBookmarkAlias(ctx, book.ID, "  "+alias+"  "))

	aliases, err := db.GetBookmarkAliases(ctx, book.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{alias}, aliases[book.ID])
}

// testBookmarkAliasConflictRejected verifies explicit rejection whenever a
// URL is already owned by another bookmark, as current URL or history URL.
func testBookmarkAliasConflictRejected(t *testing.T, db model.DB) {
	ctx := context.TODO()
	db.SetHistoryURLsEnabled(true)

	first := createBookmarkForAliasTest(t, ctx, db, "https://a.example.com/page")
	second := createBookmarkForAliasTest(t, ctx, db, "https://b.example.com/page")

	// Record an alias for the first bookmark.
	firstAlias := "https://old.example.com/page"
	require.NoError(t, db.AddBookmarkAlias(ctx, first.ID, firstAlias))

	// Another bookmark cannot steal that alias.
	err := db.AddBookmarkAlias(ctx, second.ID, firstAlias)
	assert.ErrorIs(t, err, model.ErrBookmarkAliasConflict)

	// A bookmark cannot take a URL currently used by another bookmark.
	err = db.AddBookmarkAlias(ctx, first.ID, "https://b.example.com/page")
	assert.ErrorIs(t, err, model.ErrBookmarkAliasConflict)

	// Updating a bookmark URL to an alias owned by another bookmark fails.
	second.URL = firstAlias
	_, err = db.SaveBookmarks(ctx, false, second)
	assert.ErrorIs(t, err, model.ErrBookmarkAliasConflict)

	// Creating a bookmark with an aliased URL fails too.
	_, err = db.SaveBookmarks(ctx, true, model.BookmarkDTO{
		Bookmark: model.Bookmark{URL: firstAlias, Title: "conflict"},
	})
	assert.ErrorIs(t, err, model.ErrBookmarkAliasConflict)

	// The first bookmark keeps its single alias after the failed attempts.
	aliases, err := db.GetBookmarkAliases(ctx, first.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{firstAlias}, aliases[first.ID])
}

// testBookmarkAliasChangeBackRemovesAlias ensures changing the URL back to a
// previous address removes that alias instead of leaving an orphan row that
// points at the bookmark's current URL.
func testBookmarkAliasChangeBackRemovesAlias(t *testing.T, db model.DB) {
	ctx := context.TODO()
	db.SetHistoryURLsEnabled(true)

	book := createBookmarkForAliasTest(t, ctx, db, "https://example.com/v1")

	book.URL = "https://example.com/v2"
	_, err := db.SaveBookmarks(ctx, false, book)
	require.NoError(t, err)

	aliases, err := db.GetBookmarkAliases(ctx, book.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"https://example.com/v1"}, aliases[book.ID])

	// Change back to the previous address.
	book.URL = "https://example.com/v1"
	_, err = db.SaveBookmarks(ctx, false, book)
	require.NoError(t, err)

	aliases, err = db.GetBookmarkAliases(ctx, book.ID)
	require.NoError(t, err)
	// The consumed v1 alias is gone (no orphan pointing at the current URL)
	// while the address we just left, v2, becomes history.
	assert.Equal(t, []string{"https://example.com/v2"}, aliases[book.ID],
		"changing back must remove the consumed alias without leaving orphans")
	assert.NotContains(t, aliases[book.ID], "https://example.com/v1",
		"no alias may duplicate the bookmark's current URL")

	// Both addresses still resolve to the same stable identity.
	id, found, err := db.ResolveBookmarkIDByURL(ctx, "https://example.com/v1")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, book.ID, id)

	id, found, err = db.ResolveBookmarkIDByURL(ctx, "https://example.com/v2")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, book.ID, id)
}

// testBookmarkAliasDeleteCleansUp verifies that deleting a bookmark never
// leaves orphan aliases behind.
func testBookmarkAliasDeleteCleansUp(t *testing.T, db model.DB) {
	ctx := context.TODO()
	db.SetHistoryURLsEnabled(true)

	book := createBookmarkForAliasTest(t, ctx, db, "https://example.com/current")
	require.NoError(t, db.AddBookmarkAlias(ctx, book.ID, "https://example.com/old-1"))
	require.NoError(t, db.AddBookmarkAlias(ctx, book.ID, "https://example.com/old-2"))

	require.NoError(t, db.DeleteBookmarks(ctx, book.ID))

	aliases, err := db.GetBookmarkAliases(ctx, book.ID)
	require.NoError(t, err)
	assert.Empty(t, aliases[book.ID], "aliases must be removed with the bookmark")

	_, found, err := db.ResolveBookmarkIDByURL(ctx, "https://example.com/old-1")
	require.NoError(t, err)
	assert.False(t, found, "old URL must not resolve after bookmark deletion")
}

// testBookmarkAliasManualRemove verifies the explicit alias removal path.
func testBookmarkAliasManualRemove(t *testing.T, db model.DB) {
	ctx := context.TODO()
	db.SetHistoryURLsEnabled(true)

	book := createBookmarkForAliasTest(t, ctx, db, "https://example.com/current")
	require.NoError(t, db.AddBookmarkAlias(ctx, book.ID, "https://example.com/old"))

	require.NoError(t, db.DeleteBookmarkAlias(ctx, book.ID, "https://example.com/old"))

	aliases, err := db.GetBookmarkAliases(ctx, book.ID)
	require.NoError(t, err)
	assert.Empty(t, aliases[book.ID])

	// Removing a non-existent or foreign-scoped alias is a harmless no-op.
	assert.NoError(t, db.DeleteBookmarkAlias(ctx, book.ID, "https://example.com/old"))
}

// testBookmarkAliasMultipleHistoryURLs verifies a bookmark can keep several
// historical addresses and each resolves to the same identity.
func testBookmarkAliasMultipleHistoryURLs(t *testing.T, db model.DB) {
	ctx := context.TODO()
	db.SetHistoryURLsEnabled(true)

	book := createBookmarkForAliasTest(t, ctx, db, "https://example.com/v3")

	historyURLs := []string{
		"https://example.com/v1",
		"https://example.com/v2",
	}
	for _, alias := range historyURLs {
		require.NoError(t, db.AddBookmarkAlias(ctx, book.ID, alias))
	}

	for _, alias := range historyURLs {
		id, found, err := db.ResolveBookmarkIDByURL(ctx, alias)
		require.NoError(t, err)
		require.True(t, found)
		assert.Equal(t, book.ID, id)
	}

	aliases, err := db.GetBookmarkAliases(ctx, book.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, historyURLs, aliases[book.ID])
}
