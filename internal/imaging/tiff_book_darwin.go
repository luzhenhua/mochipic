package imaging

/*
#include "raster_darwin.h"
*/
import "C"

import (
	"io"
	"unsafe"

	"imageflow/internal/l10n"
)

type tiffBook struct{ native *C.TIFFBook }

func newTIFFBook(count int) (tiffBook, error) {
	book := tiffBook{C.tiff_book_new(C.size_t(count))}
	if book.native == nil {
		return book, l10n.New("error.seriesEncode")
	}
	return book, nil
}

func (b tiffBook) close() { C.tiff_book_free(b.native) }

func (b tiffBook) add(png []byte) error {
	if len(png) == 0 || !bool(C.tiff_book_add(b.native, (*C.uchar)(unsafe.Pointer(&png[0])), C.size_t(len(png)))) {
		return l10n.New("error.seriesEncode")
	}
	return nil
}

func (b tiffBook) write(w io.Writer) error {
	var size C.size_t
	data := C.tiff_book_finish(b.native, &size)
	if data == nil || size == 0 {
		return l10n.New("error.seriesEncode")
	}
	n, err := w.Write(unsafe.Slice((*byte)(unsafe.Pointer(data)), int(size)))
	if err == nil && n != int(size) {
		err = io.ErrShortWrite
	}
	return err
}
