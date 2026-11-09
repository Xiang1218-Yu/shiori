package domains_test

import (
	"context"
	"testing"

	"github.com/go-shiori/shiori/internal/model"
	"github.com/go-shiori/shiori/internal/testutil"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBookmarkDomainHistoryURLs(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)
	domain := deps.Domains().Bookmarks()

	saved, err := deps.Database().SaveBookmarks(ctx, true, *testutil.GetValidBookmark())
	require.NoError(t, err)
	bookmarkID := saved[0].ID

	t.Run("disabled by default", func(t *testing.T) {
		assert.False(t, domain.HistoryURLsEnabled())
		err := domain.AddBookmarkAlias(ctx, bookmarkID, "https://old.example.com/page")
		assert.ErrorIs(t, err, model.ErrBookmarkHistoryDisabled)
		err = domain.DeleteBookmarkAlias(ctx, bookmarkID, "https://old.example.com/page")
		assert.ErrorIs(t, err, model.ErrBookmarkHistoryDisabled)
	})

	t.Run("rejects invalid urls", func(t *testing.T) {
		deps.Database().SetHistoryURLsEnabled(true)

		for _, invalid := range []string{"", "not-a-url", "://missing-scheme", "https://"} {
			err := domain.AddBookmarkAlias(ctx, bookmarkID, invalid)
			assert.Error(t, err, "expected rejection for %q", invalid)
		}
	})

	t.Run("add resolve and remove lifecycle", func(t *testing.T) {
		aliasURL := "https://history.example.com/page"

		require.NoError(t, domain.AddBookmarkAlias(ctx, bookmarkID, aliasURL))

		// The history URL resolves to the stable bookmark identity.
		resolved, err := domain.ResolveBookmarkByURL(ctx, aliasURL)
		require.NoError(t, err)
		assert.Equal(t, bookmarkID, resolved.ID)
		assert.Contains(t, resolved.HistoryURLs, aliasURL)

		// Removing the alias makes the address unresolvable again.
		require.NoError(t, domain.DeleteBookmarkAlias(ctx, bookmarkID, aliasURL))
		_, err = domain.ResolveBookmarkByURL(ctx, aliasURL)
		assert.ErrorIs(t, err, model.ErrBookmarkNotFound)
	})

	t.Run("resolve unknown url", func(t *testing.T) {
		_, err := domain.ResolveBookmarkByURL(ctx, "https://unknown.example.com/page")
		assert.ErrorIs(t, err, model.ErrBookmarkNotFound)
	})

	t.Run("conflict with another bookmark", func(t *testing.T) {
		other := testutil.GetValidBookmark()
		other.URL = "https://other.example.com/page"
		savedOther, err := deps.Database().SaveBookmarks(ctx, true, *other)
		require.NoError(t, err)

		// The current URL of another bookmark cannot be aliased.
		err = domain.AddBookmarkAlias(ctx, bookmarkID, other.URL)
		assert.ErrorIs(t, err, model.ErrBookmarkAliasConflict)

		// An alias owned by the first bookmark cannot be stolen.
		require.NoError(t, domain.AddBookmarkAlias(ctx, bookmarkID, "https://shared.example.com/x"))
		err = domain.AddBookmarkAlias(ctx, savedOther[0].ID, "https://shared.example.com/x")
		assert.ErrorIs(t, err, model.ErrBookmarkAliasConflict)
	})
}
