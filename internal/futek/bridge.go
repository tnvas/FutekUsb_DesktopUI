// Package futek talks to the FUTEK USB_DLL through an embedded Python
// helper. The DLL is Windows-only and needs exclusive access to the USB220,
// so callers must release the serial port (device.Manager.Pause) before
// invoking these functions and resume it afterwards.
//
// Requirements on the user's machine: Python 3 with the "pythonnet" package.
package futek

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"time"
	"unicode"
)

//go:embed futek_embedded.py
var embeddedPy string

//go:embed FUTEK_USB_DLL.dll
var embeddedDLL []byte

// ErrUnsupportedOS is returned on platforms where the FUTEK DLL cannot run.
var ErrUnsupportedOS = errors.New("changing device settings requires Windows (the FUTEK USB DLL is Windows-only)")

// ErrPythonMissing wraps failures to launch Python with a user-actionable hint.
var ErrPythonMissing = errors.New("Python 3 with the 'pythonnet' package is required (install Python, then run: pip install pythonnet)")

// SampleRates lists the sample rates supported by the USB220, ascending.
var SampleRates = []int{5, 10, 15, 20, 25, 30, 50, 60, 100, 300, 600, 960, 1200, 1600, 2400, 4800}

// rateCode maps SPS to the USB220 register code.
var rateCode = map[int]string{
	5: "00", 10: "01", 15: "02", 20: "03", 25: "04", 30: "05", 50: "06", 60: "07",
	100: "08", 300: "09", 600: "0A", 960: "0B", 1200: "0C", 1600: "0D", 2400: "0E", 4800: "0F",
}

// Bandwidth returns the effective readings-per-second for a sample rate
// (the USB220 outputs one reading per four samples).
func Bandwidth(sps int) float64 { return float64(sps) / 4 }

// ValidRate reports whether sps is a rate the device supports.
func ValidRate(sps int) bool {
	_, ok := rateCode[sps]
	return ok
}

// SetSampleRate programs the device sample rate. The serial port must be
// released first.
func SetSampleRate(ctx context.Context, serialNumber string, sps int) error {
	code, ok := rateCode[sps]
	if !ok {
		rates := make([]int, 0, len(rateCode))
		for r := range rateCode {
			rates = append(rates, r)
		}
		sort.Ints(rates)
		return fmt.Errorf("unsupported sample rate %d SPS (supported: %v)", sps, rates)
	}
	_, err := call(ctx, serialNumber, "set_rate", map[string]any{"rate_code": code})
	return err
}

// --- plumbing -------------------------------------------------------------

type request struct {
	DLLPath      string         `json:"dll_path"`
	SerialNumber string         `json:"serial_number"`
	Command      string         `json:"command"`
	Args         map[string]any `json:"args,omitempty"`
}

type response struct {
	OK    bool           `json:"ok"`
	Data  map[string]any `json:"data,omitempty"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		Detail  string `json:"detail,omitempty"`
	} `json:"error,omitempty"`
}

const bridgeTimeout = 20 * time.Second

// call runs one command through the Python helper. The DLL sometimes ignores
// the first command after the port was in use, so a get_calibration warm-up
// call is issued first (a workaround inherited from extensive field testing).
func call(ctx context.Context, serialNumber, command string, args map[string]any) (map[string]any, error) {
	if runtime.GOOS != "windows" {
		return nil, ErrUnsupportedOS
	}
	serial := digitsOnly(serialNumber)
	if serial == "" {
		return nil, errors.New("device serial number unknown; connect the USB220 first")
	}
	dll, err := ensureDLL()
	if err != nil {
		return nil, err
	}

	// Warm-up call; its result is intentionally ignored.
	runPython(ctx, request{DLLPath: dll, SerialNumber: serial, Command: "get_calibration"})
	time.Sleep(100 * time.Millisecond)

	resp, err := runPython(ctx, request{DLLPath: dll, SerialNumber: serial, Command: command, Args: args})
	if err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// ensureDLL writes the embedded DLL to a stable location and returns its path.
func ensureDLL() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "UsbSens3")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}
	path := filepath.Join(dir, "FUTEK_USB_DLL.dll")
	if info, err := os.Stat(path); err == nil && info.Size() == int64(len(embeddedDLL)) {
		return path, nil
	}
	if err := os.WriteFile(path, embeddedDLL, 0o755); err != nil {
		return "", fmt.Errorf("write FUTEK DLL: %w", err)
	}
	return path, nil
}

func runPython(ctx context.Context, req request) (*response, error) {
	ctx, cancel := context.WithTimeout(ctx, bridgeTimeout)
	defer cancel()

	// Bootstrap decodes the embedded module and runs its _main(), which
	// reads a JSON request from stdin and writes a JSON response to stdout.
	bootstrap := fmt.Sprintf(`import base64,types
code=base64.b64decode("%s").decode("utf-8")
mod=types.ModuleType("futek_embedded")
exec(code, mod.__dict__)
mod._main()
`, base64.StdEncoding.EncodeToString([]byte(embeddedPy)))

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, pythonExecutable(), "-c", bootstrap)
	hideConsoleWindow(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%w (%v)", ErrPythonMissing, err)
	}
	stdin.Write(payload)
	stdin.Close()

	outBytes, _ := io.ReadAll(stdout)
	errBytes, _ := io.ReadAll(stderr)
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("FUTEK bridge failed: %w; stderr: %s", err, string(errBytes))
	}

	var resp response
	if err := json.Unmarshal(outBytes, &resp); err != nil {
		return nil, fmt.Errorf("invalid response from FUTEK bridge: %v; output: %s; stderr: %s",
			err, string(outBytes), string(errBytes))
	}
	if !resp.OK {
		if resp.Error != nil {
			return &resp, fmt.Errorf("device command failed: %s (%s)", resp.Error.Message, resp.Error.Type)
		}
		return &resp, errors.New("device command failed")
	}
	return &resp, nil
}

func pythonExecutable() string {
	if runtime.GOOS == "windows" {
		return "python"
	}
	return "python3"
}

func digitsOnly(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if unicode.IsDigit(r) {
			out = append(out, r)
		}
	}
	return string(out)
}
