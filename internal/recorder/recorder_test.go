package recorder

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"
	"time"

	"futek_usb220/internal/processing"
	"futek_usb220/internal/storage"
)

func TestRecordSession(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	r := New(db)
	if _, active := r.Active(); active {
		t.Fatal("should start idle")
	}
	if _, err := r.Stop(); err == nil {
		t.Fatal("Stop without Start should fail")
	}

	csvPath := filepath.Join(dir, "out.csv")
	if err := r.Start("session A", csvPath); err != nil {
		t.Fatal(err)
	}
	if err := r.Start("session B", ""); err == nil {
		t.Fatal("second Start should fail while recording")
	}

	base := time.Now()
	for i := 0; i < 500; i++ {
		r.Feed(processing.Sample{At: base.Add(time.Duration(i) * time.Millisecond), Newtons: float64(i)})
	}

	info, err := r.Stop()
	if err != nil {
		t.Fatal(err)
	}
	if info.Count != 500 {
		t.Errorf("recorded %d samples, want 500", info.Count)
	}
	if r.Dropped() != 0 {
		t.Errorf("dropped %d samples", r.Dropped())
	}

	// Database contents
	sums, err := db.ListRecordingSummaries()
	if err != nil || len(sums) != 1 {
		t.Fatalf("summaries: %v %v", sums, err)
	}
	if sums[0].Count != 500 || sums[0].Name != "session A" {
		t.Errorf("summary %+v", sums[0])
	}

	// CSV contents: header + 500 rows
	f, err := os.Open(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 501 {
		t.Errorf("CSV has %d rows, want 501", len(rows))
	}
	if rows[0][0] != "Timestamp" {
		t.Errorf("missing CSV header: %v", rows[0])
	}

	// Duplicate name is rejected.
	if err := r.Start("session A", ""); err == nil {
		t.Error("duplicate recording name should be rejected")
		r.Stop()
	}

	// Feeding while idle is a harmless no-op.
	r.Feed(processing.Sample{At: time.Now(), Newtons: 1})
}
