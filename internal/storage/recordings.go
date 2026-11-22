package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// Recording is one named capture session.
type Recording struct {
	ID        int
	Name      string
	CreatedAt time.Time
}

// RecordingSummary adds aggregate information for list views, computed in
// SQL so large recordings never need to be loaded just to show a list.
type RecordingSummary struct {
	Recording
	Count    int
	First    time.Time
	Last     time.Time
	Unit     string
}

// Duration returns the time span covered by the recording.
func (r RecordingSummary) Duration() time.Duration {
	if r.Count < 2 {
		return 0
	}
	return r.Last.Sub(r.First)
}

// Reading is a single stored sensor value.
type Reading struct {
	ID          int
	RecordingID int
	Timestamp   time.Time
	Value       float64
	Unit        string
}

// CreateRecording inserts a new named recording session.
func (d *DB) CreateRecording(name string) (Recording, error) {
	now := time.Now()
	res, err := d.sql.Exec(`INSERT INTO recordings (name, created_at) VALUES (?, ?)`,
		name, now.Format(time.RFC3339))
	if err != nil {
		return Recording{}, fmt.Errorf("create recording %q: %w", name, err)
	}
	id, _ := res.LastInsertId()
	return Recording{ID: int(id), Name: name, CreatedAt: now}, nil
}

// DeleteRecording removes a recording and, via cascade, all its readings.
func (d *DB) DeleteRecording(id int) error {
	if _, err := d.sql.Exec(`DELETE FROM recordings WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete recording %d: %w", id, err)
	}
	return nil
}

// RecordingNameExists reports whether a recording with this name exists.
func (d *DB) RecordingNameExists(name string) (bool, error) {
	var id int
	err := d.sql.QueryRow(`SELECT id FROM recordings WHERE name = ?`, name).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ListRecordingSummaries returns all recordings, newest first, with counts
// and time spans aggregated by the database.
func (d *DB) ListRecordingSummaries() ([]RecordingSummary, error) {
	rows, err := d.sql.Query(`
		SELECT r.id, r.name, r.created_at,
		       COUNT(v.id), COALESCE(MIN(v.timestamp),''),
		       COALESCE(MAX(v.timestamp),''), COALESCE(MAX(v.unit),'')
		FROM recordings r
		LEFT JOIN readings v ON v.recording_id = r.id
		GROUP BY r.id
		ORDER BY r.created_at DESC, r.id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list recordings: %w", err)
	}
	defer rows.Close()

	var out []RecordingSummary
	for rows.Next() {
		var s RecordingSummary
		var created, first, last string
		if err := rows.Scan(&s.ID, &s.Name, &created, &s.Count, &first, &last, &s.Unit); err != nil {
			return nil, fmt.Errorf("scan recording row: %w", err)
		}
		s.CreatedAt, _ = time.Parse(time.RFC3339, created)
		s.First = parseTimestamp(first)
		s.Last = parseTimestamp(last)
		out = append(out, s)
	}
	return out, rows.Err()
}

// Readings returns all readings of a recording ordered by time.
func (d *DB) Readings(recordingID int) ([]Reading, error) {
	rows, err := d.sql.Query(`SELECT id, recording_id, timestamp, value, unit
		FROM readings WHERE recording_id = ? ORDER BY timestamp ASC`, recordingID)
	if err != nil {
		return nil, fmt.Errorf("query readings for recording %d: %w", recordingID, err)
	}
	defer rows.Close()

	var out []Reading
	for rows.Next() {
		var r Reading
		var ts string
		if err := rows.Scan(&r.ID, &r.RecordingID, &ts, &r.Value, &r.Unit); err != nil {
			return nil, fmt.Errorf("scan reading row: %w", err)
		}
		r.Timestamp = parseTimestamp(ts)
		out = append(out, r)
	}
	return out, rows.Err()
}

// AppendReadings writes a batch of readings inside a single transaction.
func (d *DB) AppendReadings(readings []Reading) error {
	if len(readings) == 0 {
		return nil
	}
	tx, err := d.sql.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO readings (recording_id, timestamp, value, unit) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, r := range readings {
		if _, err := stmt.Exec(r.RecordingID, r.Timestamp.Format(TimeFormat), r.Value, r.Unit); err != nil {
			return fmt.Errorf("insert reading: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit batch: %w", err)
	}
	return nil
}

// parseTimestamp accepts both the current and legacy (RFC3339Nano) layouts.
// The current layout stores local wall time without a zone, so it is parsed
// back as local time.
func parseTimestamp(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.ParseInLocation(TimeFormat, s, time.Local); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	return time.Time{}
}
