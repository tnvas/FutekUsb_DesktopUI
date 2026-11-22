// Package logging configures the global logger with file rotation.
package logging

import (
	"log"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Init routes the standard logger to a rotated log file in dir.
func Init(dir, appName string) {
	log.SetFlags(log.LstdFlags)
	log.SetOutput(&lumberjack.Logger{
		Filename:   filepath.Join(dir, appName+".log"),
		MaxSize:    2, // MB
		MaxBackups: 5,
		MaxAge:     28, // days
	})
}
