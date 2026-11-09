package cmd

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/go-shiori/shiori/internal/model"
	"github.com/spf13/cobra"
)

func aliasCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alias",
		Short: "Manage the history URLs of bookmarks",
		Long: "Manage the history URLs (aliases) of bookmarks. " +
			"History URLs let old addresses keep resolving to the same bookmark. " +
			"This feature requires SHIORI_BOOKMARK_HISTORY_URLS_ENABLED=true.",
	}

	cmd.AddCommand(aliasAddCmd(), aliasRemoveCmd(), aliasListCmd())
	return cmd
}

func aliasAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <index> <url>",
		Short: "Associate a history URL with a bookmark",
		Args:  cobra.ExactArgs(2),
		Run:   aliasAddHandler,
	}
}

func aliasRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "remove <index> <url>",
		Aliases: []string{"rm"},
		Short:   "Remove a history URL from a bookmark",
		Args:    cobra.ExactArgs(2),
		Run:     aliasRemoveHandler,
	}
}

func aliasListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list [index]",
		Aliases: []string{"ls"},
		Short:   "List the history URLs of a bookmark, or of all bookmarks",
		Args:    cobra.MaximumNArgs(1),
		Run:     aliasListHandler,
	}
}

func parseAliasBookmarkID(arg string) (int, error) {
	id, err := strconv.Atoi(arg)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("%w: %s", errInvalidIndex, arg)
	}
	return id, nil
}

// printAliasError translates domain errors into clear CLI messages.
func printAliasError(action string, err error) {
	switch {
	case errors.Is(err, model.ErrBookmarkHistoryDisabled):
		cError.Println("History URLs are disabled. Set SHIORI_BOOKMARK_HISTORY_URLS_ENABLED=true to use this command.")
	case errors.Is(err, model.ErrBookmarkAliasConflict):
		cError.Printf("Cannot %s alias: the URL is already used by another bookmark\n", action)
	case errors.Is(err, model.ErrBookmarkNotFound):
		cError.Println("Bookmark not found")
	default:
		cError.Printf("Failed to %s alias: %v\n", action, err)
	}
	os.Exit(1)
}

func aliasAddHandler(cmd *cobra.Command, args []string) {
	_, deps := initShiori(cmd.Context(), cmd)

	id, err := parseAliasBookmarkID(args[0])
	if err != nil {
		cError.Println(err)
		os.Exit(1)
	}

	if err := deps.Domains().Bookmarks().AddBookmarkAlias(cmd.Context(), id, args[1]); err != nil {
		printAliasError("add", err)
	}

	cInfo.Printf("History URL %q is now associated with bookmark %d\n", args[1], id)
}

func aliasRemoveHandler(cmd *cobra.Command, args []string) {
	_, deps := initShiori(cmd.Context(), cmd)

	id, err := parseAliasBookmarkID(args[0])
	if err != nil {
		cError.Println(err)
		os.Exit(1)
	}

	if err := deps.Domains().Bookmarks().DeleteBookmarkAlias(cmd.Context(), id, args[1]); err != nil {
		printAliasError("remove", err)
	}

	cInfo.Printf("History URL %q has been removed from bookmark %d\n", args[1], id)
}

func aliasListHandler(cmd *cobra.Command, args []string) {
	_, deps := initShiori(cmd.Context(), cmd)

	ctx := cmd.Context()
	db := deps.Database()

	if !db.HistoryURLsEnabled() {
		cError.Println("History URLs are disabled. Set SHIORI_BOOKMARK_HISTORY_URLS_ENABLED=true to use this command.")
		os.Exit(1)
	}

	// A specific bookmark was requested.
	if len(args) == 1 {
		id, err := parseAliasBookmarkID(args[0])
		if err != nil {
			cError.Println(err)
			os.Exit(1)
		}

		aliases, err := db.GetBookmarkAliases(ctx, id)
		if err != nil {
			cError.Printf("Failed to list aliases: %v\n", err)
			os.Exit(1)
		}

		for _, alias := range aliases[id] {
			fmt.Println(alias)
		}
		return
	}

	// No argument: list every bookmark that has at least one history URL.
	bookmarks, err := db.GetBookmarks(ctx, model.DBGetBookmarksOptions{})
	if err != nil {
		cError.Printf("Failed to get bookmarks: %v\n", err)
		os.Exit(1)
	}

	ids := make([]int, 0, len(bookmarks))
	for _, bookmark := range bookmarks {
		ids = append(ids, bookmark.ID)
	}

	aliases, err := db.GetBookmarkAliases(ctx, ids...)
	if err != nil {
		cError.Printf("Failed to list aliases: %v\n", err)
		os.Exit(1)
	}

	for _, bookmark := range bookmarks {
		urls := aliases[bookmark.ID]
		if len(urls) == 0 {
			continue
		}

		cIndex.Printf("%d. ", bookmark.ID)
		cTitle.Println(bookmark.Title)
		for _, alias := range urls {
			cSymbol.Print("  ~ ")
			cURL.Println(alias)
		}
		fmt.Println()
	}
}
