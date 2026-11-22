package storage

import (
	"path/filepath"
	"testing"
	"time"

	"futek_usb220/internal/physics"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestConfigRoundTrip(t *testing.T) {
	db := openTestDB(t)

	// No stored config -> defaults.
	cfg := db.LoadConfig()
	if cfg.SampleRateSPS != 4800 || cfg.DisplayUnit != physics.N {
		t.Errorf("unexpected defaults: %+v", cfg)
	}

	cfg.PreferredPort = "/dev/ttyUSB7"
	cfg.DeviceUnit = physics.KG
	cfg.SampleRateSPS = 600
	cfg.DisplayUnit = physics.KG
	cfg.DisplayDecimals = 4
	if err := db.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}

	got := db.LoadConfig()
	if got != cfg {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", got, cfg)
	}
}

func TestRecordingsCRUD(t *testing.T) {
	db := openTestDB(t)

	rec, err := db.CreateRecording("test run")
	if err != nil {
		t.Fatal(err)
	}
	if exists, _ := db.RecordingNameExists("test run"); !exists {
		t.Error("recording should exist by name")
	}

	base := time.Date(2026, 7, 24, 12, 0, 0, 0, time.Local)
	var readings []Reading
	for i := 0; i < 250; i++ {
		readings = append(readings, Reading{
			RecordingID: rec.ID,
			Timestamp:   base.Add(time.Duration(i) * time.Millisecond),
			Value:       float64(i) * 0.5,
			Unit:        physics.N,
		})
	}
	if err := db.AppendReadings(readings); err != nil {
		t.Fatal(err)
	}

	sums, err := db.ListRecordingSummaries()
	if err != nil {
		t.Fatal(err)
	}
	if len(sums) != 1 || sums[0].Count != 250 {
		t.Fatalf("summary = %+v, want 1 recording with 250 samples", sums)
	}
	if d := sums[0].Duration(); d != 249*time.Millisecond {
		t.Errorf("duration = %v, want 249ms", d)
	}

	got, err := db.Readings(rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 250 || !got[0].Timestamp.Equal(base) || got[249].Value != 124.5 {
		t.Errorf("readings mismatch: len=%d first=%v lastVal=%v", len(got), got[0].Timestamp, got[len(got)-1].Value)
	}

	if err := db.DeleteRecording(rec.ID); err != nil {
		t.Fatal(err)
	}
	sums, _ = db.ListRecordingSummaries()
	if len(sums) != 0 {
		t.Error("recording should be gone")
	}
	// Cascade should also remove readings.
	if got, _ := db.Readings(rec.ID); len(got) != 0 {
		t.Error("readings should be cascade-deleted")
	}
}
