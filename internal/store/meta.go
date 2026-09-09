package store

import (
	"database/sql"
	"errors"
)

// initMetaTable creates a small key-value table used for one-time migration
// markers (e.g. "sort_order backfilled"). Keeping it separate from the
// settings table avoids conflating schema-migration state with user settings.
func (s *TaskStore) initMetaTable() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`)
	return err
}

func (s *TaskStore) metaGet(key string) (string, error) {
	var val string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&val)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return val, err
}

func (s *TaskStore) metaSet(key, val string) error {
	_, err := s.db.Exec(`INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, val)
	return err
}
