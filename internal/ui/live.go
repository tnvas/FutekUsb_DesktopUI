package ui

import (
	"fmt"
	"image/color"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"futek_usb220/internal/device"
	"futek_usb220/internal/export"
	"futek_usb220/internal/physics"
	"futek_usb220/internal/processing"
	"futek_usb220/internal/recorder"
)

// liveTab is the main dashboard: a large live readout, session statistics,
// the rolling chart and the recording/tare controls.
type liveTab struct {
	ui      *UI
	content fyne.CanvasObject

	unit     string
	decimals int

	valueText *canvas.Text
	unitText  *canvas.Text

	statMin *widget.Label
	statMax *widget.Label
	statAvg *widget.Label

	chart *LiveChart

	recordBtn *widget.Button
	tareBtn   *widget.Button
	resetBtn  *widget.Button

	recBanner  *fyne.Container
	recDot     *canvas.Circle
	recLabel   *widget.Label
	taring     bool
	tareDialog dialog.Dialog
}

func newLiveTab(u *UI) *liveTab {
	cfg := u.engine.Config()
	t := &liveTab{
		ui:       u,
		unit:     cfg.DisplayUnit,
		decimals: cfg.DisplayDecimals,
	}

	// --- big readout ---
	t.valueText = canvas.NewText("-", theme.Color(theme.ColorNameForeground))
	t.valueText.TextSize = 58
	t.valueText.TextStyle = fyne.TextStyle{Bold: true}
	t.unitText = canvas.NewText(t.unit, theme.Color(theme.ColorNamePrimary))
	t.unitText.TextSize = 28
	t.unitText.TextStyle = fyne.TextStyle{Bold: true}
	readout := container.NewHBox(
		layout.NewSpacer(),
		t.valueText,
		container.NewVBox(layout.NewSpacer(), t.unitText),
		layout.NewSpacer(),
	)

	// --- session statistics ---
	t.statMin = statValueLabel()
	t.statMax = statValueLabel()
	t.statAvg = statValueLabel()
	stats := container.NewGridWithColumns(3,
		statBox("Min", t.statMin),
		statBox("Max", t.statMax),
		statBox("Average", t.statAvg),
	)

	// --- chart with window selector ---
	t.chart = NewLiveChart(30 * time.Second)
	windowSelect := widget.NewSelect([]string{"10 s", "30 s", "60 s", "120 s"}, func(s string) {
		secs, _ := strconv.Atoi(s[:len(s)-2])
		t.chart.SetWindow(time.Duration(secs) * time.Second)
	})
	windowSelect.Selected = "30 s"

	unitSelect := widget.NewSelect(physics.Units, func(unit string) {
		t.unit = unit
		t.unitText.Text = unit
		t.unitText.Refresh()
	})
	unitSelect.Selected = t.unit

	decimalsSelect := widget.NewSelect([]string{"0", "1", "2", "3", "4", "5", "6"}, func(s string) {
		t.decimals, _ = strconv.Atoi(s)
	})
	decimalsSelect.Selected = strconv.Itoa(t.decimals)

	chartBar := container.NewHBox(
		widget.NewLabel("Window:"), windowSelect,
		widget.NewLabel("  Unit:"), unitSelect,
		widget.NewLabel("  Decimals:"), decimalsSelect,
		layout.NewSpacer(),
	)

	// --- controls ---
	t.recordBtn = widget.NewButtonWithIcon("Start recording", theme.MediaRecordIcon(), t.onRecordTapped)
	t.recordBtn.Importance = widget.HighImportance
	t.tareBtn = widget.NewButtonWithIcon("Zero / Tare", theme.ViewRefreshIcon(), t.onTareTapped)
	t.resetBtn = widget.NewButtonWithIcon("Reset statistics", theme.DeleteIcon(), func() {
		t.ui.engine.Pipeline.ResetStats()
	})
	controls := container.NewHBox(t.recordBtn, t.tareBtn, t.resetBtn, layout.NewSpacer())

	// --- recording banner (hidden unless recording) ---
	t.recDot = canvas.NewCircle(color.NRGBA{R: 0xd9, G: 0x53, B: 0x4f, A: 0xff})
	t.recLabel = widget.NewLabel("")
	t.recLabel.TextStyle = fyne.TextStyle{Bold: true}
	t.recBanner = container.NewHBox(
		newFixedSize(t.recDot, 10, 10), t.recLabel, layout.NewSpacer(),
	)
	t.recBanner.Hide()

	top := container.NewVBox(
		readout,
		stats,
		widget.NewSeparator(),
		chartBar,
	)
	bottom := container.NewVBox(
		widget.NewSeparator(),
		t.recBanner,
		controls,
	)
	t.content = container.NewBorder(top, bottom, nil, nil, container.NewPadded(t.chart))
	return t
}

func statValueLabel() *widget.Label {
	l := widget.NewLabel("-")
	l.Alignment = fyne.TextAlignCenter
	l.TextStyle = fyne.TextStyle{Bold: true}
	return l
}

func statBox(title string, value *widget.Label) fyne.CanvasObject {
	caption := widget.NewLabel(title)
	caption.Alignment = fyne.TextAlignCenter
	caption.TextStyle = fyne.TextStyle{Italic: true}
	return container.NewVBox(caption, value)
}

// update is called from the UI refresh loop (already on the Fyne thread).
func (t *liveTab) update(stats processing.Stats, status device.StatusEvent,
	buckets []processing.Bucket, recInfo recorder.Info, recording bool) {

	factor := displayFactor(t.unit)
	connected := status.Status == device.StatusConnected

	// Track the active theme so the readout stays readable in light and dark.
	t.valueText.Color = theme.Color(theme.ColorNameForeground)
	t.unitText.Color = theme.Color(theme.ColorNamePrimary)
	t.unitText.Refresh()

	if connected {
		t.valueText.Text = strconv.FormatFloat(stats.Current*factor, 'f', t.decimals, 64)
		t.statMin.SetText(formatValue(stats.Min*factor, t.decimals, t.unit))
		t.statMax.SetText(formatValue(stats.Max*factor, t.decimals, t.unit))
		t.statAvg.SetText(formatValue(stats.Avg*factor, t.decimals, t.unit))
	} else {
		t.valueText.Text = "-"
		t.statMin.SetText("-")
		t.statMax.SetText("-")
		t.statAvg.SetText("-")
	}
	t.valueText.Refresh()

	t.chart.SetData(buckets, factor, t.unit)

	// Buttons follow connection / recording state.
	if connected {
		t.recordBtn.Enable()
		if t.taring {
			t.tareBtn.Disable()
		} else {
			t.tareBtn.Enable()
		}
	} else if !recording {
		t.recordBtn.Disable()
		t.tareBtn.Disable()
	}

	if recording {
		elapsed := time.Since(recInfo.Started).Round(time.Second)
		t.recLabel.SetText(fmt.Sprintf("Recording “%s” - %s - %s samples",
			recInfo.Name, elapsed, formatCount(recInfo.Count)))
		if !t.recBanner.Visible() {
			t.recBanner.Show()
		}
		if t.recordBtn.Text != "Stop recording" {
			t.recordBtn.SetText("Stop recording")
			t.recordBtn.SetIcon(theme.MediaStopIcon())
			t.recordBtn.Importance = widget.DangerImportance
			t.recordBtn.Refresh()
		}
	} else {
		if t.recBanner.Visible() {
			t.recBanner.Hide()
		}
		if t.recordBtn.Text != "Start recording" {
			t.recordBtn.SetText("Start recording")
			t.recordBtn.SetIcon(theme.MediaRecordIcon())
			t.recordBtn.Importance = widget.HighImportance
			t.recordBtn.Refresh()
		}
	}
}

func formatValue(v float64, decimals int, unit string) string {
	return strconv.FormatFloat(v, 'f', decimals, 64) + " " + unit
}

func formatCount(n int64) string {
	s := strconv.FormatInt(n, 10)
	out := make([]byte, 0, len(s)+len(s)/3)
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ' ')
		}
		out = append(out, c)
	}
	return string(out)
}

// --- recording ------------------------------------------------------------

func (t *liveTab) onRecordTapped() {
	if _, active := t.ui.engine.Recorder.Active(); active {
		t.stopRecording()
		return
	}

	nameEntry := widget.NewEntry()
	nameEntry.SetText(time.Now().Format("recording 2006-01-02 15.04.05"))
	csvCheck := widget.NewCheck("Also save a CSV file while recording", nil)
	hint := widget.NewLabel("The recording is always stored in the app's database;\nyou can export it as CSV or a chart image at any time.")
	hint.TextStyle = fyne.TextStyle{Italic: true}

	form := []*widget.FormItem{
		{Text: "Name", Widget: nameEntry},
		{Text: "", Widget: csvCheck},
		{Text: "", Widget: hint},
	}
	d := dialog.NewForm("Start recording", "Start", "Cancel", form, func(ok bool) {
		if !ok {
			return
		}
		name := nameEntry.Text
		if name == "" {
			dialog.ShowError(fmt.Errorf("please enter a recording name"), t.ui.win)
			return
		}
		if csvCheck.Checked {
			t.pickCSVAndStart(name)
		} else {
			t.startRecording(name, "")
		}
	}, t.ui.win)
	d.Resize(fyne.NewSize(460, 260))
	d.Show()
}

func (t *liveTab) pickCSVAndStart(name string) {
	fd := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, t.ui.win)
			return
		}
		if w == nil {
			return // user cancelled
		}
		path := w.URI().Path()
		w.Close()
		t.startRecording(name, path)
	}, t.ui.win)
	fd.SetFileName(export.SuggestFileName(name, ".csv"))
	fd.Show()
}

func (t *liveTab) startRecording(name, csvPath string) {
	if err := t.ui.engine.Recorder.Start(name, csvPath); err != nil {
		dialog.ShowError(err, t.ui.win)
		return
	}
	t.ui.engine.Pipeline.ResetStats()
}

func (t *liveTab) stopRecording() {
	info, err := t.ui.engine.Recorder.Stop()
	if err != nil {
		dialog.ShowError(err, t.ui.win)
		return
	}
	msg := fmt.Sprintf("Saved %s samples to recording “%s”.", formatCount(info.Count), info.Name)
	if info.CSVPath != "" {
		msg += "\nCSV file: " + info.CSVPath
	}
	dialog.ShowInformation("Recording saved", msg, t.ui.win)
	t.ui.recordings.reload()
}

// --- tare -----------------------------------------------------------------

func (t *liveTab) onTareTapped() {
	t.taring = true
	t.tareBtn.Disable()

	progress := dialog.NewCustomWithoutButtons("Zeroing sensor",
		container.NewVBox(
			widget.NewLabel("Collecting samples to establish the zero point.\nKeep the sensor unloaded and still…"),
			widget.NewProgressBarInfinite(),
		), t.ui.win)
	progress.Show()
	t.tareDialog = progress

	t.ui.engine.Pipeline.StartTare(func(res processing.TareResult) {
		fyne.Do(func() {
			t.taring = false
			if t.tareDialog != nil {
				t.tareDialog.Hide()
				t.tareDialog = nil
			}
			factor := displayFactor(t.unit)
			dialog.ShowInformation("Sensor zeroed",
				fmt.Sprintf("New zero point set.\nMeasured noise: ±%s",
					formatValue(res.Noise*factor, t.decimals+2, t.unit)),
				t.ui.win)
		})
	})
}
