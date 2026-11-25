//go:build windows || heifsequenceqa

package imaging

/*
#cgo pkg-config: libheif vips
#include <libheif/heif.h>
#include <libheif/heif_sequences.h>

#include <glib.h>
#include <stdint.h>
#include <string.h>

typedef struct {
    int width, height, alpha;
    uint32_t track_id, timescale, duration, repeats;
    unsigned char *pixels, *icc;
    size_t icc_size;
} SequenceFrame;

static int sequence_frame(const void *data, size_t size, int part, int side, uint64_t limit,
    uint32_t max_frames, SequenceFrame *out) {
    int ret = -1;
    heif_context *ctx = heif_context_alloc();
    heif_track *track = NULL;
    heif_decoding_options *options = NULL;
    heif_image *image = NULL;
    if (!ctx) return ret;
    heif_security_limits *limits = heif_context_get_security_limits(ctx);
    limits->max_image_size_pixels = limit;
    limits->max_sequence_frames = max_frames;
    limits->max_total_memory = 512ULL << 20;
    heif_context_set_max_decoding_threads(ctx, 1);
    if (heif_context_read_from_memory_without_copy(ctx, data, size, NULL).code != heif_error_Ok) goto done;
    track = heif_context_get_track(ctx, 0);
    if (!track) goto done;
    uint16_t width, height;
    if (heif_track_get_image_resolution(track, &width, &height).code != heif_error_Ok) goto done;
    if (!width || !height || width > side || height > side || (uint64_t)width * height > limit) {
        ret = -2;
        goto done;
    }
    out->width = width;
    out->height = height;
    out->alpha = heif_track_has_alpha_channel(track);
    out->track_id = heif_track_get_id(track);
    out->timescale = heif_track_get_timescale(track);
    out->repeats = heif_track_get_number_of_repetitions(track);
    if (part == 0) { ret = 0; goto done; }
    if (!out->timescale) goto done;
    options = heif_decoding_options_alloc();
    if (!options) goto done;
    options->convert_hdr_to_8bit = 1;
    options->strict_decoding = 1;
    options->ignore_sequence_editlist = 1;
    for (int i = 0; i < part; ++i) {
        if (image) heif_image_release(image);
        image = NULL;
        if (heif_track_decode_next_image(track, &image, heif_colorspace_RGB,
                heif_chroma_interleaved_RGBA, options).code != heif_error_Ok || !image) goto done;
    }
    out->width = heif_image_get_width(image, heif_channel_interleaved);
    out->height = heif_image_get_height(image, heif_channel_interleaved);
    if (out->width <= 0 || out->height <= 0 || out->width > side || out->height > side ||
        (uint64_t)out->width * out->height > limit) { ret = -2; goto done; }
    size_t stride = 0, row = (size_t)out->width * 4;
    const uint8_t *plane = heif_image_get_plane_readonly2(image, heif_channel_interleaved, &stride);
    if (!plane || stride < row) goto done;
    out->pixels = g_try_malloc(row * out->height);
    if (!out->pixels) goto done;
    for (int y = 0; y < out->height; ++y) memcpy(out->pixels + row * y, plane + stride * y, row);
    if (out->alpha && heif_image_is_premultiplied_alpha(image)) {
        for (size_t i = 0, n = row * out->height; i < n; i += 4) {
            unsigned char a = out->pixels[i + 3];
            for (size_t j = i; j < i + 3; ++j) {
                unsigned int v = a ? (out->pixels[j] * 255U + a / 2) / a : 0;
                out->pixels[j] = v > 255 ? 255 : v;
            }
        }
    }
    out->duration = heif_image_get_duration(image);
    out->icc_size = heif_image_get_raw_color_profile_size(image);
    if (out->icc_size > (4 << 20)) goto done;
    if (out->icc_size) {
        out->icc = g_try_malloc(out->icc_size);
        if (!out->icc || heif_image_get_raw_color_profile(image, out->icc).code != heif_error_Ok) goto done;
    }
    ret = 0;
done:
    if (ret && out->pixels) { g_free(out->pixels); out->pixels = NULL; }
    if (ret && out->icc) { g_free(out->icc); out->icc = NULL; }
    if (image) heif_image_release(image);
    if (options) heif_decoding_options_free(options);
    if (track) heif_track_release(track);
    heif_context_free(ctx);
    return ret;
}
*/
import "C"

import (
	"encoding/binary"
	"math"
	"runtime"
	"unsafe"

	"imageflow/internal/l10n"
	"imageflow/internal/series"

	"github.com/davidbyttow/govips/v2/vips"
)

func heifSequence(data []byte, format Format, part, limit int, decode bool) (frameMeta, Info, *vips.ImageRef, error) {
	bad := l10n.New("error.animationDecode")
	var pin runtime.Pinner
	pin.Pin(&data[0])
	defer pin.Unpin()
	var frame C.SequenceFrame
	read := func(index int) error {
		switch C.sequence_frame(unsafe.Pointer(&data[0]), C.size_t(len(data)), C.int(index), MaxSide,
			C.uint64_t(limit), series.MaxParts, &frame) {
		case 0:
			return nil
		case -2:
			return imageBounds(limit)
		default:
			return bad
		}
	}
	if err := read(0); err != nil {
		return frameMeta{}, Info{}, nil, err
	}
	count, indefinite := heifSampleTable(data, uint32(frame.track_id))
	if count > series.MaxParts {
		return frameMeta{}, Info{}, nil, seriesLimit()
	}
	if count < 1 || part < 1 || part > count {
		return frameMeta{}, Info{}, nil, bad
	}
	loops := int(frame.repeats)
	if loops == math.MaxUint32 || indefinite {
		loops = 0 // WebP's infinite loop.
	} else {
		loops = max(1, loops)
	}
	meta := frameMeta{series: series.Info{Kind: series.Frames, Count: count}, loop: loops}
	info := Info{Format: format, Width: int(frame.width), Height: int(frame.height), Alpha: frame.alpha != 0}
	if !decode {
		return meta, info, nil, nil
	}
	if err := read(part); err != nil {
		return frameMeta{}, Info{}, nil, err
	}
	defer C.g_free(C.gpointer(frame.pixels))
	defer C.g_free(C.gpointer(frame.icc))
	meta.delay = int(math.Round(float64(frame.duration) * 1000 / float64(frame.timescale)))
	info.Width, info.Height = int(frame.width), int(frame.height)
	pixels := unsafe.Slice((*byte)(unsafe.Pointer(frame.pixels)), info.Width*info.Height*4)
	im, err := imageFromRGBA(pixels, info)
	if err == nil && frame.icc_size != 0 {
		im.SetBlob("icc-profile-data", C.GoBytes(unsafe.Pointer(frame.icc), C.int(frame.icc_size)))
	}
	return meta, info, im, err
}

// ISO BMFF stores the sample count in stsz or stz2, independent of the edit
// list's loop count. Follow only the track chosen by libheif.
func heifSampleTable(data []byte, id uint32) (count int, indefinite bool) {
	movie := childBox(data, "moov")
	for len(movie) > 0 {
		kind, contents, rest := nextBox(movie)
		movie = rest
		if kind != "trak" {
			continue
		}
		header := childBox(contents, "tkhd")
		if len(header) < 16 {
			continue
		}
		offset := 12
		if header[0] == 1 {
			offset = 20
		}
		if len(header) < offset+8 || binary.BigEndian.Uint32(header[offset:]) != id {
			continue
		}
		table := childBox(childBox(childBox(contents, "mdia"), "minf"), "stbl")
		for _, kind := range []string{"stsz", "stz2"} {
			if sampleSizes := childBox(table, kind); len(sampleSizes) >= 12 {
				editList := childBox(childBox(contents, "edts"), "elst")
				// AVIF encoders also write signed-duration sentinels in tkhd.
				// Honor them only when elst actually asks for repetition.
				if len(editList) >= 8 && editList[3]&1 != 0 {
					duration := header[offset+8:]
					if header[0] == 1 && len(duration) >= 8 {
						value := binary.BigEndian.Uint64(duration)
						indefinite = value == math.MaxInt64 || value == math.MaxUint64
					} else if header[0] == 0 && len(duration) >= 4 {
						value := binary.BigEndian.Uint32(duration)
						indefinite = value == math.MaxInt32 || value == math.MaxUint32
					}
				}
				return int(binary.BigEndian.Uint32(sampleSizes[8:])), indefinite
			}
		}
		break
	}
	return 0, false
}

func childBox(data []byte, name string) []byte {
	for len(data) > 0 {
		kind, payload, rest := nextBox(data)
		if kind == name {
			return payload
		}
		data = rest
	}
	return nil
}

func nextBox(data []byte) (kind string, payload, rest []byte) {
	if len(data) < 8 {
		return "", nil, nil
	}
	header, size := uint64(8), uint64(binary.BigEndian.Uint32(data))
	switch size {
	case 0:
		size = uint64(len(data))
	case 1:
		if len(data) < 16 {
			return "", nil, nil
		}
		header, size = 16, binary.BigEndian.Uint64(data[8:])
	}
	if size < header || size > uint64(len(data)) {
		return "", nil, nil
	}
	return string(data[4:8]), data[header:size], data[size:]
}
