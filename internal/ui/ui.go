// Package ui implements the Fyne GUI: a live dashboard, recordings manager,
// guided settings and built-in help.
package ui

import (
	"context"
	_ "embed"
	"fmt"
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"futek_usb220/internal/app"
	"futek_usb220/internal/device"
	"futek_usb220/internal/physics"
	"futek_usb220/internal/settings"
)

// refreshInterval is how often live widgets are updated. 10 Hz is smooth for
// the eye without loading the UI thread.
const refreshInterval = 100 * time.Millisecond

//go:embed icon.png
var iconPNG []byte

var appIcon = fyne.NewStaticResource("icon.png", iconPNG)

// UI is the application window and all of its tabs.
type UI struct {
	engine *app.Engine
	ctx    context.Context

	fyneApp fyne.App
	win     fyne.Window

	live       *liveTab
	recordings *recordingsTab

	// status bar
	statusDot   *canvas.Circle
	statusLabel *widget.Label
	rateLabel   *widget.Label
}

// New builds the complete UI on top of a running engine.
func New(ctx context.Context, engine *app.Engine) *UI {
	a := fyneapp.NewWithID("com.futek.usbsens3")
	a.SetIcon(appIcon)
	win := a.NewWindow(fmt.Sprintf("%s %s - FUTEK USB220 Sensor Recorder", settings.APP_NAME, settings.RELEASE_VERSION))
	win.SetIcon(appIcon)

	u := &UI{
		engine:  engine,
		ctx:     ctx,
		fyneApp: a,
		win:     win,
	}
	u.applyThemeMode(u.currentThemeMode())

	u.live = newLiveTab(u)
	u.recordings = newRecordingsTab(u)
	settingsTab := newSettingsTab(u)
	helpTab := newHelpTab(engine.DB.Path())

	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Live", theme.MediaPlayIcon(), u.live.content),
		container.NewTabItemWithIcon("Recordings", theme.FolderIcon(), u.recordings.content),
		container.NewTabItemWithIcon("Settings", theme.SettingsIcon(), settingsTab.content),
		container.NewTabItemWithIcon("Help", theme.HelpIcon(), helpTab),
	)
	tabs.OnSelected = func(item *container.TabItem) {
		switch item.Text {
		case "Recordings":
			u.recordings.reload()
		case "Settings":
			settingsTab.refreshPorts()
		}
	}

	root := container.NewBorder(nil, u.buildStatusBar(), nil, nil, tabs)
	win.SetContent(root)
	win.Resize(fyne.NewSize(1000, 700))
	win.CenterOnScreen()

	go u.refreshLoop()
	return u
}

// ShowAndRun displays the window and enters the Fyne main loop (blocks).
func (u *UI) ShowAndRun() {
	u.win.ShowAndRun()
}

// Quit closes the application from any goroutine.
func (u *UI) Quit() {
	fyne.Do(func() { u.fyneApp.Quit() })
}

func (u *UI) buildStatusBar() fyne.CanvasObject {
	u.statusDot = canvas.NewCircle(color.NRGBA{R: 0xd9, G: 0x53, B: 0x4f, A: 0xff})
	u.statusDot.Resize(fyne.NewSize(10, 10))
	dotHolder := container.NewCenter(newFixedSize(u.statusDot, 10, 10))

	u.statusLabel = widget.NewLabel("Searching for device…")
	u.rateLabel = widget.NewLabel("")
	version := widget.NewLabel(settings.APP_NAME + " " + settings.RELEASE_VERSION)
	version.Importance = widget.LowImportance

	return container.NewBorder(widget.NewSeparator(), nil, nil, nil,
		container.NewHBox(
			widget.NewLabel(" "), dotHolder, u.statusLabel,
			widget.NewSeparator(),
			u.rateLabel,
			layout.NewSpacer(),
			version, widget.NewLabel(" "),
		))
}

// refreshLoop updates all live widgets at a fixed cadence.
func (u *UI) refreshLoop() {
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-u.ctx.Done():
			return
		case <-ticker.C:
		}

		stats := u.engine.Pipeline.Stats()
		status := u.engine.Devices.LastStatus()
		buckets := u.engine.Chart.Snapshot(u.live.chart.Window())
		recInfo, recording := u.engine.Recorder.Active()

		fyne.Do(func() {
			u.updateStatusBar(status, stats.Rate)
			u.live.update(stats, status, buckets, recInfo, recording)
		})
	}
}

func (u *UI) updateStatusBar(status device.StatusEvent, rate int) {
	var dotColor color.NRGBA
	text := status.Status.String()
	switch status.Status {
	case device.StatusConnected:
		dotColor = color.NRGBA{R: 0x2e, G: 0xb8, B: 0x5c, A: 0xff}
		if status.Port.Name != "" {
			text = "Connected - " + status.Port.Name
			if status.Port.Product != "" {
				text += " (" + status.Port.Product + ")"
			}
		}
	case device.StatusSearching, device.StatusPaused:
		dotColor = color.NRGBA{R: 0xe6, G: 0xa2, B: 0x3c, A: 0xff}
	default:
		dotColor = color.NRGBA{R: 0xd9, G: 0x53, B: 0x4f, A: 0xff}
	}
	u.statusDot.FillColor = dotColor
	u.statusDot.Refresh()
	u.statusLabel.SetText(text)

	if status.Status == device.StatusConnected && rate > 0 {
		u.rateLabel.SetText(fmt.Sprintf("%d readings/s", rate))
	} else {
		u.rateLabel.SetText("")
	}
}

// displayFactor returns the multiplier converting Newtons to the current
// display unit.
func displayFactor(unit string) float64 {
	f, err := physics.FromNewtons(unit, 1)
	if err != nil {
		return 1
	}
	return f
}

// --- small layout helpers -------------------------------------------------

// newFixedSize wraps an object at a constant size (used for the status dot).
func newFixedSize(obj fyne.CanvasObject, w, h float32) fyne.CanvasObject {
	return container.New(&fixedLayout{w: w, h: h}, obj)
}

type fixedLayout struct{ w, h float32 }

func (f *fixedLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(f.w, f.h) }
func (f *fixedLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Resize(fyne.NewSize(f.w, f.h))
		o.Move(fyne.NewPos((size.Width-f.w)/2, (size.Height-f.h)/2))
	}
}
