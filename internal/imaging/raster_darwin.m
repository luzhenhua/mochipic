#include "raster_darwin.h"
#import <CoreGraphics/CoreGraphics.h>
#import <CoreImage/CoreImage.h>
#import <Foundation/Foundation.h>
#import <ImageIO/ImageIO.h>
#import <UniformTypeIdentifiers/UniformTypeIdentifiers.h>
#include <math.h>
#include <stdlib.h>

static bool valid_size(double w, double h, size_t side, size_t limit) {
	return w >= 1 && h >= 1 && w <= side && h <= side && w * h <= limit;
}

static int draw_image(CGImageRef image, Raster *out) {
	if (!image || CGImageGetWidth(image) != out->width || CGImageGetHeight(image) != out->height)
		return RASTER_BAD;
	CGColorSpaceRef space = CGColorSpaceCreateWithName(kCGColorSpaceSRGB);
	size_t area = out->width * out->height;
	unsigned char *pixels = calloc(area, 4);
	CGContextRef context = pixels && space ? CGBitmapContextCreate(pixels, out->width, out->height,
		8, out->width * 4, space, kCGImageAlphaPremultipliedLast | kCGBitmapByteOrder32Big) : NULL;
	if (space) CGColorSpaceRelease(space);
	if (!context) {
		free(pixels);
		return RASTER_BAD;
	}
	CGContextSetBlendMode(context, kCGBlendModeCopy);
	CGContextDrawImage(context, CGRectMake(0, 0, out->width, out->height), image);
	CGContextRelease(context);
	// Core Graphics returns premultiplied RGBA; libvips expects straight alpha.
	for (size_t i = 0; i < area * 4; i += 4) {
		unsigned int a = pixels[i + 3];
		if (a != 255) out->alpha = true;
		if (a && a != 255)
			for (size_t c = 0; c < 3; c++)
				pixels[i + c] = (pixels[i + c] * 255 + a / 2) / a;
	}
	out->pixels = pixels;
	return RASTER_OK;
}

static long number(CFDictionaryRef props, CFStringRef key) {
	CFNumberRef value = CFDictionaryGetValue(props, key);
	long n = 0;
	if (value && CFGetTypeID(value) == CFNumberGetTypeID())
		CFNumberGetValue(value, kCFNumberLongType, &n);
	return n;
}

static int read_imageio(CGImageSourceRef source, CFDictionaryRef options, CFStringRef expected,
	enum RasterKind kind, size_t side, size_t limit, bool decode, Raster *out) {
	CFStringRef type = CGImageSourceGetType(source);
	if (!type || !CFEqual(type, expected)) return RASTER_BAD;
	size_t count = CGImageSourceGetCount(source);
	if (!count || count > 1024) return RASTER_BAD;
	if (kind == RASTER_BMP && count > 1) return RASTER_MULTIPLE;
	// ICO sizes represent the same icon. For PSD, index 0 is the saved composite.
	size_t best = 0, area = 0;
	int result = RASTER_BAD;
	for (size_t i = 0; i < (kind == RASTER_ICO ? count : 1); i++) {
		CFDictionaryRef properties = CGImageSourceCopyPropertiesAtIndex(source, i, options);
		if (!properties) continue;
		long w = number(properties, kCGImagePropertyPixelWidth);
		long h = number(properties, kCGImagePropertyPixelHeight);
		long orientation = number(properties, kCGImagePropertyOrientation);
		CFRelease(properties);
		if (w <= 0 || h <= 0) continue;
		if (!valid_size(w, h, side, limit)) {
			result = RASTER_BOUNDS;
			continue;
		}
		if ((size_t)w * h > area && CGImageSourceGetStatusAtIndex(source, i) == kCGImageStatusComplete) {
			best = i;
			area = (size_t)w * h;
			out->width = w;
			out->height = h;
			out->orientation = orientation >= 1 && orientation <= 8 ? (int)orientation : 1;
		}
	}
	if (!area) return result;
	if (!decode) return RASTER_OK;
	CGImageRef image = CGImageSourceCreateImageAtIndex(source, best, options);
	if (image && kind == RASTER_PSD && CGImageGetAlphaInfo(image) == kCGImageAlphaLast) {
		// Image I/O's PSD decoder can return premultiplied samples while
		// marking them as straight alpha. Rewrap the provider before drawing;
		// otherwise partially transparent pixels get darkened a second time.
		CGBitmapInfo info = (CGImageGetBitmapInfo(image) & ~kCGBitmapAlphaInfoMask) | kCGImageAlphaPremultipliedLast;
		CGImageRef corrected = CGImageCreate(CGImageGetWidth(image), CGImageGetHeight(image),
			CGImageGetBitsPerComponent(image), CGImageGetBitsPerPixel(image), CGImageGetBytesPerRow(image),
			CGImageGetColorSpace(image), info, CGImageGetDataProvider(image), CGImageGetDecode(image),
			CGImageGetShouldInterpolate(image), CGImageGetRenderingIntent(image));
		if (corrected) {
			CGImageRelease(image);
			image = corrected;
		}
	}
	result = draw_image(image, out);
	if (image) CGImageRelease(image);
	return result;
}

static int read_raw(CFDataRef data, CGImageSourceRef source, size_t side, size_t limit, bool decode, Raster *out) {
	CFStringRef type = CGImageSourceGetType(source);
	if (!type) return RASTER_RAW_UNSUPPORTED;
	NSString *hint = (NSString *)type;
	if (![[UTType typeWithIdentifier:hint] conformsToType:UTTypeRAWImage]) return RASTER_BAD;
	CIRAWFilter *filter = [CIRAWFilter filterWithImageData:(NSData *)data identifierHint:hint];
	if (!filter) return RASTER_RAW_UNSUPPORTED;
	// RAW 9 can need an on-demand model download. Use the newest built-in
	// decoder version available for this file so opening images stays offline.
	NSSet<NSString *> *localVersions = [NSSet setWithArray:@[
		CIRAWDecoderVersion8, CIRAWDecoderVersion8DNG, CIRAWDecoderVersion7,
		@"7.dng", CIRAWDecoderVersion6, CIRAWDecoderVersion6DNG]];
	NSString *version = nil;
	for (NSString *v in [filter.supportedDecoderVersions reverseObjectEnumerator]) {
		if ([localVersions containsObject:v]) {
			version = v;
			break;
		}
	}
	if (!version) return RASTER_RAW_UNSUPPORTED;
	filter.decoderVersion = version;
	CGSize native = filter.nativeSize;
	if (native.width <= 0 || native.height <= 0) return RASTER_RAW_UNSUPPORTED;
	if (!valid_size(native.width, native.height, side, limit)) return RASTER_BOUNDS;
	filter.extendedDynamicRangeAmount = 0;
	filter.draftModeEnabled = NO;
	CIImage *rendered = filter.outputImage; // Uses the RAW pixels and applies file orientation.
	if (!rendered) return RASTER_RAW_UNSUPPORTED;
	CGRect rect = CGRectIntegral(rendered.extent);
	if (!valid_size(rect.size.width, rect.size.height, side, limit)) return RASTER_BOUNDS;
	out->width = (size_t)rect.size.width;
	out->height = (size_t)rect.size.height;
	out->orientation = 1;
	if (!decode) return RASTER_OK;

	CGColorSpaceRef space = CGColorSpaceCreateWithName(kCGColorSpaceSRGB);
	if (!space) return RASTER_RAW_UNSUPPORTED;
	CIContext *context = [CIContext contextWithOptions:@{kCIContextCacheIntermediates: @NO}];
	CGImageRef image = [context createCGImage:rendered fromRect:rect format:kCIFormatRGBA8 colorSpace:space];
	CGColorSpaceRelease(space);
	int result = image ? draw_image(image, out) : RASTER_RAW_UNSUPPORTED;
	if (image) CGImageRelease(image);
	return result;
}

int raster_read(const unsigned char *bytes, size_t length, enum RasterKind kind, const char *raw_hint,
	size_t side, size_t limit, bool decode, Raster *out) {
	@autoreleasepool {
		CFDataRef data = kind == RASTER_RAW ? CFDataCreate(NULL, bytes, length)
			: CFDataCreateWithBytesNoCopy(NULL, bytes, length, kCFAllocatorNull);
		if (!data) return RASTER_BAD;
		NSString *hint = raw_hint ? [NSString stringWithUTF8String:raw_hint] : nil;
		const void *keys[] = { kCGImageSourceShouldCache, kCGImageSourceTypeIdentifierHint };
		const void *values[] = { kCFBooleanFalse, hint };
		CFDictionaryRef options = CFDictionaryCreate(NULL, keys, values, hint ? 2 : 1,
			&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
		CGImageSourceRef source = CGImageSourceCreateWithData(data, options);
		int result = RASTER_BAD;
		if (source) {
			switch (kind) {
			case RASTER_RAW:
				result = read_raw(data, source, side, limit, decode, out);
				break;
			case RASTER_ICO:
				result = read_imageio(source, options, CFSTR("com.microsoft.ico"), kind, side, limit, decode, out);
				break;
			case RASTER_PSD:
				result = read_imageio(source, options, CFSTR("com.adobe.photoshop-image"), kind, side, limit, decode, out);
				break;
			case RASTER_J2K:
				result = read_imageio(source, options, CFSTR("public.jpeg-2000"), kind, side, limit, decode, out);
				break;
			case RASTER_BMP:
				result = read_imageio(source, options, CFSTR("com.microsoft.bmp"), kind, side, limit, decode, out);
				break;
			}
			CFRelease(source);
		} else if (kind == RASTER_RAW) {
			result = RASTER_RAW_UNSUPPORTED;
		}
		if (options) CFRelease(options);
		CFRelease(data);
		return result;
	}
}

static CFDictionaryRef dictionary(CFDictionaryRef from, CFStringRef key) {
	CFTypeRef value = from ? CFDictionaryGetValue(from, key) : NULL;
	return value && CFGetTypeID(value) == CFDictionaryGetTypeID() ? value : NULL;
}

static bool real_number(CFDictionaryRef props, CFStringRef key, double *out) {
	CFNumberRef value = props ? CFDictionaryGetValue(props, key) : NULL;
	double n;
	if (!value || CFGetTypeID(value) != CFNumberGetTypeID() ||
		!CFNumberGetValue(value, kCFNumberDoubleType, &n) || !isfinite(n)) return false;
	*out = n;
	return true;
}

static CFDictionaryRef animation_record(CFDictionaryRef props, size_t part) {
	CFTypeRef records = props ? CFDictionaryGetValue(props, CFSTR("FrameInfo")) : NULL;
	if (!records || CFGetTypeID(records) != CFArrayGetTypeID() || part >= (size_t)CFArrayGetCount(records)) return NULL;
	CFTypeRef record = CFArrayGetValueAtIndex(records, part);
	return record && CFGetTypeID(record) == CFDictionaryGetTypeID() ? record : NULL;
}

int animation_read(const unsigned char *bytes, size_t length, enum AnimationKind kind,
	size_t part, size_t max_frames, size_t side, size_t limit,
	bool decode, AnimationFrame *out) {
	@autoreleasepool {
		int result = RASTER_BAD;
		CFDataRef data = CFDataCreateWithBytesNoCopy(NULL, bytes, length, kCFAllocatorNull);
		if (!data) return result;
		const void *key = kCGImageSourceShouldCache, *value = kCFBooleanFalse;
		CFDictionaryRef options = CFDictionaryCreate(NULL, &key, &value, 1,
			&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
		CGImageSourceRef source = CGImageSourceCreateWithData(data, options);
		CFDictionaryRef global = NULL, props = NULL;
		if (!source) goto done;
		CFStringRef expected = kind == ANIMATION_AVIF ? CFSTR("public.avis") : CFSTR("public.heics");
		CFStringRef type = CGImageSourceGetType(source);
		if (!type || !CFEqual(type, expected)) goto done;
		size_t total = CGImageSourceGetCount(source);
		if (!total || !part || part > total) goto done;
		if (total > max_frames) {
			result = RASTER_LIMIT;
			goto done;
		}
		size_t index = part - 1;
		props = CGImageSourceCopyPropertiesAtIndex(source, index, options);
		if (!props) goto done;
		long w = number(props, kCGImagePropertyPixelWidth), h = number(props, kCGImagePropertyPixelHeight);
		long orientation = number(props, kCGImagePropertyOrientation);
		if (!valid_size(w, h, side, limit)) {
			result = RASTER_BOUNDS;
			goto done;
		}
		CFStringRef dict_key = kind == ANIMATION_AVIF ? kCGImagePropertyAVISDictionary : kCGImagePropertyHEICSDictionary;
		global = CGImageSourceCopyProperties(source, options);
		CFDictionaryRef details = dictionary(global, dict_key);
		// A timed sequence must expose animation metadata. This prevents an
		// older decoder from quietly supplying only the container's poster.
		if (!details) goto done;
		CFDictionaryRef timing = dictionary(props, dict_key);
		double delay = 0, loop = 1;
		if (!real_number(timing, CFSTR("UnclampedDelayTime"), &delay) && !real_number(timing, CFSTR("DelayTime"), &delay)) {
			timing = animation_record(details, part - 1);
			if (!real_number(timing, CFSTR("UnclampedDelayTime"), &delay) && !real_number(timing, CFSTR("DelayTime"), &delay))
				goto done;
		}
		real_number(details, CFSTR("LoopCount"), &loop);
		if (delay < 0 || delay > 86400 || loop < 0 || loop > UINT32_MAX) goto done;
		out->count = total;
		out->delay_ms = lround(delay * 1000);
		out->loop = (long)loop;
		out->raster.width = w;
		out->raster.height = h;
		out->raster.orientation = orientation >= 1 && orientation <= 8 ? (int)orientation : 1;
		if (!decode) {
			result = RASTER_OK;
			goto done;
		}
		if (CGImageSourceGetStatusAtIndex(source, index) != kCGImageStatusComplete) goto done;
		CGImageRef image = CGImageSourceCreateImageAtIndex(source, index, options);
		result = draw_image(image, &out->raster);
		if (image) CGImageRelease(image);
done:
		if (props) CFRelease(props);
		if (global) CFRelease(global);
		if (source) CFRelease(source);
		if (options) CFRelease(options);
		CFRelease(data);
		return result;
	}
}

struct TIFFBook {
	CFMutableDataRef buffer;
	CGImageDestinationRef destination;
};

TIFFBook *tiff_book_new(size_t count) {
	TIFFBook *book = calloc(1, sizeof(TIFFBook));
	if (!book) return NULL;
	book->buffer = CFDataCreateMutable(NULL, 0);
	if (book->buffer)
		book->destination = CGImageDestinationCreateWithData(book->buffer, CFSTR("public.tiff"), count, NULL);
	if (!book->destination) {
		tiff_book_free(book);
		return NULL;
	}
	return book;
}

bool tiff_book_add(TIFFBook *book, const unsigned char *png, size_t size) {
	@autoreleasepool {
		// The destination may retain the input CGImage until finalization.
		CFDataRef data = CFDataCreate(NULL, png, size);
		if (!data) return false;
		CGImageSourceRef source = CGImageSourceCreateWithData(data, NULL);
		if (!source) {
			CFRelease(data);
			return false;
		}
		CGImageRef image = CGImageSourceCreateImageAtIndex(source, 0, NULL);
		if (image) {
			NSDictionary *options = @{(NSString *)kCGImagePropertyTIFFDictionary:
				@{(NSString *)kCGImagePropertyTIFFCompression: @5}}; // LZW
			CGImageDestinationAddImage(book->destination, image, (CFDictionaryRef)options);
			CGImageRelease(image);
		}
		CFRelease(source);
		CFRelease(data);
		return image != NULL;
	}
}

const unsigned char *tiff_book_finish(TIFFBook *book, size_t *size) {
	if (!CGImageDestinationFinalize(book->destination)) return NULL;
	*size = (size_t)CFDataGetLength(book->buffer);
	return CFDataGetBytePtr(book->buffer);
}

void tiff_book_free(TIFFBook *book) {
	if (book->destination) CFRelease(book->destination);
	if (book->buffer) CFRelease(book->buffer);
	free(book);
}
