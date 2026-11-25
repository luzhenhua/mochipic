package imaging

/*
#cgo pkg-config: libwebpmux libwebp
#include <webp/mux.h>

static WebPMux* new_animation(int loop) {
	WebPMux *mux = WebPMuxNew();
	WebPMuxAnimParams params = {0, loop};
	if (mux && WebPMuxSetAnimationParams(mux, &params) != WEBP_MUX_OK) {
		WebPMuxDelete(mux);
		return NULL;
	}
	return mux;
}

static WebPMuxError append_frame(WebPMux *mux, const uint8_t *bytes, size_t len, int duration) {
	WebPMuxFrameInfo frame = {0};
	frame.bitstream.bytes = bytes;
	frame.bitstream.size = len;
	frame.duration = duration;
	frame.id = WEBP_CHUNK_ANMF;
	frame.blend_method = WEBP_MUX_NO_BLEND;
	return WebPMuxPushFrame(mux, &frame, 1);
}
*/
import "C"

import (
	"io"
	"unsafe"

	"imageflow/internal/l10n"
)

// Mux the already processed, full-canvas WebP stills. This keeps even
// identical consecutive frames and preserves their exact millisecond delays.
type webpAnimation struct{ mux *C.WebPMux }

func newWebPAnimation(loop int) (webpAnimation, error) {
	a := webpAnimation{C.new_animation(C.int(loop))}
	if a.mux == nil {
		return a, l10n.New("error.seriesWebP")
	}
	return a, nil
}

func (a webpAnimation) close() { C.WebPMuxDelete(a.mux) }

func (a webpAnimation) add(frame []byte, duration int) error {
	if len(frame) == 0 || C.append_frame(a.mux, (*C.uint8_t)(unsafe.Pointer(&frame[0])), C.size_t(len(frame)), C.int(duration)) != C.WEBP_MUX_OK {
		return l10n.New("error.seriesWebP")
	}
	return nil
}

func (a webpAnimation) write(w io.Writer) error {
	var data C.WebPData
	if C.WebPMuxAssemble(a.mux, &data) != C.WEBP_MUX_OK {
		return l10n.New("error.seriesWebP")
	}
	defer C.WebPDataClear(&data)
	p := unsafe.Slice((*byte)(unsafe.Pointer(data.bytes)), int(data.size))
	n, err := w.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	return err
}
