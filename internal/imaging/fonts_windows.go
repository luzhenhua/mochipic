package imaging

/*
#cgo pkg-config: vips
#include <glib.h>
#include <stdlib.h>
*/
import "C"

import (
	"os"
	"path/filepath"
	"unsafe"
)

// InstalledWatermarkFont maps persisted macOS font choices to the closest
// system fonts on Windows, for both previews and exports.
func InstalledWatermarkFont(name string) string {
	switch name {
	case "Helvetica Neue Bold":
		return "Segoe UI Bold"
	case "PingFang SC":
		return "Microsoft YaHei"
	case "Songti SC":
		return "SimSun"
	case "Menlo":
		return "Consolas"
	default:
		return name
	}
}

func ConfigureBundledFonts() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	root := filepath.Dir(exe)
	for key, path := range map[string]string{
		"FONTCONFIG_FILE":       filepath.Join(root, "etc", "fonts", "fonts.conf"),
		"MAGICK_CONFIGURE_PATH": filepath.Join(root, "etc", "ImageMagick-7"),
		"VIPSHOME":              root,
	} {
		if _, err := os.Stat(path); err != nil || os.Getenv(key) != "" {
			continue
		}
		k, v := C.CString(key), C.CString(path)
		C.g_setenv(k, v, 0)
		C.free(unsafe.Pointer(k))
		C.free(unsafe.Pointer(v))
	}
}
