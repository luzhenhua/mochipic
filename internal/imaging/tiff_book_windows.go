package imaging

/*
#cgo pkg-config: libtiff-4
#include <stdint.h>
#include <tiffio.h>

static int add_tiff_page(TIFF *tiff, unsigned char *pixels, uint32_t width,
    uint32_t height, uint16_t index, uint16_t count) {
    uint16_t alpha = EXTRASAMPLE_UNASSALPHA;
    if (!TIFFSetField(tiff, TIFFTAG_IMAGEWIDTH, width) ||
        !TIFFSetField(tiff, TIFFTAG_IMAGELENGTH, height) ||
        !TIFFSetField(tiff, TIFFTAG_SAMPLESPERPIXEL, 4) ||
        !TIFFSetField(tiff, TIFFTAG_BITSPERSAMPLE, 8) ||
        !TIFFSetField(tiff, TIFFTAG_ORIENTATION, ORIENTATION_TOPLEFT) ||
        !TIFFSetField(tiff, TIFFTAG_PLANARCONFIG, PLANARCONFIG_CONTIG) ||
        !TIFFSetField(tiff, TIFFTAG_PHOTOMETRIC, PHOTOMETRIC_RGB) ||
        !TIFFSetField(tiff, TIFFTAG_COMPRESSION, COMPRESSION_ADOBE_DEFLATE) ||
        !TIFFSetField(tiff, TIFFTAG_EXTRASAMPLES, 1, &alpha) ||
        !TIFFSetField(tiff, TIFFTAG_ROWSPERSTRIP, TIFFDefaultStripSize(tiff, 0)) ||
        !TIFFSetField(tiff, TIFFTAG_SUBFILETYPE, FILETYPE_PAGE) ||
        !TIFFSetField(tiff, TIFFTAG_PAGENUMBER, index, count)) return 0;
    for (uint32_t y = 0; y < height; ++y) {
        if (TIFFWriteScanline(tiff, pixels + (size_t)y * width * 4, y, 0) < 0) return 0;
    }
    return TIFFWriteDirectory(tiff);
}
*/
import "C"

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"unsafe"

	"imageflow/internal/l10n"

	"golang.org/x/sys/windows"
)

type tiffBook struct {
	native       *C.TIFF
	dir, file    string
	count, added int
}

func newTIFFBook(count int) (tiffBook, error) {
	dir, err := os.MkdirTemp("", "mochipic-tiff-")
	if err != nil {
		return tiffBook{}, err
	}
	book := tiffBook{dir: dir, file: filepath.Join(dir, "pages.tiff"), count: count}
	name, err := windows.UTF16PtrFromString(book.file)
	if err == nil {
		mode := [...]C.char{'w', 0}
		book.native = C.TIFFOpenW((*C.wchar_t)(unsafe.Pointer(name)), &mode[0])
		if book.native != nil {
			return book, nil
		}
	}
	book.close()
	return tiffBook{}, l10n.New("error.seriesEncode")
}

func (b *tiffBook) close() {
	if b.native != nil {
		C.TIFFClose(b.native)
		b.native = nil
	}
	_ = os.Remove(b.file)
	_ = os.Remove(b.dir)
}

func (b *tiffBook) add(encoded []byte) error {
	im, err := png.Decode(bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	pixels, ok := im.(*image.NRGBA)
	if !ok {
		pixels = image.NewNRGBA(im.Bounds())
		draw.Draw(pixels, pixels.Bounds(), im, im.Bounds().Min, draw.Src)
	}
	size := pixels.Bounds().Size()
	if C.add_tiff_page(b.native, (*C.uchar)(unsafe.Pointer(&pixels.Pix[0])), C.uint32_t(size.X), C.uint32_t(size.Y),
		C.uint16_t(b.added), C.uint16_t(b.count)) == 0 {
		return l10n.New("error.seriesEncode")
	}
	b.added++
	return nil
}

func (b *tiffBook) write(w io.Writer) error {
	if b.added != b.count || C.TIFFFlush(b.native) == 0 {
		return l10n.New("error.seriesEncode")
	}
	C.TIFFClose(b.native)
	b.native = nil
	f, err := os.Open(b.file)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}
