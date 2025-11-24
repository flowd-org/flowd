package coredb

import (
	"errors"
	"strings"

	sqlite3 "modernc.org/sqlite/lib"
)

// codeError matches modernc.org/sqlite error types exposed by the driver.
type codeError interface {
	Code() int
}

// IsQuotaExceeded reports whether the supplied error indicates that the
// configured Core DB storage quota has been exhausted. This covers both
// explicit journal limits (ErrJournalQuotaExceeded) and SQLite's SQLITE_FULL
// result when the max_page_count boundary is reached. A string fallback handles
// filesystem-level quota messages surfaced by SQLite.
func IsQuotaExceeded(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrJournalQuotaExceeded) {
		return true
	}
	var coder codeError
	if errors.As(err, &coder) {
		if coder.Code() == int(sqlite3.SQLITE_FULL) {
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "database or disk is full"):
		return true
	case strings.Contains(msg, "quota") && strings.Contains(msg, "exceeded"):
		return true
	default:
		return false
	}
}
