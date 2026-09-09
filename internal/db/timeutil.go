package db

import (
	"database/sql"
	"time"
)

// nowString returns t formatted as an RFC3339 UTC string, the canonical
// timestamp representation used by every table (see ARCHITECTURE.md §5).
func nowString(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// parseTime parses a stored RFC3339 timestamp. An empty string yields the
// zero time so callers can treat "not set" uniformly.
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		// Tolerate plain RFC3339 (no fractional seconds) written elsewhere.
		if t2, err2 := time.Parse(time.RFC3339, s); err2 == nil {
			return t2.UTC()
		}
		return time.Time{}
	}
	return t.UTC()
}

// nullTime converts a nullable timestamp column into *time.Time.
func nullTime(ns sql.NullString) *time.Time {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	t := parseTime(ns.String)
	return &t
}
