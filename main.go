// UsbSens3 records, visualises and exports force readings from FUTEK
// sensors connected through a FUTEK USB220 serial adapter.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"futek_usb220/internal/app"
	"futek_usb220/internal/logging"
	"futek_usb220/internal/settings"
	"futek_usb220/internal/storage"
	"futek_usb220/internal/ui"
)

func main() {
	dbPath := storage.DefaultPath(settings.APP_NAME)
	logging.Init(filepath.Dir(dbPath), settings.APP_NAME)
	log.Printf("%s %s starting (simulate=%v)", settings.APP_NAME, settings.RELEASE_VERSION, settings.Simulate())

	db, err := storage.Open(dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()
	log.Printf("database: %s", db.Path())

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	engine := app.New(db, settings.Simulate())
	go engine.Run(ctx)

	// Close the window if the process is asked to terminate.
	window := ui.New(ctx, engine)
	go func() {
		<-ctx.Done()
		window.Quit()
	}()

	window.ShowAndRun() // blocks until the window closes

	// Orderly shutdown: finish any active recording before closing the DB.
	cancel()
	if _, active := engine.Recorder.Active(); active {
		if info, err := engine.Recorder.Stop(); err == nil {
			log.Printf("recording %q flushed on exit (%d samples)", info.Name, info.Count)
		}
	}
	log.Println("shutdown complete")
}
