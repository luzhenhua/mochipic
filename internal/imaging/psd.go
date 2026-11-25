package imaging

import (
	"bytes"
	"encoding/binary"
	"io"

	"imageflow/internal/l10n"
)

// Photoshop stores the merged pixels after the color-mode, image-resource,
// and layer/mask sections. Resource 1057 can mark the merged data as a
// placeholder; do not present that placeholder or the embedded thumbnail.
func inspectPSD(r io.ReaderAt, size int64) error {
	bad := l10n.New("error.corruptOrWrongExtension")
	missing := l10n.New("error.psdMissingComposite")
	var h [26]byte
	if _, err := r.ReadAt(h[:], 0); err != nil || !bytes.Equal(h[:6], []byte{'8', 'B', 'P', 'S', 0, 1}) {
		return bad
	}
	channels := int64(binary.BigEndian.Uint16(h[12:14]))
	height, width := int64(binary.BigEndian.Uint32(h[14:18])), int64(binary.BigEndian.Uint32(h[18:22]))
	depth := int64(binary.BigEndian.Uint16(h[22:24]))
	if channels < 1 || channels > 56 || (depth != 1 && depth != 8 && depth != 16 && depth != 32) {
		return bad
	}
	if !validSize(int(width), int(height), MaxInputPixels) {
		return imageBounds(MaxInputPixels)
	}
	decoded := ((width*depth + 7) / 8) * height * channels
	const maxPSDComposite = 512 << 20
	if decoded > maxPSDComposite {
		return l10n.Error("error.decodedImageLimit", l10n.Args{"megabytes": maxPSDComposite >> 20})
	}

	readLength := func(at, end int64) (int64, bool) {
		var b [4]byte
		if at < 0 || at+4 > end {
			return 0, false
		}
		if _, err := r.ReadAt(b[:], at); err != nil {
			return 0, false
		}
		n := int64(binary.BigEndian.Uint32(b[:]))
		return n, n <= end-at-4
	}
	color, ok := readLength(26, size)
	if !ok {
		return bad
	}
	resourcesAt := 26 + 4 + color
	resources, ok := readLength(resourcesAt, size)
	if !ok {
		return bad
	}
	layerAt := resourcesAt + 4 + resources
	// Resource names and payloads are each padded to an even number of bytes.
	for at, count := resourcesAt+4, 0; at < layerAt; count++ {
		var b [7]byte
		if at+12 > layerAt || count >= 16384 {
			return bad
		}
		if _, err := r.ReadAt(b[:], at); err != nil {
			return bad
		}
		lengthAt := at + 6 + ((int64(b[6]) + 2) &^ 1)
		n, ok := readLength(lengthAt, layerAt)
		if !ok {
			return bad
		}
		dataAt := lengthAt + 4
		if string(b[:4]) == "8BIM" && binary.BigEndian.Uint16(b[4:6]) == 1057 {
			var version [5]byte
			if n < int64(len(version)) {
				return bad
			}
			if _, err := r.ReadAt(version[:], dataAt); err != nil {
				return bad
			}
			if version[4] == 0 {
				return missing
			}
		}
		at = dataAt + ((n + 1) &^ 1)
		if at > layerAt {
			return bad
		}
	}
	layers, ok := readLength(layerAt, size)
	if !ok {
		return bad
	}
	pixelsAt := layerAt + 4 + layers
	var compression [2]byte
	if size-pixelsAt <= 2 {
		return missing
	}
	if _, err := r.ReadAt(compression[:], pixelsAt); err != nil {
		return bad
	}
	remaining := size - pixelsAt - 2
	switch binary.BigEndian.Uint16(compression[:]) {
	case 0:
		if decoded > remaining {
			return missing
		}
	case 1:
		if 2*height*channels >= remaining {
			return missing
		}
	case 2, 3:
		// The decoder validates the ZIP payload.
	default:
		return bad
	}
	return nil
}
