package export

import (
	"errors"
	"fmt"
	"image/color"
	"math"
	"path/filepath"
	"time"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"

	"futek_usb220/internal/physics"
	"futek_usb220/internal/storage"
)

// PlotOptions controls the rendered graph.
type PlotOptions struct {
	Title    string
	Unit     string // convert values to this unit ("" = keep stored unit)
	WidthPx  int
	HeightPx int
}

// WritePlot renders readings as a time/force line chart to a PNG or SVG file
// (chosen by the path extension; PNG when unrecognised).
func WritePlot(path string, readings []storage.Reading, opts PlotOptions) error {
	if len(readings) == 0 {
		return errors.New("recording contains no data")
	}

	unit := opts.Unit
	if unit == "" {
		unit = readings[0].Unit
	}
	pts := make(plotter.XYs, len(readings))
	for i, r := range readings {
		v := r.Value
		if converted, err := physics.Convert(r.Unit, unit, r.Value); err == nil {
			v = converted
		}
		pts[i].X = float64(r.Timestamp.UnixNano()) / float64(time.Second)
		pts[i].Y = v
	}

	p := plot.New()
	p.Title.Text = opts.Title
	if p.Title.Text == "" {
		p.Title.Text = "Force over time"
	}
	p.Title.TextStyle.Font.Size = 18
	p.BackgroundColor = color.White
	p.X.Label.Text = "Time"
	p.Y.Label.Text = "Force (" + unit + ")"
	p.X.Label.TextStyle.Font.Size = 14
	p.Y.Label.TextStyle.Font.Size = 14
	p.X.Tick.Marker = timeTicks{loc: time.Local}

	grid := plotter.NewGrid()
	grid.Vertical.Color = color.RGBA{A: 30}
	grid.Horizontal.Color = color.RGBA{A: 30}
	p.Add(grid)

	line, err := plotter.NewLine(pts)
	if err != nil {
		return err
	}
	line.LineStyle.Width = vg.Points(1)
	line.LineStyle.Color = color.RGBA{R: 33, G: 94, B: 168, A: 255}
	p.Add(line)

	// Explicit ranges with a little padding.
	xmin, xmax := pts[0].X, pts[len(pts)-1].X
	if xmin == xmax {
		xmin, xmax = xmin-30, xmax+30
	}
	p.X.Min, p.X.Max = xmin, xmax
	ymin, ymax := minmax(pts)
	if ymin == ymax {
		d := math.Max(1, math.Abs(ymin)*0.1)
		ymin, ymax = ymin-d, ymax+d
	}
	pad := (ymax - ymin) * 0.05
	p.Y.Min, p.Y.Max = ymin-pad, ymax+pad

	w := opts.WidthPx
	if w <= 0 {
		w = 1600
	}
	h := opts.HeightPx
	if h <= 0 {
		h = 900
	}
	switch filepath.Ext(path) {
	case ".png", ".svg":
	default:
		path += ".png"
	}
	return p.Save(vg.Length(w)*0.75, vg.Length(h)*0.75, path)
}

func minmax(pts plotter.XYs) (float64, float64) {
	lo, hi := pts[0].Y, pts[0].Y
	for _, p := range pts[1:] {
		if p.Y < lo {
			lo = p.Y
		}
		if p.Y > hi {
			hi = p.Y
		}
	}
	return lo, hi
}

// timeTicks renders X-axis ticks as clock times with a step that suits the
// visible span.
type timeTicks struct{ loc *time.Location }

func (t timeTicks) Ticks(min, max float64) []plot.Tick {
	if max <= min {
		return nil
	}
	start := time.Unix(0, int64(min*float64(time.Second))).In(t.loc)
	end := time.Unix(0, int64(max*float64(time.Second))).In(t.loc)
	span := end.Sub(start)

	var step time.Duration
	var format string
	switch {
	case span <= 30*time.Second:
		step, format = 5*time.Second, "15:04:05"
	case span <= 5*time.Minute:
		step, format = 30*time.Second, "15:04:05"
	case span <= 30*time.Minute:
		step, format = 2*time.Minute, "15:04"
	case span <= 6*time.Hour:
		step, format = 15*time.Minute, "15:04"
	case span <= 48*time.Hour:
		step, format = time.Hour, "Jan _2 15:04"
	default:
		step, format = 24*time.Hour, "2006-01-02"
	}

	aligned := start.Truncate(step)
	if aligned.Before(start) {
		aligned = aligned.Add(step)
	}
	var ticks []plot.Tick
	for tm := aligned; !tm.After(end); tm = tm.Add(step) {
		ticks = append(ticks, plot.Tick{
			Value: float64(tm.UnixNano()) / float64(time.Second),
			Label: tm.Format(format),
		})
	}
	if len(ticks) == 0 {
		ticks = []plot.Tick{
			{Value: min, Label: start.Format(format)},
			{Value: max, Label: end.Format(format)},
		}
	}
	return ticks
}

// SuggestFileName builds a safe default file name for exports.
func SuggestFileName(recordingName, ext string) string {
	safe := make([]rune, 0, len(recordingName))
	for _, r := range recordingName {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			safe = append(safe, r)
		case r == ' ':
			safe = append(safe, '_')
		}
	}
	if len(safe) == 0 {
		safe = []rune("recording")
	}
	return fmt.Sprintf("%s%s", string(safe), ext)
}
