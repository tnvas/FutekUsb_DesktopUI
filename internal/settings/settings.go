// Package settings holds build-time metadata injected via -ldflags -X.
package settings

var (
	RELEASE_VERSION = "v2.0.0"
	APP_NAME        = "UsbSens3"
	SIMULATE_USB    = "" // "true" enables the simulated sensor
	DEBUG           = ""
	BUILD_OS        = "" // retained for build-script compatibility; unused
)

// Simulate reports whether the app should run against a simulated sensor.
func Simulate() bool { return SIMULATE_USB == "true" }
