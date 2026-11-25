package imaging

/*
#cgo pkg-config: vips
#include "raster_darwin.h"
#include <vips/vips.h>

static int svg_read(const unsigned char *data, size_t length, double scale,
	size_t side, size_t limit, bool decode, Raster *out, char **message) {
	VipsImage *image = NULL;
	// Use CSS's 96 pixels per inch without changing the natural pixel
	// dimensions. libvips multiplies scale by dpi / 72. No base file is
	// provided, so librsvg cannot resolve external file or network URLs.
	if (vips_svgload_buffer((void *)data, length, &image, "dpi", 96.0,
		"scale", 0.75 * scale, "unlimited", FALSE, NULL)) {
		*message = g_strdup(vips_error_buffer());
		vips_error_clear();
		return RASTER_BAD;
	}
	int status = RASTER_BOUNDS;
	int w = vips_image_get_width(image), h = vips_image_get_height(image);
	if (w > 0 && h > 0 && (size_t)w <= side && (size_t)h <= side && (size_t)w <= limit / h) {
		out->width = w;
		out->height = h;
		out->alpha = true;
		status = RASTER_OK;
		if (decode) {
			size_t size = 0;
			void *pixels = vips_image_write_to_memory(image, &size);
			if (!pixels || size != out->width * out->height * 4) {
				g_free(pixels);
				*message = g_strdup(vips_error_buffer());
				vips_error_clear();
				status = RASTER_BAD;
			} else {
				out->pixels = pixels;
			}
		}
	}
	g_object_unref(image);
	return status;
}
*/
import "C"

import (
	"errors"
	"strings"
	"unsafe"

	"imageflow/internal/l10n"

	"github.com/davidbyttow/govips/v2/vips"
)

func svgRaster(data []byte, limit int, scale float64, decode bool) (Info, *vips.ImageRef, error) {
	var result C.Raster
	var message *C.char
	switch C.svg_read((*C.uchar)(unsafe.Pointer(&data[0])), C.size_t(len(data)), C.double(scale),
		MaxSide, C.size_t(limit), C.bool(decode), &result, &message) {
	case C.RASTER_OK:
	case C.RASTER_BOUNDS:
		return Info{}, nil, imageBounds(limit)
	default:
		if message != nil {
			detail := strings.TrimSpace(C.GoString(message))
			C.g_free(C.gpointer(message))
			if detail != "" {
				return Info{}, nil, l10n.Wrap("error.readImage", errors.New(detail))
			}
		}
		return Info{}, nil, l10n.New("error.corruptOrWrongExtension")
	}
	info := Info{Format: SVG, Width: int(result.width), Height: int(result.height), Alpha: true}
	if !decode {
		return info, nil, nil
	}
	defer C.g_free(C.gpointer(result.pixels))
	im, err := imageFromRGBA(unsafe.Slice((*byte)(unsafe.Pointer(result.pixels)), info.Width*info.Height*4), info)
	return info, im, err
}
