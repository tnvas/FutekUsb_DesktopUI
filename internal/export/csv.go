// Package export writes recorded data to user-chosen CSV and image files.
package export

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"

	"futek_usb220/internal/physics"
	"futek_usb220/internal/storage"
)

// WriteCSV exports readings to path, converting values to targetUnit
// (pass "" to keep each reading's stored unit).
func WriteCSV(path string, readings []storage.Reading, targetUnit string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if err := w.Write([]string{"Timestamp", "Value", "Unit"}); err != nil {
		return err
	}
	for _, r := range readings {
		value, unit := r.Value, r.Unit
		if targetUnit != "" {
			if v, err := physics.Convert(r.Unit, targetUnit, r.Value); err == nil {
				value, unit = v, targetUnit
			}
		}
		if err := w.Write([]string{
			r.Timestamp.Format(storage.TimeFormat),
			strconv.FormatFloat(value, 'f', 6, 64),
			unit,
		}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}
