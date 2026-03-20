package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate db: %w", err)
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS pools (
		name        TEXT PRIMARY KEY,
		description TEXT NOT NULL DEFAULT '',
		created_at  INTEGER NOT NULL DEFAULT (unixepoch())
	);

	CREATE TABLE IF NOT EXISTS nodes (
		name           TEXT PRIMARY KEY,
		pool           TEXT NOT NULL,
		ip             TEXT NOT NULL,
		status         TEXT NOT NULL DEFAULT 'pending',
		fail_count     INTEGER NOT NULL DEFAULT 0,
		last_check_at  INTEGER,
		last_online_at INTEGER,
		created_at     INTEGER NOT NULL DEFAULT (unixepoch()),
		FOREIGN KEY (pool) REFERENCES pools(name)
	);

	CREATE TABLE IF NOT EXISTS events (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		node_name  TEXT NOT NULL,
		event_type TEXT NOT NULL,
		detail     TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL DEFAULT (unixepoch())
	);

	CREATE INDEX IF NOT EXISTS idx_nodes_pool ON nodes(pool);
	CREATE INDEX IF NOT EXISTS idx_nodes_status ON nodes(status);
	CREATE INDEX IF NOT EXISTS idx_events_node ON events(node_name);
	CREATE INDEX IF NOT EXISTS idx_events_created ON events(created_at);
	`
	_, err := db.Exec(schema)
	return err
}
