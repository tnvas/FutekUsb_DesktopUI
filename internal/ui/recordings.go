package ui

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"futek_usb220/internal/export"
	"futek_usb220/internal/storage"
)

// recordingsTab lists stored recordings with a preview and export actions.
type recordingsTab struct {
	ui      *UI
	content fyne.CanvasObject

	summaries []storage.RecordingSummary
	selected  int // index into summaries, -1 = none

	list       *widget.List
	emptyLabel *widget.Label

	detailName    *widget.Label
	detailMeta    *widget.Label
	preview       *canvas.Image
	previewStatus *widget.Label
	exportCSVBtn  *widget.Button
	exportImgBtn  *widget.Button
	deleteBtn     *widget.Button

	previewToken int // invalidates stale async preview renders
}

func newRecordingsTab(u *UI) *recordingsTab {
	t := &recordingsTab{ui: u, selected: -1}

	t.list = widget.NewList(
		func() int { return len(t.summaries) },
		func() fyne.CanvasObject {
			name := widget.NewLabel("name")
			name.TextStyle = fyne.TextStyle{Bold: true}
			meta := widget.NewLabel("meta")
			meta.TextStyle = fyne.TextStyle{Italic: true}
			return container.NewVBox(name, meta)
		},
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			if i >= len(t.summaries) {
				return
			}
			s := t.summaries[i]
			box := obj.(*fyne.Container)
			box.Objects[0].(*widget.Label).SetText(s.Name)
			box.Objects[1].(*widget.Label).SetText(summaryLine(s))
		},
	)
	t.list.OnSelected = func(id widget.ListItemID) { t.select_(int(id)) }

	t.emptyLabel = widget.NewLabel("No recordings yet.\nUse “Start recording” on the Live tab to capture data.")
	t.emptyLabel.Alignment = fyne.TextAlignCenter
	t.emptyLabel.Hide()

	refreshBtn := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), t.reload)

	// --- detail pane ---
	t.detailName = widget.NewLabel("Select a recording")
	t.detailName.TextStyle = fyne.TextStyle{Bold: true}
	t.detailMeta = widget.NewLabel("")
	t.detailMeta.TextStyle = fyne.TextStyle{Italic: true}

	t.preview = canvas.NewImageFromResource(nil)
	t.preview.FillMode = canvas.ImageFillContain
	t.preview.SetMinSize(fyne.NewSize(400, 260))
	t.previewStatus = widget.NewLabel("")
	t.previewStatus.Alignment = fyne.TextAlignCenter

	t.exportCSVBtn = widget.NewButtonWithIcon("Export CSV…", theme.DocumentSaveIcon(), t.exportCSV)
	t.exportImgBtn = widget.NewButtonWithIcon("Export chart image…", theme.MediaPhotoIcon(), t.exportImage)
	t.deleteBtn = widget.NewButtonWithIcon("Delete…", theme.DeleteIcon(), t.deleteSelected)
	t.deleteBtn.Importance = widget.DangerImportance
	t.setActionsEnabled(false)

	actions := container.NewHBox(t.exportCSVBtn, t.exportImgBtn, layout.NewSpacer(), t.deleteBtn)
	detail := container.NewBorder(
		container.NewVBox(t.detailName, t.detailMeta, widget.NewSeparator()),
		container.NewVBox(widget.NewSeparator(), actions),
		nil, nil,
		container.NewStack(t.preview, container.NewCenter(t.previewStatus)),
	)

	left := container.NewBorder(
		container.NewVBox(refreshBtn, widget.NewSeparator()), nil, nil, nil,
		container.NewStack(t.list, container.NewCenter(t.emptyLabel)),
	)

	split := container.NewHSplit(left, container.NewPadded(detail))
	split.SetOffset(0.35)
	t.content = split
	return t
}

func summaryLine(s storage.RecordingSummary) string {
	line := fmt.Sprintf("%s samples", formatCount(int64(s.Count)))
	if d := s.Duration(); d > 0 {
		line += " · " + d.Round(time.Second).String()
	}
	return line + " · " + s.CreatedAt.Format("2006-01-02 15:04")
}

// reload refreshes the list from the database.
func (t *recordingsTab) reload() {
	summaries, err := t.ui.engine.DB.ListRecordingSummaries()
	if err != nil {
		log.Printf("list recordings: %v", err)
		return
	}
	t.summaries = summaries
	t.selected = -1
	t.list.UnselectAll()
	t.list.Refresh()
	if len(summaries) == 0 {
		t.emptyLabel.Show()
	} else {
		t.emptyLabel.Hide()
	}
	t.detailName.SetText("Select a recording")
	t.detailMeta.SetText("")
	t.preview.Resource = nil
	t.preview.File = ""
	t.preview.Refresh()
	t.previewStatus.SetText("")
	t.setActionsEnabled(false)
}

func (t *recordingsTab) setActionsEnabled(on bool) {
	if on {
		t.exportCSVBtn.Enable()
		t.exportImgBtn.Enable()
		t.deleteBtn.Enable()
	} else {
		t.exportCSVBtn.Disable()
		t.exportImgBtn.Disable()
		t.deleteBtn.Disable()
	}
}

func (t *recordingsTab) select_(i int) {
	if i < 0 || i >= len(t.summaries) {
		return
	}
	t.selected = i
	s := t.summaries[i]
	t.detailName.SetText(s.Name)
	meta := fmt.Sprintf("Created %s · %s samples", s.CreatedAt.Format("2006-01-02 15:04:05"), formatCount(int64(s.Count)))
	if d := s.Duration(); d > 0 {
		meta += fmt.Sprintf(" · duration %s", d.Round(time.Second))
	}
	t.detailMeta.SetText(meta)
	t.setActionsEnabled(true)
	t.renderPreview(s)
}

// renderPreview draws the recording chart into a temp PNG asynchronously.
func (t *recordingsTab) renderPreview(s storage.RecordingSummary) {
	t.previewToken++
	token := t.previewToken
	t.preview.File = ""
	t.preview.Resource = nil
	t.preview.Refresh()
	if s.Count == 0 {
		t.previewStatus.SetText("This recording contains no samples.")
		return
	}
	t.previewStatus.SetText("Rendering preview…")

	go func() {
		readings, err := t.ui.engine.DB.Readings(s.ID)
		path := filepath.Join(os.TempDir(), fmt.Sprintf("usbsens3-preview-%d.png", s.ID))
		if err == nil {
			err = export.WritePlot(path, readings, export.PlotOptions{
				Title:   s.Name,
				Unit:    t.ui.live.unit,
				WidthPx: 1200, HeightPx: 700,
			})
		}
		fyne.Do(func() {
			if token != t.previewToken {
				return // user selected another recording meanwhile
			}
			if err != nil {
				t.previewStatus.SetText("Preview failed: " + err.Error())
				return
			}
			t.previewStatus.SetText("")
			t.preview.File = path
			t.preview.Refresh()
		})
	}()
}

func (t *recordingsTab) currentSummary() (storage.RecordingSummary, bool) {
	if t.selected < 0 || t.selected >= len(t.summaries) {
		return storage.RecordingSummary{}, false
	}
	return t.summaries[t.selected], true
}

// exportCSV asks for a destination and writes the recording as CSV.
func (t *recordingsTab) exportCSV() {
	sum, ok := t.currentSummary()
	if !ok {
		return
	}
	fd := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if err != nil || w == nil {
			return
		}
		path := w.URI().Path()
		w.Close()
		t.runExport("CSV", func() error {
			readings, err := t.ui.engine.DB.Readings(sum.ID)
			if err != nil {
				return err
			}
			return export.WriteCSV(path, readings, t.ui.live.unit)
		}, path)
	}, t.ui.win)
	fd.SetFileName(export.SuggestFileName(sum.Name, ".csv"))
	fd.Show()
}

// exportImage asks for a destination and writes the recording chart as PNG.
func (t *recordingsTab) exportImage() {
	sum, ok := t.currentSummary()
	if !ok {
		return
	}
	fd := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if err != nil || w == nil {
			return
		}
		path := w.URI().Path()
		w.Close()
		t.runExport("chart image", func() error {
			readings, err := t.ui.engine.DB.Readings(sum.ID)
			if err != nil {
				return err
			}
			return export.WritePlot(path, readings, export.PlotOptions{
				Title: sum.Name,
				Unit:  t.ui.live.unit,
			})
		}, path)
	}, t.ui.win)
	fd.SetFileName(export.SuggestFileName(sum.Name, ".png"))
	fd.Show()
}

// runExport executes fn asynchronously behind a progress dialog.
func (t *recordingsTab) runExport(kind string, fn func() error, path string) {
	progress := dialog.NewCustomWithoutButtons("Exporting",
		container.NewVBox(
			widget.NewLabel("Writing "+kind+"…\nLarge recordings can take a moment."),
			widget.NewProgressBarInfinite(),
		), t.ui.win)
	progress.Show()
	go func() {
		err := fn()
		fyne.Do(func() {
			progress.Hide()
			if err != nil {
				dialog.ShowError(err, t.ui.win)
				return
			}
			dialog.ShowInformation("Export complete", "Saved to:\n"+path, t.ui.win)
		})
	}()
}

func (t *recordingsTab) deleteSelected() {
	sum, ok := t.currentSummary()
	if !ok {
		return
	}
	dialog.ShowConfirm("Delete recording",
		fmt.Sprintf("Delete “%s” and its %s samples?\nThis cannot be undone.", sum.Name, formatCount(int64(sum.Count))),
		func(yes bool) {
			if !yes {
				return
			}
			if err := t.ui.engine.DB.DeleteRecording(sum.ID); err != nil {
				dialog.ShowError(err, t.ui.win)
				return
			}
			t.reload()
		}, t.ui.win)
}
