#pragma once
#include <stdbool.h>
#include <stddef.h>

enum RasterStatus { RASTER_OK, RASTER_BAD, RASTER_BOUNDS, RASTER_MULTIPLE, RASTER_RAW_UNSUPPORTED, RASTER_LIMIT };
enum RasterKind { RASTER_BMP, RASTER_ICO, RASTER_PSD, RASTER_J2K, RASTER_RAW };
enum AnimationKind { ANIMATION_AVIF, ANIMATION_HEIF };

typedef struct {
	size_t width, height;
	int orientation;
	bool alpha;
	unsigned char *pixels;
} Raster;

int raster_read(const unsigned char *bytes, size_t length, enum RasterKind kind, const char *raw_hint,
	size_t side, size_t limit, bool decode, Raster *out);

typedef struct {
	Raster raster;
	size_t count;
	long delay_ms, loop;
} AnimationFrame;

int animation_read(const unsigned char *bytes, size_t length, enum AnimationKind kind,
	size_t part, size_t max_frames, size_t side, size_t limit,
	bool decode, AnimationFrame *out);

typedef struct TIFFBook TIFFBook;
TIFFBook *tiff_book_new(size_t count);
bool tiff_book_add(TIFFBook *book, const unsigned char *png, size_t size);
const unsigned char *tiff_book_finish(TIFFBook *book, size_t *size);
void tiff_book_free(TIFFBook *book);
