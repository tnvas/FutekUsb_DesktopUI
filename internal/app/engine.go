// Package app wires the device, processing, storage and recording layers
// together, independent of any GUI concerns.
package app

import (
	"context"
	"log"
	"runtime"
	"strings"
	"sync"
	"time"

	"futek_usb220/internal/device"
	"futek_usb220/internal/futek"
	"futek_usb220/internal/physics"
	"futek_usb220/internal/processing"
	"futek_usb220/internal/recorder"
	"futek_usb220/internal/storage"
)

// ChartWindow is how much live history is kept for the on-screen chart.
const ChartWindow = 2 * time.Minute

// chartBucket is the aggregation slot size for the live chart.
const chartBucket = 50 * time.Millisecond

// Engine owns the data path: raw device lines are parsed, calibrated and
// fanned out to the live chart buffer and the recorder.
type Engine struct {
	DB       *storage.DB
	Devices  *device.Manager
	Pipeline *processing.Pipeline
	Chart    *processing.ChartBuffer
	Recorder *recorder.Recorder

	mu  sync.Mutex
	cfg storage.AppConfig
}

// New assembles an engine from persisted configuration.
func New(db *storage.DB, simulate bool) *Engine {
	cfg := db.LoadConfig()
	mgr := device.NewManager(simulate)
	mgr.SetPreferredPort(cfg.PreferredPort)
	return &Engine{
		DB:       db,
		Devices:  mgr,
		Pipeline: processing.NewPipeline(cfg.DeviceUnit),
		Chart:    processing.NewChartBuffer(ChartWindow, chartBucket),
		Recorder: recorder.New(db),
		cfg:      cfg,
	}
}

// Config returns a copy of the current configuration.
func (e *Engine) Config() storage.AppConfig {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cfg
}

// SaveConfig persists cfg and applies it to the running pipeline and device
// manager.
func (e *Engine) SaveConfig(cfg storage.AppConfig) error {
	if err := e.DB.SaveConfig(cfg); err != nil {
		return err
	}
	e.mu.Lock()
	old := e.cfg
	e.cfg = cfg
	e.mu.Unlock()

	e.Pipeline.SetFallbackUnit(cfg.DeviceUnit)
	if old.PreferredPort != cfg.PreferredPort {
		e.Devices.SetPreferredPort(cfg.PreferredPort)
	}
	return nil
}

// Run starts the device manager and consumes its sample stream until ctx is
// cancelled. Blocks; run in a goroutine.
func (e *Engine) Run(ctx context.Context) {
	go e.Devices.Run(ctx)

	deviceErrs := 0
	rateApplied := false
	spikes := 0
	despike := processing.NewDespiker(func(r processing.Reading) {
		spikes++
		if spikes%20 == 1 {
			log.Printf("discarding corrupted reading #%d: raw=%q -> %.1f N (isolated spike)",
				spikes, r.Raw, r.Newtons)
		}
	})

	var lastLogged time.Time
	feed := func(r processing.Reading) {
		out, ok := despike.Push(r)
		if !ok {
			return
		}
		// Throttled trace of accepted (non-spike) readings: what the device
		// actually sent and what this app computed from it, so a real test
		// can be verified end-to-end from the log alone.
		if time.Since(lastLogged) >= time.Second {
			lastLogged = out.At
			kg, _ := physics.FromNewtons(physics.KG, out.Newtons)
			log.Printf("accepted reading: raw=%q parsed=%.4f %s -> %.2f N (%.2f kg)",
				out.Raw, out.Value, out.Unit, out.Newtons, kg)
		}
		e.feedSample(out)
	}
	defer func() {
		if out, ok := despike.Flush(); ok {
			e.feedSample(out)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-e.Devices.Status():
			// Program the configured sample rate into the device once per
			// session (Windows only; silently skipped elsewhere).
			if ev.Status == device.StatusConnected && !rateApplied && runtime.GOOS == "windows" {
				rateApplied = true
				sps := e.Config().SampleRateSPS
				go func() {
					if err := e.ApplySampleRate(ctx, sps); err != nil {
						log.Printf("startup sample rate (%d SPS) not applied: %v", sps, err)
					} else {
						log.Printf("sample rate set to %d SPS on startup", sps)
					}
				}()
			}
		case raw := <-e.Devices.Lines():
			value, unit, ok, err := processing.ParseLine(string(raw.Value))
			if err != nil {
				if err == processing.ErrDeviceError {
					deviceErrs++
					if deviceErrs%1000 == 1 {
						log.Printf("device reported %d error line(s)", deviceErrs)
					}
				}
				continue
			}
			if !ok {
				continue
			}

			resolvedUnit := unit
			if resolvedUnit == "" {
				resolvedUnit = e.Config().DeviceUnit
			}
			newtons, uerr := physics.ToNewtons(resolvedUnit, value)
			if uerr != nil {
				continue
			}
			feed(processing.Reading{
				At: raw.At, Value: value, Unit: unit,
				Raw: strings.TrimSpace(string(raw.Value)), Newtons: newtons,
			})
		}
	}
}

// feedSample runs one despiked reading through the pipeline and fans it out
// to the chart and recorder.
func (e *Engine) feedSample(r processing.Reading) {
	sample := e.Pipeline.Process(r.At, r.Value, r.Unit)
	e.Chart.Add(sample)
	e.Recorder.Feed(sample)
}

// ApplySampleRate programs the device sample rate through the FUTEK DLL.
// The serial connection is paused while the DLL owns the port. Windows only;
// on other platforms the device keeps its stored rate and an explanatory
// error is returned.
func (e *Engine) ApplySampleRate(ctx context.Context, sps int) error {
	port, connected := e.Devices.ConnectedPort()
	if !connected {
		// Try anyway with the last known port from a scan.
		if p, ok := device.FindFutekPort(); ok {
			port = p
		}
	}

	e.Devices.Pause()
	defer e.Devices.Resume()
	// Give the OS a moment to fully release the COM port before the DLL grabs it.
	time.Sleep(300 * time.Millisecond)

	if err := futek.SetSampleRate(ctx, port.SerialNumber, sps); err != nil {
		return err
	}
	return nil
}
