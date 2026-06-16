package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Entry struct {
	ID        int64
	Timestamp time.Time
	Cmd       string
	CWD       string
	RC        int
	Duration  int64
	Session   string
	Metadata  map[string]any
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := initSchema(db); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func initSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp INTEGER NOT NULL,
		cmd TEXT NOT NULL,
		cwd TEXT NOT NULL,
		rc INTEGER NOT NULL,
		duration_ms INTEGER NOT NULL,
		session TEXT NOT NULL,
		metadata TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_history_timestamp ON history(timestamp);
	CREATE INDEX IF NOT EXISTS idx_history_session ON history(session);
	CREATE INDEX IF NOT EXISTS idx_history_cmd ON history(cmd);
	`
	_, err := db.Exec(schema)
	return err
}

func (s *Store) Add(ctx context.Context, e *Entry) error {
	metadata, _ := json.Marshal(e.Metadata)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO history (timestamp, cmd, cwd, rc, duration_ms, session, metadata)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.Timestamp.UnixMilli(), e.Cmd, e.CWD, e.RC, e.Duration, e.Session, string(metadata))
	return err
}

func (s *Store) Recent(ctx context.Context, limit int) ([]*Entry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, timestamp, cmd, cwd, rc, duration_ms, session, metadata
		 FROM history ORDER BY timestamp DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*Entry
	for rows.Next() {
		var e Entry
		var ts int64
		var meta string
		if err := rows.Scan(&e.ID, &ts, &e.Cmd, &e.CWD, &e.RC, &e.Duration, &e.Session, &meta); err != nil {
			return nil, err
		}
		e.Timestamp = time.UnixMilli(ts)
		json.Unmarshal([]byte(meta), &e.Metadata)
		entries = append(entries, &e)
	}
	return entries, rows.Err()
}

func (s *Store) Search(ctx context.Context, query string, limit int) ([]*Entry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, timestamp, cmd, cwd, rc, duration_ms, session, metadata
		 FROM history WHERE cmd LIKE ? ORDER BY timestamp DESC LIMIT ?`,
		"%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*Entry
	for rows.Next() {
		var e Entry
		var ts int64
		var meta string
		if err := rows.Scan(&e.ID, &ts, &e.Cmd, &e.CWD, &e.RC, &e.Duration, &e.Session, &meta); err != nil {
			return nil, err
		}
		e.Timestamp = time.UnixMilli(ts)
		json.Unmarshal([]byte(meta), &e.Metadata)
		entries = append(entries, &e)
	}
	return entries, rows.Err()
}

func (s *Store) Close() error {
	return s.db.Close()
}