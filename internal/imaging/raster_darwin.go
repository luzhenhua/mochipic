package imaging

/*
#cgo LDFLAGS: -framework CoreFoundation -framework CoreGraphics -framework Foundation -framework ImageIO -framework CoreImage -framework UniformTypeIdentifiers
#include "raster_darwin.h"
#include <stdlib.h>
*/
import "C"

import (
	"unsafe"

	"imageflow/internal/l10n"
	"imageflow/internal/series"

	"github.com/davidbyttow/govips/v2/vips"
)

func systemRaster(data []byte, format Format, limit int, decode bool, rawHint string) (Info, *vips.ImageRef, error) {
	kind := C.enum_RasterKind(C.RASTER_BMP)
	switch format {
	case ICO:
		kind = C.RASTER_ICO
	case PSD:
		kind = C.RASTER_PSD
	case JP2:
		kind = C.RASTER_J2K
	case RAW:
		kind = C.RASTER_RAW
	}
	var hint *C.char
	if rawHint != "" {
		hint = C.CString(rawHint)
		defer C.free(unsafe.Pointer(hint))
	}
	var result C.Raster
	switch C.raster_read((*C.uchar)(unsafe.Pointer(&data[0])), C.size_t(len(data)),
		kind, hint, MaxSide, C.size_t(limit), C.bool(decode), &result) {
	case C.RASTER_BOUNDS:
		return Info{}, nil, imageBounds(limit)
	case C.RASTER_MULTIPLE:
		return Info{}, nil, l10n.New("error.multipleImages")
	case C.RASTER_RAW_UNSUPPORTED:
		return Info{}, nil, l10n.New("error.rawNotSupported")
	case C.RASTER_OK:
	default:
		return Info{}, nil, l10n.New("error.corruptOrWrongExtension")
	}
	info := Info{Format: format, Width: int(result.width), Height: int(result.height), Alpha: bool(result.alpha)}
	if !decode {
		return info, nil, nil
	}
	defer C.free(unsafe.Pointer(result.pixels))
	pixels := unsafe.Slice((*byte)(unsafe.Pointer(result.pixels)), info.Width*info.Height*4)
	im, err := imageFromRGBA(pixels, info)
	if err != nil {
		return info, nil, err
	}
	if result.orientation > 1 {
		err = im.SetOrientation(int(result.orientation))
		if err == nil {
			err = im.AutoRotate()
		}
		if err != nil {
			im.Close()
			return info, nil, err
		}
		info.Width, info.Height = im.Width(), im.Height()
	}
	return info, im, nil
}

func systemAnimation(data []byte, format Format, part, limit int, decode bool) (frameMeta, Info, *vips.ImageRef, error) {
	kind := C.enum_AnimationKind(C.ANIMATION_AVIF)
	if format == HEIF {
		kind = C.ANIMATION_HEIF
	}
	var out C.AnimationFrame
	switch C.animation_read((*C.uchar)(unsafe.Pointer(&data[0])), C.size_t(len(data)), kind,
		C.size_t(part), series.MaxParts, MaxSide,
		C.size_t(limit), C.bool(decode), &out) {
	case C.RASTER_OK:
	case C.RASTER_LIMIT:
		return frameMeta{}, Info{}, nil, seriesLimit()
	case C.RASTER_BOUNDS:
		return frameMeta{}, Info{}, nil, imageBounds(limit)
	default:
		return frameMeta{}, Info{}, nil, l10n.New("error.animationDecode")
	}
	meta := frameMeta{series: series.Info{Kind: series.Frames, Count: int(out.count)}, delay: int(out.delay_ms), loop: int(out.loop)}
	info := Info{Format: format, Width: int(out.raster.width), Height: int(out.raster.height), Alpha: bool(out.raster.alpha)}
	if !decode {
		return meta, info, nil, nil
	}
	defer C.free(unsafe.Pointer(out.raster.pixels))
	pixels := unsafe.Slice((*byte)(unsafe.Pointer(out.raster.pixels)), info.Width*info.Height*4)
	im, err := imageFromRGBA(pixels, info)
	if err == nil && out.raster.orientation > 1 {
		err = im.SetOrientation(int(out.raster.orientation))
		if err == nil {
			err = im.AutoRotate()
		}
	}
	if err != nil {
		if im != nil {
			im.Close()
		}
		return meta, info, nil, err
	}
	info.Width, info.Height = im.Width(), im.Height()
	return meta, info, im, nil
}
