package ui

import (
	"fmt"
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"futek_usb220/internal/device"
	"futek_usb220/internal/futek"
	"futek_usb220/internal/physics"
)

const autoDetectLabel = "Auto-detect (recommended)"

// settingsTab lets the user manage sampling, display, connection and
// appearance settings, with plain-language hints for people who have never
// used a FUTEK sensor before.
type settingsTab struct {
	ui      *UI
	content fyne.CanvasObject

	deviceUnit *widget.Select
	sampleRate *widget.Select

	portSelect *widget.Select
	ports      []device.PortInfo
}

func newSettingsTab(u *UI) *settingsTab {
	t := &settingsTab{ui: u}
	cfg := u.engine.Config()

	// --- sensor output card ---
	t.deviceUnit = widget.NewSelect(physics.Units, nil)
	t.deviceUnit.Selected = cfg.DeviceUnit
	sensorForm := container.NewVBox(
		hint("The USB220 calibrates itself: it was configured at the factory using the "+
			"calibration certificate that shipped with your sensor, and every reading it "+
			"sends already carries its unit (for example \"20.53     lbs\"). There is nothing "+
			"to enter here — the app reads the certificate's work directly off the wire."),
		formRow("Fallback unit", t.deviceUnit,
			"Only used if a reading ever arrives without a recognisable unit label. "+
				"Leave this matching your sensor's configured output unit (see its calibration certificate)."),
	)

	// --- sampling card ---
	rateOptions := make([]string, len(futek.SampleRates))
	for i, r := range futek.SampleRates {
		rateOptions[i] = fmt.Sprintf("%d SPS  (≈%g readings/s)", r, futek.Bandwidth(r))
	}
	t.sampleRate = widget.NewSelect(rateOptions, nil)
	t.selectRate(cfg.SampleRateSPS)

	samplingHint := "How fast the sensor samples internally. The USB220 outputs one reading per four samples, " +
		"so 4800 SPS ≈ 1200 readings per second. Higher = more detail, larger recordings."
	if runtime.GOOS != "windows" {
		samplingHint += "\nNote: programming the rate into the device only works on Windows (FUTEK's driver DLL); " +
			"on Linux the device keeps its last programmed rate."
	} else {
		samplingHint += "\nChanging the rate needs Python 3 with the “pythonnet” package installed."
	}
	samplingForm := container.NewVBox(
		formRow("Sample rate", t.sampleRate, samplingHint),
	)

	// --- appearance card ---
	themeSelect := widget.NewSelect(themeModes, func(mode string) {
		u.applyThemeMode(mode)
	})
	themeSelect.Selected = u.currentThemeMode()
	appearanceForm := container.NewVBox(
		formRow("Theme", themeSelect,
			"Dark is easier on the eyes in dim environments (e.g. at the range); "+
				"System follows your operating system. Applied immediately."),
	)

	// --- connection card ---
	t.portSelect = widget.NewSelect([]string{autoDetectLabel}, nil)
	t.portSelect.Selected = autoDetectLabel
	rescanBtn := widget.NewButtonWithIcon("Rescan ports", theme.SearchIcon(), t.refreshPorts)

	connectionForm := container.NewVBox(
		formRow("Serial port", container.NewBorder(nil, nil, nil, rescanBtn, t.portSelect),
			"Leave on auto-detect: the app finds the USB220 by its USB identifiers. "+
				"Pick a specific port only if you have several adapters connected."),
	)

	// --- save ---
	saveBtn := widget.NewButtonWithIcon("Save and apply", theme.ConfirmIcon(), t.save)
	saveBtn.Importance = widget.HighImportance

	content := container.NewVBox(
		widget.NewCard("Sensor output", "How the USB220 reports force", sensorForm),
		widget.NewCard("Sampling", "", samplingForm),
		widget.NewCard("Appearance", "", appearanceForm),
		widget.NewCard("Connection", "", connectionForm),
		container.NewHBox(layout.NewSpacer(), saveBtn),
	)
	t.content = container.NewVScroll(container.NewPadded(content))
	t.refreshPorts()
	return t
}

// formRow renders a labelled input with a hint line underneath.
func formRow(label string, input fyne.CanvasObject, hintText string) fyne.CanvasObject {
	l := widget.NewLabel(label)
	l.TextStyle = fyne.TextStyle{Bold: true}
	return container.NewVBox(
		container.NewBorder(nil, nil, newFixedWidth(l, 150), nil, input),
		hint(hintText),
	)
}

func hint(text string) fyne.CanvasObject {
	h := widget.NewLabel(text)
	h.Wrapping = fyne.TextWrapWord
	h.TextStyle = fyne.TextStyle{Italic: true}
	return h
}

func newFixedWidth(obj fyne.CanvasObject, w float32) fyne.CanvasObject {
	return container.New(&fixedWidthLayout{w: w}, obj)
}

type fixedWidthLayout struct{ w float32 }

func (f *fixedWidthLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	h := float32(0)
	for _, o := range objs {
		if m := o.MinSize().Height; m > h {
			h = m
		}
	}
	return fyne.NewSize(f.w, h)
}

func (f *fixedWidthLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Resize(size)
		o.Move(fyne.NewPos(0, 0))
	}
}

func (t *settingsTab) selectRate(sps int) {
	for i, r := range futek.SampleRates {
		if r == sps {
			t.sampleRate.SetSelectedIndex(i)
			return
		}
	}
}

func (t *settingsTab) selectedRate() int {
	idx := t.sampleRate.SelectedIndex()
	if idx < 0 || idx >= len(futek.SampleRates) {
		return t.ui.engine.Config().SampleRateSPS
	}
	return futek.SampleRates[idx]
}

// refreshPorts re-enumerates serial ports for the connection selector.
func (t *settingsTab) refreshPorts() {
	go func() {
		ports, err := device.ListPorts()
		fyne.Do(func() {
			if err != nil {
				return
			}
			t.ports = ports
			options := []string{autoDetectLabel}
			for _, p := range ports {
				options = append(options, p.Label())
			}
			current := t.portSelect.Selected
			t.portSelect.Options = options
			t.portSelect.Selected = current
			preferred := t.ui.engine.Devices.PreferredPort()
			if preferred != "" {
				for i, p := range ports {
					if p.Name == preferred {
						t.portSelect.Selected = options[i+1]
					}
				}
			} else if !contains(options, current) {
				t.portSelect.Selected = autoDetectLabel
			}
			t.portSelect.Refresh()
		})
	}()
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// save validates, persists and applies all settings.
func (t *settingsTab) save() {
	old := t.ui.engine.Config()
	cfg := old
	cfg.DeviceUnit = t.deviceUnit.Selected
	cfg.SampleRateSPS = t.selectedRate()
	cfg.DisplayUnit = t.ui.live.unit
	cfg.DisplayDecimals = t.ui.live.decimals

	cfg.PreferredPort = ""
	if t.portSelect.Selected != autoDetectLabel {
		for _, p := range t.ports {
			if p.Label() == t.portSelect.Selected {
				cfg.PreferredPort = p.Name
			}
		}
	}

	if err := t.ui.engine.SaveConfig(cfg); err != nil {
		dialog.ShowError(err, t.ui.win)
		return
	}

	if cfg.SampleRateSPS != old.SampleRateSPS {
		t.applySampleRate(cfg.SampleRateSPS)
		return
	}
	dialog.ShowInformation("Settings saved", "Your settings were saved and applied.", t.ui.win)
}

// applySampleRate programs the device through the FUTEK DLL, with progress.
func (t *settingsTab) applySampleRate(sps int) {
	progress := dialog.NewCustomWithoutButtons("Programming device",
		container.NewVBox(
			widget.NewLabel(fmt.Sprintf("Setting sample rate to %d SPS…\nThe connection is briefly interrupted while the device is programmed.", sps)),
			widget.NewProgressBarInfinite(),
		), t.ui.win)
	progress.Show()

	go func() {
		err := t.ui.engine.ApplySampleRate(t.ui.ctx, sps)
		fyne.Do(func() {
			progress.Hide()
			switch {
			case err == nil:
				dialog.ShowInformation("Settings saved",
					fmt.Sprintf("Sample rate set to %d SPS (≈%g readings/s).", sps, futek.Bandwidth(sps)), t.ui.win)
			case err == futek.ErrUnsupportedOS:
				dialog.ShowInformation("Saved - Windows needed to program the device",
					"The setting was saved, but the FUTEK driver DLL only runs on Windows, so the device itself keeps "+
						"its previously programmed rate. Connect the sensor to a Windows machine to change it.", t.ui.win)
			default:
				dialog.ShowError(fmt.Errorf("the setting was saved, but programming the device failed:\n%w", err), t.ui.win)
			}
		})
	}()
}
