package model

import "errors"

var (
	ErrBookmarkNotFound      = errors.New("bookmark not found")
	ErrBookmarkInvalidID     = errors.New("invalid bookmark ID")
	ErrBookmarkAlreadyExists = errors.New("bookmark already exists")
	ErrTagNotFound           = errors.New("tag not found")
	ErrTagAlreadyExists      = errors.New("tag already exists")

	// ErrBookmarkAliasConflict is returned when a URL is already associated
	// with another bookmark, either as its current URL or as a history URL.
	ErrBookmarkAliasConflict = errors.New("bookmark URL alias is already used by another bookmark")
	// ErrBookmarkHistoryDisabled is returned when history URL features are
	// used while the BOOKMARK_HISTORY_URLS_ENABLED option is turned off.
	ErrBookmarkHistoryDisabled = errors.New("bookmark history URLs are disabled")

	ErrUnauthorized       = errors.New("unauthorized user")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrNotFound           = errors.New("not found")
	ErrAlreadyExists      = errors.New("already exists")
	ErrPipelineStop       = errors.New("pipeline stop")
)
