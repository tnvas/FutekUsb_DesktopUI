package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"futek_usb220/internal/processing"
)

// chartPoint is one aggregated slot ready for drawing (already converted to
// the display unit).
type chartPoint struct {
	at        time.Time
	min, max  float64
	mean      float64
}

// LiveChart renders the rolling sensor history as a min–max envelope with a
// mean line. Drawing is rasterised, so cost is bounded by pixels rather than
// sample rate.
type LiveChart struct {
	widget.BaseWidget

	mu     sync.Mutex
	data   []chartPoint
	unit   string
	window time.Duration
	yLo    float64
	yHi    float64
	ticks  []float64
}

// NewLiveChart creates the chart with the given initial window.
func NewLiveChart(window time.Duration) *LiveChart {
	c := &LiveChart{window: window, unit: "N", yLo: 0, yHi: 1}
	c.ExtendBaseWidget(c)
	return c
}

// SetWindow changes the visible time span.
func (c *LiveChart) SetWindow(window time.Duration) {
	c.mu.Lock()
	c.window = window
	c.mu.Unlock()
	c.Refresh()
}

// Window returns the current visible time span.
func (c *LiveChart) Window() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.window
}

// SetData replaces the chart contents. factor converts stored Newton values
// into the display unit. Must be called on the Fyne thread (fyne.Do).
func (c *LiveChart) SetData(buckets []processing.Bucket, factor float64, unit string) {
	pts := make([]chartPoint, len(buckets))
	lo, hi := math.Inf(1), math.Inf(-1)
	for i, b := range buckets {
		p := chartPoint{
			at:   b.Start,
			min:  b.Min * factor,
			max:  b.Max * factor,
			mean: b.Mean() * factor,
		}
		pts[i] = p
		if p.min < lo {
			lo = p.min
		}
		if p.max > hi {
			hi = p.max
		}
	}
	if len(pts) == 0 || math.IsInf(lo, 1) {
		lo, hi = 0, 1
	}
	if hi-lo < 1e-9 {
		pad := math.Max(1, math.Abs(hi)*0.1)
		lo, hi = lo-pad, hi+pad
	} else {
		pad := (hi - lo) * 0.08
		lo, hi = lo-pad, hi+pad
	}

	c.mu.Lock()
	c.data = pts
	c.unit = unit
	c.yLo, c.yHi = lo, hi
	c.ticks = niceTicks(lo, hi, 5)
	c.mu.Unlock()
	c.Refresh()
}

// CreateRenderer implements fyne.Widget.
func (c *LiveChart) CreateRenderer() fyne.WidgetRenderer {
	r := &chartRenderer{chart: c}
	r.raster = canvas.NewRaster(r.draw)
	r.empty = canvas.NewText("Waiting for sensor data…", theme.Color(theme.ColorNamePlaceHolder))
	r.empty.Alignment = fyne.TextAlignCenter
	for i := 0; i < maxYTicks; i++ {
		t := canvas.NewText("", theme.Color(theme.ColorNameForeground))
		t.TextSize = 11
		t.Alignment = fyne.TextAlignTrailing
		r.yLabels = append(r.yLabels, t)
	}
	for i := 0; i < 3; i++ {
		t := canvas.NewText("", theme.Color(theme.ColorNameForeground))
		t.TextSize = 11
		r.xLabels = append(r.xLabels, t)
	}
	return r
}

const (
	maxYTicks    = 6
	marginLeft   = float32(64)
	marginBottom = float32(22)
	marginTop    = float32(8)
	marginRight  = float32(12)
)

type chartRenderer struct {
	chart   *LiveChart
	raster  *canvas.Raster
	empty   *canvas.Text
	yLabels []*canvas.Text
	xLabels []*canvas.Text
	size    fyne.Size
}

func (r *chartRenderer) MinSize() fyne.Size { return fyne.NewSize(320, 180) }

func (r *chartRenderer) Layout(size fyne.Size) {
	r.size = size
	r.raster.Move(fyne.NewPos(marginLeft, marginTop))
	r.raster.Resize(fyne.NewSize(size.Width-marginLeft-marginRight, size.Height-marginTop-marginBottom))
	r.empty.Move(fyne.NewPos(size.Width/2-100, size.Height/2-10))
	r.empty.Resize(fyne.NewSize(200, 20))
	r.positionLabels()
}

func (r *chartRenderer) positionLabels() {
	c := r.chart
	c.mu.Lock()
	ticks := append([]float64(nil), c.ticks...)
	lo, hi := c.yLo, c.yHi
	window := c.window
	hasData := len(c.data) > 0
	c.mu.Unlock()

	plotH := r.size.Height - marginTop - marginBottom
	plotW := r.size.Width - marginLeft - marginRight

	for i, lbl := range r.yLabels {
		if i < len(ticks) && hi > lo {
			frac := (ticks[i] - lo) / (hi - lo)
			y := marginTop + plotH*(1-float32(frac)) - 7
			lbl.Text = formatTick(ticks[i])
			lbl.Move(fyne.NewPos(0, y))
			lbl.Resize(fyne.NewSize(marginLeft-8, 14))
			lbl.Show()
		} else {
			lbl.Hide()
		}
		lbl.Refresh()
	}

	xTexts := [3]string{fmt.Sprintf("-%ds", int(window.Seconds())), fmt.Sprintf("-%ds", int(window.Seconds()/2)), "now"}
	xFracs := [3]float32{0, 0.5, 1}
	for i, lbl := range r.xLabels {
		lbl.Text = xTexts[i]
		x := marginLeft + plotW*xFracs[i]
		if i == 2 {
			x -= 30
		} else if i == 1 {
			x -= 12
		}
		lbl.Move(fyne.NewPos(x, r.size.Height-marginBottom+4))
		lbl.Resize(fyne.NewSize(60, 14))
		lbl.Refresh()
	}

	if hasData {
		r.empty.Hide()
	} else {
		r.empty.Show()
	}
}

func (r *chartRenderer) Refresh() {
	fg := theme.Color(theme.ColorNameForeground)
	for _, l := range r.yLabels {
		l.Color = fg
	}
	for _, l := range r.xLabels {
		l.Color = fg
	}
	r.positionLabels()
	r.raster.Refresh()
}

func (r *chartRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.raster, r.empty}
	for _, l := range r.yLabels {
		objs = append(objs, l)
	}
	for _, l := range r.xLabels {
		objs = append(objs, l)
	}
	return objs
}

func (r *chartRenderer) Destroy() {}

// draw rasterises the chart contents at the requested pixel size.
func (r *chartRenderer) draw(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	if w == 0 || h == 0 {
		return img
	}

	c := r.chart
	c.mu.Lock()
	data := c.data
	lo, hi := c.yLo, c.yHi
	ticks := append([]float64(nil), c.ticks...)
	window := c.window
	c.mu.Unlock()

	bg := toNRGBA(theme.Color(theme.ColorNameInputBackground))
	fillRect(img, 0, 0, w, h, bg)

	grid := withAlpha(toNRGBA(theme.Color(theme.ColorNameForeground)), 28)
	if hi > lo {
		for _, tv := range ticks {
			y := int(float64(h) * (1 - (tv-lo)/(hi-lo)))
			drawHLine(img, 0, w, y, grid)
		}
	}
	// Vertical quarter lines
	for _, fx := range []float64{0.25, 0.5, 0.75} {
		drawVLine(img, int(float64(w)*fx), 0, h, grid)
	}

	if len(data) == 0 || hi <= lo {
		return img
	}

	primary := toNRGBA(theme.Color(theme.ColorNamePrimary))
	band := withAlpha(primary, 70)

	now := data[len(data)-1].at
	tStart := now.Add(-window)
	scaleX := float64(w) / window.Seconds()
	scaleY := func(v float64) int {
		return int(float64(h) * (1 - (v-lo)/(hi-lo)))
	}

	prevX, prevY := -1, 0
	for _, p := range data {
		x := int(p.at.Sub(tStart).Seconds() * scaleX)
		if x < 0 {
			continue
		}
		if x >= w {
			x = w - 1
		}
		yMin, yMax := scaleY(p.max), scaleY(p.min) // inverted axis
		drawVLine(img, x, yMin, yMax+1, band)

		y := scaleY(p.mean)
		if prevX >= 0 {
			drawLine(img, prevX, prevY, x, y, primary)
		}
		prevX, prevY = x, y
	}
	return img
}

// --- small software drawing helpers --------------------------------------

func toNRGBA(c color.Color) color.NRGBA {
	r, g, b, a := c.RGBA()
	if a == 0 {
		return color.NRGBA{}
	}
	return color.NRGBA{
		R: uint8((r * 0xff) / a),
		G: uint8((g * 0xff) / a),
		B: uint8((b * 0xff) / a),
		A: uint8(a >> 8),
	}
}

func withAlpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = a
	return c
}

func fillRect(img *image.NRGBA, x0, y0, x1, y1 int, c color.NRGBA) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
}

func blendPx(img *image.NRGBA, x, y int, c color.NRGBA) {
	b := img.Bounds()
	if x < b.Min.X || x >= b.Max.X || y < b.Min.Y || y >= b.Max.Y {
		return
	}
	if c.A == 0xff {
		img.SetNRGBA(x, y, c)
		return
	}
	dst := img.NRGBAAt(x, y)
	a := uint32(c.A)
	ia := 255 - a
	img.SetNRGBA(x, y, color.NRGBA{
		R: uint8((uint32(c.R)*a + uint32(dst.R)*ia) / 255),
		G: uint8((uint32(c.G)*a + uint32(dst.G)*ia) / 255),
		B: uint8((uint32(c.B)*a + uint32(dst.B)*ia) / 255),
		A: 0xff,
	})
}

func drawHLine(img *image.NRGBA, x0, x1, y int, c color.NRGBA) {
	for x := x0; x < x1; x++ {
		blendPx(img, x, y, c)
	}
}

func drawVLine(img *image.NRGBA, x, y0, y1 int, c color.NRGBA) {
	for y := y0; y < y1; y++ {
		blendPx(img, x, y, c)
	}
}

// drawLine draws a 2px-thick line using Bresenham's algorithm.
func drawLine(img *image.NRGBA, x0, y0, x1, y1 int, c color.NRGBA) {
	dx := abs(x1 - x0)
	dy := -abs(y1 - y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy
	for {
		blendPx(img, x0, y0, c)
		blendPx(img, x0, y0+1, c)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// niceTicks returns up to n round-valued tick positions within [lo, hi].
func niceTicks(lo, hi float64, n int) []float64 {
	if hi <= lo || n < 2 {
		return nil
	}
	span := hi - lo
	rawStep := span / float64(n)
	mag := math.Pow(10, math.Floor(math.Log10(rawStep)))
	norm := rawStep / mag
	var step float64
	switch {
	case norm <= 1:
		step = mag
	case norm <= 2:
		step = 2 * mag
	case norm <= 5:
		step = 5 * mag
	default:
		step = 10 * mag
	}
	first := math.Ceil(lo/step) * step
	var ticks []float64
	for v := first; v <= hi && len(ticks) < maxYTicks; v += step {
		ticks = append(ticks, v)
	}
	return ticks
}

func formatTick(v float64) string {
	av := math.Abs(v)
	switch {
	case av >= 100000:
		return fmt.Sprintf("%.3g", v)
	case av >= 100:
		return fmt.Sprintf("%.0f", v)
	case av >= 1:
		return fmt.Sprintf("%.1f", v)
	default:
		return fmt.Sprintf("%.2f", v)
	}
}
