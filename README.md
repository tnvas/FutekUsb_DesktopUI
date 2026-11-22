# UsbSens3 — FUTEK USB220 Sensor Recorder (development repo)

Desktop application (Go + [Fyne](https://fyne.io)) that reads, records, charts and
exports force data from FUTEK sensors connected through a **FUTEK USB220**
serial-to-USB adapter. Tested with the LLB400 load button.

The user-facing README (for the released binaries) lives in the public repo:
[`Futek-UsbSens3/Readme.md`](Futek-UsbSens3/Readme.md).

## Architecture

```
main.go                    thin entry point
internal/
  app/         Engine: wires device → processing → chart/recorder
  device/      serial discovery + connection manager (auto-reconnect), simulator
  processing/  line parser, calibration, EMA smoothing, tare, stats, chart buckets
  storage/     SQLite: config, recordings, readings (per-user config dir)
  recorder/    batched recording writer (DB + optional live CSV)
  export/      CSV and PNG chart export
  futek/       FUTEK USB_DLL bridge via embedded Python (Windows only)
  physics/     force unit conversions (N is the canonical unit)
  ui/          Fyne GUI: Live / Recordings / Settings / Help tabs
  settings/    build-time metadata (-ldflags -X)
  logging/     rotated log file
```

Data flow: the device manager owns the serial port and streams raw ASCII lines.
The engine parses each line, applies calibration + smoothing + tare (all values
kept in Newtons), and fans samples out to the live chart buffer and the recorder.
The UI polls snapshots at 10 Hz via `fyne.Do`.

Data locations (per user): database and log in `os.UserConfigDir()/UsbSens3/`.
A legacy `./db/database.sqlite` next to the working directory is still honoured,
as is the `DB_PATH` environment variable.

## Building

```bash
make app        # Linux binary -> bin/UsbSens3
make win_build  # Windows binary via fyne-cross (Docker)
make release    # both + zip packages
make test       # unit tests
make sim        # run with a simulated sensor (no hardware needed)
```

Simulation is a build flag: `SIMULATE_USB=true` (see makefile) replaces the
serial device with a synthetic signal generator — useful for GUI work and demos.

## Hardware notes

- The USB220 runs at a fixed 2,000,000 baud and is identified by FTDI VID `0403`
  / PID `6001`.
- Reading data works on Linux and Windows. **Changing the sample rate and reading
  the stored calibration require Windows** (FUTEK's `FUTEK_USB_DLL.dll`) plus
  Python 3 with `pythonnet`. The DLL and the Python helper are embedded in the
  binary (`internal/futek/`).
- The device streams one reading per four ADC samples (4800 SPS ≈ 1200 readings/s).

Reference documentation: [`docs/api/`](docs/api/) (FUTEK DLL programmer's guide,
command overview) and [`docs/calibration/`](docs/calibration/) (example
calibration certificate).
