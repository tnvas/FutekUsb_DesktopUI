// Package storage persists application configuration and recorded sensor
// data in a single SQLite database.
package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// TimeFormat is the timestamp layout used for readings. Kept identical to
// previous releases so existing databases remain readable.
const TimeFormat = "2006-01-02 15:04:05.999999999"

// DB wraps the SQLite connection.
type DB struct {
	sql  *sql.DB
	path string
}

const schema = `
CREATE TABLE IF NOT EXISTS recordings (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS readings (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	recording_id INTEGER NOT NULL,
	timestamp TEXT NOT NULL,
	value REAL NOT NULL,
	unit TEXT NOT NULL,
	FOREIGN KEY (recording_id) REFERENCES recordings (id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS config (
	id INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1),
	usb_name TEXT,
	usb_baudrate TEXT,
	usb_target TEXT,
	usb_serial TEXT,
	usb_product_id TEXT,
	usb_vendor_id TEXT,
	decimal_point TEXT,
	fullscale_load TEXT,
	fullscale_value TEXT,
	offset_value TEXT,
	offset_load TEXT
);
CREATE INDEX IF NOT EXISTS idx_readings_recording ON readings(recording_id);
`

// Columns added after the original release; applied best-effort so old
// databases upgrade in place. calibration_unit is a legacy column kept only
// so old databases still open cleanly — no longer read or written (the
// device streams already-calibrated, unit-labelled values; see device_unit).
var migrations = []string{
	`ALTER TABLE config ADD COLUMN calibration_unit TEXT`,
	`ALTER TABLE config ADD COLUMN display_unit TEXT`,
	`ALTER TABLE config ADD COLUMN display_decimals TEXT`,
	`ALTER TABLE config ADD COLUMN sample_rate TEXT`,
	`ALTER TABLE config ADD COLUMN device_unit TEXT`,
}

// DefaultPath resolves where the database lives. Priority:
//  1. DB_PATH environment variable
//  2. legacy ./db/database.sqlite next to the executable's working dir
//  3. the per-user config directory (created on demand)
func DefaultPath(appName string) string {
	if p := os.Getenv("DB_PATH"); p != "" {
		return p
	}
	legacy := filepath.Join("db", "database.sqlite")
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return legacy
	}
	return filepath.Join(base, appName, "database.sqlite")
}

// Open opens (creating if necessary) the database at path.
func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	dsn := fmt.Sprintf("file:%s?_fk=1&_journal_mode=WAL&_busy_timeout=5000", path)
	sqldb, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database %s: %w", path, err)
	}
	// A single connection avoids SQLite lock contention entirely.
	sqldb.SetMaxOpenConns(1)

	if _, err := sqldb.Exec(schema); err != nil {
		sqldb.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	for _, m := range migrations {
		if _, err := sqldb.Exec(m); err != nil && !isDuplicateColumn(err) {
			sqldb.Close()
			return nil, fmt.Errorf("migrate schema: %w", err)
		}
	}
	return &DB{sql: sqldb, path: path}, nil
}

// Path returns the on-disk location of the database.
func (d *DB) Path() string { return d.path }

// Close closes the underlying connection.
func (d *DB) Close() error { return d.sql.Close() }

func isDuplicateColumn(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate column name")
}
