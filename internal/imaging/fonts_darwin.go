package imaging

import (
	"os"
	"path/filepath"
)

func InstalledWatermarkFont(name string) string { return name }

// ConfigureBundledFonts sets the private Fontconfig configuration when running
// from an application bundle. Existing dev overrides are respected.
func ConfigureBundledFonts() {
	if os.Getenv("FONTCONFIG_FILE") != "" {
		return
	}
	if exe, err := os.Executable(); err == nil {
		conf := filepath.Join(filepath.Dir(exe), "..", "Resources", "fonts.conf")
		if _, err = os.Stat(conf); err == nil {
			_ = os.Setenv("FONTCONFIG_FILE", conf)
		}
	}
}
