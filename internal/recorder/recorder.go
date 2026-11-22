// Package recorder captures the live sample stream into the database and,
// optionally, a CSV file written incrementally while recording.
package recorder

import (
	"encoding/csv"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"futek_usb220/internal/physics"
	"futek_usb220/internal/processing"
	"futek_usb220/internal/storage"
)

const (
	batchSize     = 200
	flushInterval = 500 * time.Millisecond
	feedBuffer    = 8192
)

// Info describes the active recording for display purposes.
type Info struct {
	Name    string
	Started time.Time
	Count   int64
	CSVPath string // empty when recording to database only
}

// Recorder buffers incoming samples and persists them in batches. All
// methods are safe for concurrent use; Feed is cheap enough to call at the
// full 4800 SPS device rate.
type Recorder struct {
	db *storage.DB

	mu      sync.Mutex
	session *session
	dropped atomic.Int64
}

type session struct {
	rec     storage.Recording
	started time.Time
	count   atomic.Int64

	in   chan storage.Reading
	done chan struct{} // closed when the writer goroutine has flushed everything

	csvPath string
	csvFile *os.File
	csvW    *csv.Writer
}

// New creates a recorder writing to db.
func New(db *storage.DB) *Recorder {
	return &Recorder{db: db}
}

// Start begins a new recording. csvPath, when non-empty, additionally streams
// the readings to a CSV file as they arrive.
func (r *Recorder) Start(name, csvPath string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.session != nil {
		return errors.New("a recording is already running")
	}
	if name == "" {
		return errors.New("recording name must not be empty")
	}
	if exists, err := r.db.RecordingNameExists(name); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("a recording named %q already exists", name)
	}

	rec, err := r.db.CreateRecording(name)
	if err != nil {
		return err
	}

	s := &session{
		rec:     rec,
		started: time.Now(),
		in:      make(chan storage.Reading, feedBuffer),
		done:    make(chan struct{}),
		csvPath: csvPath,
	}
	if csvPath != "" {
		f, err := os.Create(csvPath)
		if err != nil {
			r.db.DeleteRecording(rec.ID)
			return fmt.Errorf("create CSV file: %w", err)
		}
		s.csvFile = f
		s.csvW = csv.NewWriter(f)
		s.csvW.Write([]string{"Timestamp", "Value", "Unit"})
	}

	go s.run(r.db)
	r.session = s
	return nil
}

// Stop ends the active recording, flushing all buffered data, and returns
// its final state.
func (r *Recorder) Stop() (Info, error) {
	r.mu.Lock()
	s := r.session
	r.session = nil
	r.mu.Unlock()
	if s == nil {
		return Info{}, errors.New("no recording is running")
	}
	close(s.in)
	<-s.done
	return Info{Name: s.rec.Name, Started: s.started, Count: s.count.Load(), CSVPath: s.csvPath}, nil
}

// Feed hands one sample to the active recording; it is a no-op when idle.
// Samples are stored in Newtons.
func (r *Recorder) Feed(sample processing.Sample) {
	r.mu.Lock()
	s := r.session
	r.mu.Unlock()
	if s == nil {
		return
	}
	reading := storage.Reading{
		RecordingID: s.rec.ID,
		Timestamp:   sample.At,
		Value:       sample.Newtons,
		Unit:        physics.N,
	}
	select {
	case s.in <- reading:
	default:
		r.dropped.Add(1) // writer stalled; drop rather than block acquisition
	}
}

// Active returns the state of the running recording, if any.
func (r *Recorder) Active() (Info, bool) {
	r.mu.Lock()
	s := r.session
	r.mu.Unlock()
	if s == nil {
		return Info{}, false
	}
	return Info{Name: s.rec.Name, Started: s.started, Count: s.count.Load(), CSVPath: s.csvPath}, true
}

// Dropped reports how many samples were discarded because storage could not
// keep up (should stay zero in normal operation).
func (r *Recorder) Dropped() int64 { return r.dropped.Load() }

// run drains the feed channel, writing batches to the database and CSV.
func (s *session) run(db *storage.DB) {
	defer close(s.done)
	defer s.closeCSV()

	batch := make([]storage.Reading, 0, batchSize)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := db.AppendReadings(batch); err != nil {
			log.Printf("recorder: database write failed: %v", err)
		}
		if s.csvW != nil {
			for _, r := range batch {
				s.csvW.Write([]string{
					r.Timestamp.Format(storage.TimeFormat),
					strconv.FormatFloat(r.Value, 'f', 6, 64),
					r.Unit,
				})
			}
			s.csvW.Flush()
			if err := s.csvW.Error(); err != nil {
				log.Printf("recorder: CSV write failed: %v", err)
			}
		}
		s.count.Add(int64(len(batch)))
		batch = batch[:0]
	}

	for {
		select {
		case reading, ok := <-s.in:
			if !ok {
				flush()
				return
			}
			batch = append(batch, reading)
			if len(batch) >= batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (s *session) closeCSV() {
	if s.csvFile != nil {
		s.csvFile.Close()
	}
}
