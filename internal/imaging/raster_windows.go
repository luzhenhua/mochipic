package imaging

/*
#cgo pkg-config: vips
#include <vips/vips.h>

// Keep the native image and its borrowed input buffer inside this call. The
// caller receives an independent sRGB pixel buffer before the input unpins.
static int windows_raster(const void *data, size_t size, int kind, int max_side,
	size_t max_pixels, gboolean decode, int *w, int *h, int *bands,
	unsigned char **pixels) {
	VipsImage *src = NULL, *oriented = NULL, *rgb = NULL, *u8 = NULL;
	int ret = -1;
	int err;
	if (kind == 1) {
		err = vips_dcrawload_buffer((void *)data, size, &src, NULL);
	} else if (kind == 2) {
		err = vips_jp2kload_buffer((void *)data, size, &src, NULL);
	} else if (kind == 3) {
		err = vips_pngload_buffer((void *)data, size, &src, NULL);
	} else {
		err = vips_magickload_buffer((void *)data, size, &src, "page", 0, "n", 1, NULL);
	}
	if (err) goto done;
	int sw = vips_image_get_width(src), sh = vips_image_get_height(src);
	if (sw <= 0 || sh <= 0 || sw > max_side || sh > max_side || (size_t)sw > max_pixels / sh) {
		ret = -2;
		goto done;
	}
	if (!decode) {
		*w = sw;
		*h = sh;
		*bands = vips_image_hasalpha(src) ? 4 : 3;
		ret = 0;
		goto done;
	}
	if (vips_autorot(src, &oriented, NULL)) goto done;
	if (vips_image_get_typeof(oriented, VIPS_META_ICC_NAME) != 0) {
		if (vips_icc_transform(oriented, &rgb, "srgb", "embedded", TRUE, NULL)) goto done;
	} else if (vips_colourspace(oriented, &rgb, VIPS_INTERPRETATION_sRGB, NULL)) goto done;
	if (vips_cast(rgb, &u8, VIPS_FORMAT_UCHAR, NULL)) goto done;
	*w = vips_image_get_width(u8);
	*h = vips_image_get_height(u8);
	*bands = vips_image_get_bands(u8);
	if (*bands != 3 && *bands != 4) goto done;
	size_t actual = 0;
	*pixels = vips_image_write_to_memory(u8, &actual);
	if (!*pixels || actual != (size_t)*w * *h * *bands) {
		g_free(*pixels);
		*pixels = NULL;
		goto done;
	}
	ret = 0;
done:
	if (u8) g_object_unref(u8);
	if (rgb) g_object_unref(rgb);
	if (oriented) g_object_unref(oriented);
	if (src) g_object_unref(src);
	vips_error_clear();
	return ret;
}
*/
import "C"

import (
	"bytes"
	"encoding/binary"
	"runtime"
	"unsafe"

	"imageflow/internal/l10n"

	"github.com/davidbyttow/govips/v2/vips"
)

func systemRaster(data []byte, format Format, limit int, decode bool, _ string) (Info, *vips.ImageRef, error) {
	var kind C.int
	switch format {
	case RAW:
		kind = 1
	case JP2:
		kind = 2
	case ICO:
		var pngFrame bool
		data, pngFrame = largestIconFrame(data)
		if data == nil {
			return Info{}, nil, l10n.New("error.corruptOrWrongExtension")
		}
		if pngFrame {
			kind = 3
		}
	}
	var pin runtime.Pinner
	pin.Pin(&data[0])
	defer pin.Unpin()
	var width, height, bands C.int
	var pixels *C.uchar
	status := C.windows_raster(unsafe.Pointer(&data[0]), C.size_t(len(data)), kind, MaxSide, C.size_t(limit),
		C.gboolean(boolInt(decode)), &width, &height, &bands, &pixels)
	if status == -2 {
		return Info{}, nil, imageBounds(limit)
	}
	if status != 0 {
		if format == RAW {
			return Info{}, nil, l10n.New("error.rawNotSupported")
		}
		return Info{}, nil, l10n.New("error.corruptOrWrongExtension")
	}
	info := Info{Width: int(width), Height: int(height), Format: format, Alpha: bands == 4}
	if !decode {
		return info, nil, nil
	}
	defer C.g_free(C.gpointer(pixels))
	raw := unsafe.Slice((*byte)(unsafe.Pointer(pixels)), info.Width*info.Height*int(bands))
	im, err := vips.NewImageFromMemory(raw, info.Width, info.Height, int(bands), vips.BandFormatUchar, vips.InterpretationSRGB)
	return info, im, err
}

func largestIconFrame(data []byte) ([]byte, bool) {
	const headerSize, entrySize = 6, 16
	if len(data) < headerSize || !bytes.HasPrefix(data, []byte{0, 0, 1, 0}) {
		return nil, false
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	directoryEnd := headerSize + count*entrySize
	if directoryEnd > len(data) {
		return nil, false
	}

	var bestEntry, bestFrame []byte
	var bestArea, bestDepth int
	for offset := headerSize; offset < directoryEnd; offset += entrySize {
		entry := data[offset : offset+entrySize]
		length := int(binary.LittleEndian.Uint32(entry[8:12]))
		start := int(binary.LittleEndian.Uint32(entry[12:16]))
		if length <= 0 || start < directoryEnd || start > len(data) || length > len(data)-start {
			continue
		}
		width, height := int(entry[0]), int(entry[1])
		if width == 0 {
			width = 256
		}
		if height == 0 {
			height = 256
		}
		area := width * height
		depth := int(binary.LittleEndian.Uint16(entry[6:8]))
		if area > bestArea || (area == bestArea && depth > bestDepth) {
			bestEntry, bestFrame = entry, data[start:start+length]
			bestArea, bestDepth = area, depth
		}
	}
	if bestFrame == nil {
		return nil, false
	}
	if bytes.HasPrefix(bestFrame, []byte("\x89PNG\r\n\x1a\n")) {
		return bestFrame, true
	}

	icon := make([]byte, headerSize+entrySize+len(bestFrame))
	copy(icon, data[:4])
	binary.LittleEndian.PutUint16(icon[4:6], 1)
	copy(icon[headerSize:], bestEntry)
	binary.LittleEndian.PutUint32(icon[headerSize+12:headerSize+16], headerSize+entrySize)
	copy(icon[headerSize+entrySize:], bestFrame)
	return icon, false
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func systemAnimation(data []byte, format Format, part, limit int, decode bool) (frameMeta, Info, *vips.ImageRef, error) {
	return heifSequence(data, format, part, limit, decode)
}
