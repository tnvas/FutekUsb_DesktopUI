package ui

import (
	"runtime"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"futek_usb220/internal/settings"
)

// newHelpTab renders the built-in user guide. Written for users who have
// never worked with a FUTEK sensor before.
func newHelpTab(dbPath string) fyne.CanvasObject {
	md := `
# Quick start

1. **Plug in** the FUTEK USB220 adapter with your sensor attached. The status bar at the
   bottom turns green when the device is found - no drivers or setup needed for reading.
2. **Zero the sensor**: with nothing pressing on the sensor, press **Zero / Tare** on the
   Live tab. This makes the current (unloaded) state read as 0.
3. **Watch the Live tab**: the big number is the current force; the chart shows the last
   seconds of data. Change the unit and decimals right below the chart.
4. **Record**: press **Start recording**, give it a name, press **Stop recording** when done.
5. **Export**: open the **Recordings** tab, select your recording, and export it as a
   CSV file (for Excel/analysis) or a chart image (PNG).

---

# Understanding the readings

**Zero / Tare** - Load sensors always report a small value even with no load (temperature,
mounting pressure, drift). Taring measures that resting value and subtracts it, so your
measurements start from a true zero. Re-tare whenever you re-mount the sensor or the
unloaded reading has drifted.

**Units** - The sensor measures force. You can display it as Newtons (N), kilograms-force
(kg), grams-force (g), pounds-force (lbs) or ounces-force (oz). Changing the unit only
changes the display - recordings are stored internally in Newtons and can be exported in
any unit.

**Sample rate vs. readings per second** - The sample rate (SPS) is how fast the sensor's
electronics sample internally. The USB220 sends one reading for every four samples, so
4800 SPS gives roughly 1200 readings per second. Higher rates capture fast events (impacts)
but produce larger recordings; lower rates give smoother, smaller data.

**Smoothing** - The live value is lightly smoothed (exponential moving average) to keep the
display readable. Recorded data uses the same smoothed values.

---

# Calibration, in plain words

Unlike some sensor modules, the USB220 does **not** send raw ADC numbers that need a
transform applied afterwards. It was calibrated at the factory using the certificate that
shipped with your sensor, and it streams already-calibrated, unit-labelled readings directly
— for example, a line might read “20.53 lbs”. This app simply reads that value, applies
light smoothing, and lets you zero it with **Zero / Tare**. There is nothing to enter from
the calibration certificate; it's baked into the sensor.

If the live reading looks wrong (for example, a known 1 kg weight doesn't read ~1 kg or
~9.81 N), it's almost always one of:

- **Not zeroed** — press **Zero / Tare** with nothing on the sensor first.
- **Wrong sensor for the reading** — double-check you're reading the channel your sensor is
  wired to.
- **A damaged or overloaded sensor** — compare against the certificate in the app's
  docs/calibration folder for your sensor's serial number; a large, consistent error
  suggests the sensor itself needs re-calibration by FUTEK.

---

# Troubleshooting

**The status bar stays “Searching for device…”**
- Check the USB cable and that the adapter's LED is on.
- Linux: your user needs permission for serial ports. Run ` + "`sudo usermod -aG dialout $USER`" + `
  and log out and back in (or start the app with sudo once to verify it's a permission issue).
- Windows: the USB220 uses standard FTDI drivers; Windows Update installs them automatically.
  Check Device Manager for a “USB Serial Port (COMx)” entry.
- Several serial devices connected? Pick the right port in *Settings → Connection*.

**Changing the sample rate fails**
- This uses FUTEK's Windows-only driver DLL, so it needs **Windows** plus **Python 3** with
  the ` + "`pythonnet`" + ` package (` + "`pip install pythonnet`" + `). Reading data works without any of this.

**The value drifts or won't sit at zero**
- Re-tare with the sensor unloaded. Some drift with temperature is normal for load cells.

**Readings look far too big / too small**
- Zero the sensor first (see above). If it's still off, the *Settings → Sensor output →
  Fallback unit* must match what your sensor actually streams — but this is rarely the
  cause, since the app reads the unit the device sends on every line automatically.

---

# Where is my data?

Recordings are stored in a private database (shown below) and are safe across restarts.
Use the Recordings tab to export them wherever you want.

- Database: ` + "`%DBPATH%`" + `
- This build: %VERSION% (%OS%)

Made for the FUTEK USB220 with LLB-series load sensors. Other FUTEK sensors using the same
serial protocol may work, but are untested.
`
	md = strings.NewReplacer(
		"%VERSION%", settings.RELEASE_VERSION,
		"%OS%", runtime.GOOS+"/"+runtime.GOARCH,
		"%DBPATH%", dbPath,
	).Replace(md)

	rich := widget.NewRichTextFromMarkdown(md)
	rich.Wrapping = fyne.TextWrapWord
	return container.NewVScroll(container.NewPadded(rich))
}
