# UsbSens3 build targets
#
# First time here? Run:  make setup

VERSION    ?= v2.0.0
SIMULATION ?= false
LDFLAGS     = -X 'futek_usb220/internal/settings.RELEASE_VERSION=$(VERSION)' \
              -X 'futek_usb220/internal/settings.SIMULATE_USB=$(SIMULATION)'

GOPATH_BIN := $(shell go env GOPATH)/bin
FYNE_CROSS ?= $(GOPATH_BIN)/fyne-cross

.PHONY: setup setup-linux-deps run sim test vet app win_build pack zip_windows zip_linux release

# ---------------------------------------------------------------- setup ----

# Install the development prerequisites (Go tools + module cache).
# Run once after cloning; safe to re-run.
setup:
	@command -v go >/dev/null 2>&1 || { echo "ERROR: Go is not installed — https://go.dev/dl/"; exit 1; }
	@echo "==> Downloading Go module dependencies…"
	go mod download
	@echo "==> Installing fyne-cross (Windows cross-builds)…"
	go install github.com/fyne-io/fyne-cross@latest
	@command -v gcc >/dev/null 2>&1 || echo "WARNING: gcc not found — needed to build Fyne. Run: make setup-linux-deps"
	@command -v docker >/dev/null 2>&1 || echo "WARNING: docker not found — needed by 'make win_build' (fyne-cross). See https://docs.docker.com/engine/install/"
	@echo ""
	@echo "Setup complete. Common targets:"
	@echo "  make sim       run with a simulated sensor (no hardware)"
	@echo "  make run       build and run against real hardware"
	@echo "  make test      unit tests"
	@echo "  make release   Linux + Windows binaries and packages"

# System packages Fyne needs on Debian/Ubuntu-based distros (requires sudo).
setup-linux-deps:
	sudo apt-get update
	sudo apt-get install -y gcc libgl1-mesa-dev xorg-dev

# ----------------------------------------------------------------- build ----

# Build and run for the current OS.
run:
	mkdir -p bin
	go build -ldflags="$(LDFLAGS)" -o bin/UsbSens3 main.go
	./bin/UsbSens3

# Run with a simulated sensor (no hardware needed).
sim:
	$(MAKE) run SIMULATION=true

test:
	go test ./...

vet:
	go vet ./...

# Linux binary.
app:
	mkdir -p bin
	go build -ldflags="$(LDFLAGS)" -o bin/UsbSens3 main.go

# Windows binary via fyne-cross (Docker based; run 'make setup' first).
win_build:
	@test -x "$(FYNE_CROSS)" || command -v fyne-cross >/dev/null 2>&1 || { \
		echo "ERROR: fyne-cross not installed. Run: make setup"; exit 1; }
	@command -v docker >/dev/null 2>&1 || { \
		echo "ERROR: docker is required for Windows cross-builds. See https://docs.docker.com/engine/install/"; exit 1; }
	$(FYNE_CROSS) windows -arch=amd64 -app-version=$(subst v,,$(VERSION))
	mv fyne-cross/bin/windows-amd64/UsbSens3.exe bin/UsbSens3.exe

# --------------------------------------------------------------- package ----

PACKAGE_NAME_WIN   := futek-usb220-$(VERSION)-win64
PACKAGE_NAME_LINUX := futek-usb220-$(VERSION)-linux

pack: zip_windows zip_linux

zip_windows:
	mkdir -p $(PACKAGE_NAME_WIN) zip
	cp bin/UsbSens3.exe $(PACKAGE_NAME_WIN)/UsbSens3.exe
	zip -r zip/$(PACKAGE_NAME_WIN).zip $(PACKAGE_NAME_WIN)
	rm -rf $(PACKAGE_NAME_WIN)
	@echo "Windows package created: zip/$(PACKAGE_NAME_WIN).zip"

zip_linux:
	mkdir -p $(PACKAGE_NAME_LINUX) zip
	cp bin/UsbSens3 $(PACKAGE_NAME_LINUX)/UsbSens3
	tar -czf zip/$(PACKAGE_NAME_LINUX).tar.gz $(PACKAGE_NAME_LINUX)
	rm -rf $(PACKAGE_NAME_LINUX)
	@echo "Linux package created: zip/$(PACKAGE_NAME_LINUX).tar.gz"

release: app win_build pack
