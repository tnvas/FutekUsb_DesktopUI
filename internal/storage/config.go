package storage

import (
	"database/sql"
	"fmt"
	"strconv"

	"futek_usb220/internal/physics"
)

// AppConfig is the persisted application configuration.
type AppConfig struct {
	// Connection
	PreferredPort string // "" = auto-detect

	// DeviceUnit is the unit assumed for device lines that don't carry a
	// recognisable unit label. The USB220 always labels its output (e.g.
	// "lbs"), so this only matters as a fallback for unusual hardware.
	DeviceUnit string

	// Sampling
	SampleRateSPS int

	// Display
	DisplayUnit     string
	DisplayDecimals int
}

// DefaultConfig returns the configuration used on first run.
func DefaultConfig() AppConfig {
	return AppConfig{
		DeviceUnit:      physics.LBS,
		SampleRateSPS:   4800,
		DisplayUnit:     physics.N,
		DisplayDecimals: 2,
	}
}

// LoadConfig reads the stored configuration, filling gaps with defaults.
func (d *DB) LoadConfig() AppConfig {
	cfg := DefaultConfig()
	row := d.sql.QueryRow(`SELECT
		COALESCE(usb_target,''), COALESCE(device_unit,''),
		COALESCE(display_unit,''), COALESCE(display_decimals,''),
		COALESCE(sample_rate,'')
		FROM config WHERE id = 1`)

	var target, deviceUnit, dispUnit, dispDec, rate string
	if err := row.Scan(&target, &deviceUnit, &dispUnit, &dispDec, &rate); err != nil {
		if err != sql.ErrNoRows {
			// Corrupt row: fall back to defaults rather than failing startup.
			fmt.Println("warning: could not read config:", err)
		}
		return cfg
	}

	cfg.PreferredPort = target
	if deviceUnit != "" {
		cfg.DeviceUnit = deviceUnit
	}
	if dispUnit != "" {
		cfg.DisplayUnit = dispUnit
	}
	setInt(&cfg.DisplayDecimals, dispDec)
	setInt(&cfg.SampleRateSPS, rate)
	return cfg
}

// SaveConfig persists the configuration.
func (d *DB) SaveConfig(cfg AppConfig) error {
	_, err := d.sql.Exec(`INSERT OR REPLACE INTO config
		(id, usb_target, device_unit, display_unit, display_decimals, sample_rate)
		VALUES (1, ?, ?, ?, ?, ?)`,
		cfg.PreferredPort,
		cfg.DeviceUnit,
		cfg.DisplayUnit,
		strconv.Itoa(cfg.DisplayDecimals),
		strconv.Itoa(cfg.SampleRateSPS),
	)
	if err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

func setInt(dst *int, s string) {
	if s == "" {
		return
	}
	if v, err := strconv.Atoi(s); err == nil {
		*dst = v
	}
}
