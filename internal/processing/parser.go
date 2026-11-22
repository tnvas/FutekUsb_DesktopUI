package processing

import (
	"errors"
	"strconv"
	"strings"
	"unicode"

	"futek_usb220/internal/physics"
)

// ErrDeviceError indicates the device streamed an "Error" message.
var ErrDeviceError = errors.New("device reported error")

// ParseLine interprets one ASCII line from the USB220 stream, e.g.:
//
//	"+20.5320     lbs\r\n"
//
// The USB220 performs its own calibration onboard (from the loading points
// stored in the sensor's EEPROM) and streams an already-calibrated,
// unit-labelled reading — there is no separate "raw counts" value for this
// application to recalibrate. ParseLine returns the numeric value and the
// unit as reported by the device; unit is "" if the line carried no
// recognisable unit label (Process falls back to a configured default).
//
// Other line shapes are handled gracefully:
//
//	"Error <code/text>"  -> device-side error
//	"----" or headers    -> non-data lines (ignored)
func ParseLine(line string) (value float64, unit string, ok bool, err error) {
	l := strings.TrimSpace(line)
	if l == "" {
		return 0, "", false, nil
	}
	if strings.Contains(strings.ToUpper(l), "ERROR") {
		return 0, "", false, ErrDeviceError
	}

	// The device writes a numeric value followed by whitespace and a unit
	// label ("lbs", "kg", …). Scan the leading numeric run, then treat the
	// remainder as the unit.
	numEnd := 0
	hasDigit := false
	for i, r := range l {
		if unicode.IsDigit(r) || r == '+' || r == '-' || r == '.' || r == 'e' || r == 'E' {
			numEnd = i + 1
			hasDigit = hasDigit || unicode.IsDigit(r)
			continue
		}
		break
	}
	if !hasDigit {
		return 0, "", false, nil // benign non-data line (separators, headers)
	}

	f, perr := strconv.ParseFloat(l[:numEnd], 64)
	if perr != nil {
		return 0, "", false, perr
	}
	return f, normalizeUnitToken(l[numEnd:]), true, nil
}

// normalizeUnitToken maps a unit label from the device stream to one of the
// identifiers in the physics package. Returns "" if unrecognised.
func normalizeUnitToken(tok string) string {
	switch strings.ToLower(strings.TrimSpace(tok)) {
	case "lb", "lbs", "lbf":
		return physics.LBS
	case "kg", "kgf":
		return physics.KG
	case "g", "gf":
		return physics.G
	case "n", "nt":
		return physics.N
	case "oz", "ozf", "os":
		return physics.OZ
	default:
		return ""
	}
}
