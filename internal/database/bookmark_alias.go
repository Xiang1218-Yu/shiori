package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strings"

	"github.com/go-shiori/shiori/internal/model"
	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

// aliasRow maps a row of the bookmark_alias table.
type aliasRow struct {
	Alias      string `db:"alias"`
	AliasHash  string `db:"alias_hash"`
	BookmarkID int    `db:"bookmark_id"`
}

// bookmarkAliasHash returns the stable hash used as primary key for an alias.
// Hashing the URL keeps the schema portable across database engines and
// avoids index length limitations on very long URLs.
func bookmarkAliasHash(alias string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(alias)))
	return hex.EncodeToString(sum[:])
}

// intsToAny expands an int slice into the variadic form expected by
// go-sqlbuilder's In method.
func intsToAny(ids []int) []any {
	result := make([]any, len(ids))
	for i, id := range ids {
		result[i] = id
	}
	return result
}

// HistoryURLsEnabled reports whether history URL aliases are active.
func (db *dbbase) HistoryURLsEnabled() bool {
	return db.historyURLsEnabled
}

// SetHistoryURLsEnabled toggles history URL aliases at runtime.
func (db *dbbase) SetHistoryURLsEnabled(enabled bool) {
	db.historyURLsEnabled = enabled
}

// ResolveBookmarkIDByURL maps a URL to the stable ID of a bookmark.
// Current URLs are always checked first; history URLs are only considered
// when the feature is enabled, preserving the legacy URL lookup behavior.
func (db *dbbase) ResolveBookmarkIDByURL(ctx context.Context, url string) (int, bool, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return 0, false, nil
	}

	sb := sqlbuilder.NewSelectBuilder()
	sb.SetFlavor(db.flavor)
	sb.Select("id").From("bookmark").Where(sb.EQ("url", url))
	query, args := sb.Build()

	var id int
	err := db.reader.GetContext(ctx, &id, query, args...)
	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}

	if !db.historyURLsEnabled {
		return 0, false, nil
	}

	ab := sqlbuilder.NewSelectBuilder()
	ab.SetFlavor(db.flavor)
	ab.Select("bookmark_id").From("bookmark_alias").Where(ab.EQ("alias_hash", bookmarkAliasHash(url)))
	query, args = ab.Build()

	err = db.reader.GetContext(ctx, &id, query, args...)
	if err == nil {
		return id, true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return 0, false, err
}

// AddBookmarkAlias associates a history URL with a bookmark.
// Submitting the same URL again is a no-op so it cannot create duplicate
// associations. A URL already owned by another bookmark is explicitly
// rejected with model.ErrBookmarkAliasConflict.
func (db *dbbase) AddBookmarkAlias(ctx context.Context, bookmarkID int, alias string) error {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return errors.New("bookmark alias URL cannot be empty")
	}
	if !db.historyURLsEnabled {
		return model.ErrBookmarkHistoryDisabled
	}

	exists, err := db.BookmarkExists(ctx, bookmarkID)
	if err != nil {
		return err
	}
	if !exists {
		return model.ErrBookmarkNotFound
	}

	return db.withTx(ctx, func(tx *sqlx.Tx) error {
		return db.insertBookmarkAlias(ctx, tx, bookmarkID, alias)
	})
}

// DeleteBookmarkAlias removes a history URL association. The deletion is
// scoped to the given bookmark so an alias owned by another bookmark can
// never be removed through this call.
func (db *dbbase) DeleteBookmarkAlias(ctx context.Context, bookmarkID int, alias string) error {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return errors.New("bookmark alias URL cannot be empty")
	}
	if !db.historyURLsEnabled {
		return model.ErrBookmarkHistoryDisabled
	}

	db2 := sqlbuilder.NewDeleteBuilder()
	db2.SetFlavor(db.flavor)
	db2.DeleteFrom("bookmark_alias").
		Where(db2.EQ("alias_hash", bookmarkAliasHash(alias))).
		Where(db2.EQ("bookmark_id", bookmarkID))
	query, args := db2.Build()

	_, err := db.writer.ExecContext(ctx, query, args...)
	return err
}

// GetBookmarkAliases returns the history URLs of the requested bookmarks,
// keyed by bookmark ID and ordered from oldest to newest.
func (db *dbbase) GetBookmarkAliases(ctx context.Context, bookmarkIDs ...int) (map[int][]string, error) {
	result := make(map[int][]string)
	if len(bookmarkIDs) == 0 {
		return result, nil
	}

	sb := sqlbuilder.NewSelectBuilder()
	sb.SetFlavor(db.flavor)
	sb.Select("bookmark_id", "alias").From("bookmark_alias").
		Where(sb.In("bookmark_id", intsToAny(bookmarkIDs)...)).
		OrderBy("bookmark_id ASC, created_at ASC")
	query, args := sb.Build()

	rows := []aliasRow{}
	if err := db.reader.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}

	for _, row := range rows {
		result[row.BookmarkID] = append(result[row.BookmarkID], row.Alias)
	}
	return result, nil
}

// insertBookmarkAlias is the transaction-safe core of AddBookmarkAlias.
// It is also reused when a bookmark URL changes and the previous URL has to
// be kept as history.
func (db *dbbase) insertBookmarkAlias(ctx context.Context, ext sqlx.ExtContext, bookmarkID int, alias string) error {
	hash := bookmarkAliasHash(alias)

	// The alias may already be recorded (repeated submission).
	sb := sqlbuilder.NewSelectBuilder()
	sb.SetFlavor(db.flavor)
	sb.Select("bookmark_id").From("bookmark_alias").Where(sb.EQ("alias_hash", hash))
	query, args := sb.Build()

	var existingID int
	err := sqlx.GetContext(ctx, ext, &existingID, query, args...)
	if err == nil {
		if existingID == bookmarkID {
			// Same bookmark: idempotent no-op, no duplicate association.
			return nil
		}
		return model.ErrBookmarkAliasConflict
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	// The URL may be the current URL of another bookmark.
	ub := sqlbuilder.NewSelectBuilder()
	ub.SetFlavor(db.flavor)
	ub.Select("id").From("bookmark").Where(ub.EQ("url", alias))
	query, args = ub.Build()

	err = sqlx.GetContext(ctx, ext, &existingID, query, args...)
	if err == nil {
		if existingID == bookmarkID {
			// It is this bookmark's current URL, so the association already
			// exists implicitly and nothing needs to be stored.
			return nil
		}
		return model.ErrBookmarkAliasConflict
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	ib := sqlbuilder.NewInsertBuilder()
	ib.SetFlavor(db.flavor)
	ib.InsertInto("bookmark_alias").
		Cols("alias", "alias_hash", "bookmark_id").
		Values(alias, hash, bookmarkID)
	query, args = ib.Build()

	if _, err := ext.ExecContext(ctx, query, args...); err != nil {
		return err
	}
	return nil
}

// rejectURLUsedAsAlias fails when url is already recorded as a history URL
// of any bookmark. It guards bookmark creation against hijacking an address
// that must keep resolving to an existing bookmark.
func (db *dbbase) rejectURLUsedAsAlias(ctx context.Context, ext sqlx.ExtContext, url string) error {
	sb := sqlbuilder.NewSelectBuilder()
	sb.SetFlavor(db.flavor)
	sb.Select("bookmark_id").From("bookmark_alias").
		Where(sb.EQ("alias_hash", bookmarkAliasHash(url)))
	query, args := sb.Build()

	var id int
	err := sqlx.GetContext(ctx, ext, &id, query, args...)
	if err == nil {
		return model.ErrBookmarkAliasConflict
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}

// recordBookmarkURLHistory maintains aliases when a bookmark URL changes.
// It rejects new URLs already owned by another bookmark, removes the alias
// when the URL is changed back to a previous address, and records the old
// URL as history. It must run inside the bookmark update transaction.
func (db *dbbase) recordBookmarkURLHistory(ctx context.Context, ext sqlx.ExtContext, bookmarkID int, oldURL, newURL string) error {
	oldURL = strings.TrimSpace(oldURL)
	newURL = strings.TrimSpace(newURL)
	if oldURL == "" || oldURL == newURL {
		return nil
	}

	hash := bookmarkAliasHash(newURL)

	// Check whether the new URL is already a history URL of some bookmark.
	sb := sqlbuilder.NewSelectBuilder()
	sb.SetFlavor(db.flavor)
	sb.Select("bookmark_id").From("bookmark_alias").Where(sb.EQ("alias_hash", hash))
	query, args := sb.Build()

	var ownerID int
	err := sqlx.GetContext(ctx, ext, &ownerID, query, args...)
	switch {
	case err == nil && ownerID != bookmarkID:
		// Explicitly reject conflicts between new and historical addresses.
		return model.ErrBookmarkAliasConflict
	case err == nil && ownerID == bookmarkID:
		// Changing back to a previous address: drop that alias so no orphan
		// alias remains pointing at the bookmark's current URL.
		del := sqlbuilder.NewDeleteBuilder()
		del.SetFlavor(db.flavor)
		del.DeleteFrom("bookmark_alias").Where(del.EQ("alias_hash", hash))
		delQuery, delArgs := del.Build()
		if _, err := ext.ExecContext(ctx, delQuery, delArgs...); err != nil {
			return err
		}
	case !errors.Is(err, sql.ErrNoRows):
		return err
	}

	// Keep the previous URL reachable as a history URL. The insert is
	// idempotent, so saving the same change twice cannot create duplicates.
	return db.insertBookmarkAlias(ctx, ext, bookmarkID, oldURL)
}

// deleteBookmarkAliases removes every alias of the given bookmarks. It runs
// inside the bookmark deletion transaction so orphan aliases cannot survive
// a bookmark removal, complementing the ON DELETE CASCADE foreign key.
func (db *dbbase) deleteBookmarkAliases(ctx context.Context, ext sqlx.ExtContext, ids []int) error {
	db2 := sqlbuilder.NewDeleteBuilder()
	db2.SetFlavor(db.flavor)
	db2.DeleteFrom("bookmark_alias")

	if len(ids) > 0 {
		db2.Where(db2.In("bookmark_id", intsToAny(ids)...))
	}

	query, args := db2.Build()
	_, err := ext.ExecContext(ctx, query, args...)
	return err
}

// enrichBookmarksWithAliases populates the HistoryURLs field of every
// bookmark DTO. When the feature is disabled the field is left untouched.
func (db *dbbase) enrichBookmarksWithAliases(ctx context.Context, bookmarks []model.BookmarkDTO) error {
	if !db.historyURLsEnabled || len(bookmarks) == 0 {
		return nil
	}

	ids := make([]int, 0, len(bookmarks))
	for _, bookmark := range bookmarks {
		ids = append(ids, bookmark.ID)
	}

	aliases, err := db.GetBookmarkAliases(ctx, ids...)
	if err != nil {
		return err
	}

	for i := range bookmarks {
		if urls, ok := aliases[bookmarks[i].ID]; ok {
			bookmarks[i].HistoryURLs = urls
		} else {
			bookmarks[i].HistoryURLs = []string{}
		}
	}
	return nil
}

// aliasKeywordSubquery builds a subquery matching bookmarks whose history
// URL contains keyword. It returns nil when the feature is disabled.
func (db *dbbase) aliasKeywordSubquery(keyword string) *sqlbuilder.SelectBuilder {
	if !db.historyURLsEnabled || keyword == "" {
		return nil
	}

	sub := sqlbuilder.NewSelectBuilder()
	sub.SetFlavor(db.flavor)
	sub.Select("bookmark_id").From("bookmark_alias").
		Where(sub.Like("alias", "%"+keyword+"%"))
	return sub
}
