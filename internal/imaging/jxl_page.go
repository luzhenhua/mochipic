package imaging

/*
#cgo pkg-config: vips
#include <vips/vips.h>

// govips 2.20 forwards page/n for TIFF and WebP, but not JPEG XL.
// Decode just the selected page with libvips and bridge back losslessly.
static int jxl_page(const void *data, size_t len, int page, int side, size_t limit,
	void **png, size_t *png_len, char **message) {
	VipsImage *im = NULL, *upright = NULL;
	int result = -1;
	if (vips_jxlload_buffer((void *)data, len, &im, "page", page, "n", 1, NULL)) goto done;
	int w = vips_image_get_width(im), h = vips_image_get_height(im);
	if (w <= 0 || h <= 0 || w > side || h > side || (size_t)w > limit / h) {
		result = 1;
		goto done;
	}
	if (vips_autorot(im, &upright, NULL)) goto done;
	if (vips_pngsave_buffer(upright, png, png_len, "compression", 1, "keep", VIPS_FOREIGN_KEEP_ICC, NULL)) goto done;
	result = 0;
done:
	if (upright) g_object_unref(upright);
	if (im) g_object_unref(im);
	if (result == -1) {
		*message = g_strdup(vips_error_buffer());
		vips_error_clear();
	}
	return result;
}
*/
import "C"

import (
	"errors"
	"unsafe"

	"imageflow/internal/l10n"
)

func jxlPage(data []byte, index, limit int) ([]byte, error) {
	var png unsafe.Pointer
	var size C.size_t
	var message *C.char
	result := C.jxl_page(unsafe.Pointer(&data[0]), C.size_t(len(data)), C.int(index-1), MaxSide, C.size_t(limit), &png, &size, &message)
	defer C.g_free(C.gpointer(png))
	defer C.g_free(C.gpointer(unsafe.Pointer(message)))
	switch result {
	case 0:
		return C.GoBytes(png, C.int(size)), nil
	case 1:
		return nil, imageBounds(limit)
	default:
		return nil, l10n.Wrap("error.readImage", errors.New(C.GoString(message)))
	}
}
