package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Theme mode identifiers persisted in preferences.
const (
	themeDark   = "Dark"
	themeLight  = "Light"
	themeSystem = "System"

	themePrefKey = "ui.theme"
)

var themeModes = []string{themeDark, themeLight, themeSystem}

// variantTheme forces the standard theme into a fixed light or dark variant.
// With mode "System" the OS preference is followed as usual.
type variantTheme struct {
	base    fyne.Theme
	variant fyne.ThemeVariant
	force   bool
}

func (t *variantTheme) Color(name fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	if t.force {
		v = t.variant
	}
	return t.base.Color(name, v)
}

func (t *variantTheme) Font(style fyne.TextStyle) fyne.Resource { return t.base.Font(style) }
func (t *variantTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return t.base.Icon(name)
}
func (t *variantTheme) Size(name fyne.ThemeSizeName) float32 { return t.base.Size(name) }

// applyThemeMode switches the application appearance and persists the choice.
func (u *UI) applyThemeMode(mode string) {
	base := theme.DefaultTheme()
	switch mode {
	case themeLight:
		u.fyneApp.Settings().SetTheme(&variantTheme{base: base, variant: theme.VariantLight, force: true})
	case themeSystem:
		u.fyneApp.Settings().SetTheme(&variantTheme{base: base})
	default: // dark - easiest on the eyes in range/lab environments
		mode = themeDark
		u.fyneApp.Settings().SetTheme(&variantTheme{base: base, variant: theme.VariantDark, force: true})
	}
	u.fyneApp.Preferences().SetString(themePrefKey, mode)
}

// currentThemeMode returns the stored appearance choice (default dark).
func (u *UI) currentThemeMode() string {
	return u.fyneApp.Preferences().StringWithFallback(themePrefKey, themeDark)
}
