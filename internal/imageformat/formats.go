// Package imageformat is the shared registry for import and export.
package imageformat

import (
	_ "embed"
	"encoding/json"
	"path/filepath"
	"strings"
)

type Format string

const (
	JPEG Format = "JPG"
	PNG  Format = "PNG"
	WebP Format = "WebP"
	AVIF Format = "AVIF"
	HEIF Format = "HEIF"
	TIFF Format = "TIFF"
	BMP  Format = "BMP"
	ICO  Format = "ICO"
	GIF  Format = "GIF"
	JXL  Format = "JPEG XL"
	JP2  Format = "JPEG 2000"
	SVG  Format = "SVG"
	PSD  Format = "PSD"
	RAW  Format = "RAW"
)

// The packager uses the same registry for the macOS document types.
//
//go:embed formats.json
var formatData []byte

type formatSpec struct {
	Format     Format
	Extensions []string
	Types      []string
	Output     string
}

var formats = func() []formatSpec {
	var specs []formatSpec
	if err := json.Unmarshal(formatData, &specs); err != nil {
		panic(err)
	}
	return specs
}()

// FormatOf uses the filename as a hint. The loader verifies the actual data.
func FormatOf(path string) Format {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	for _, spec := range formats {
		for _, supported := range spec.Extensions {
			if ext == supported {
				return spec.Format
			}
		}
	}
	return ""
}

func InputExtensions() []string {
	var extensions []string
	for _, spec := range formats {
		extensions = append(extensions, spec.Extensions...)
	}
	return extensions
}

// RawTypeHint maps a camera file's extension to the corresponding UTI. Some
// RAWs use a TIFF container; the macOS data decoder needs this hint to
// distinguish their sensor data from a regular TIFF.
func RawTypeHint(path string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	for _, spec := range formats {
		if spec.Format == RAW && len(spec.Extensions) == len(spec.Types) {
			for i, candidate := range spec.Extensions {
				if candidate == ext {
					return spec.Types[i]
				}
			}
		}
	}
	return ""
}

func OutputFormats() []Format {
	var list []Format
	for _, spec := range formats {
		if spec.Output != "" {
			list = append(list, spec.Format)
		}
	}
	return list
}

// Extension is the canonical extension of an exportable format.
func (f Format) Extension() string {
	for _, spec := range formats {
		if spec.Format == f {
			return spec.Output
		}
	}
	return ""
}

func SupportsOutputExtension(extension string) bool {
	if extension == "" {
		return false
	}
	for _, spec := range formats {
		if spec.Output == extension {
			return true
		}
	}
	return false
}
