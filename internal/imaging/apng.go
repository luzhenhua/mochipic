package imaging

import (
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/draw"
	"runtime"

	"imageflow/internal/l10n"
	"imageflow/internal/series"

	"github.com/davidbyttow/govips/v2/vips"
)

const pngSignature = "\x89PNG\r\n\x1a\n"

type pngFrame struct {
	rect           image.Rectangle
	delay          int
	dispose, blend byte
	data           [][]byte
}

type pngAnimation struct {
	header [13]byte
	size   image.Point
	loop   int
	prefix [][]byte
	frames []pngFrame
	canvas *image.NRGBA
	next   int
}

// APNG's PNG poster is optional and may not be a real frame. Parse its
// sequence explicitly; Image I/O can blend the poster or later subframes
// into the wrong frame when the default image is outside the animation.
func parsePNGAnimation(ctx context.Context, data []byte, declared int) (*pngAnimation, error) {
	bad := l10n.New("error.animationDecode")
	if len(data) < 33 || string(data[:8]) != pngSignature || declared < 1 || declared > series.MaxParts {
		return nil, bad
	}
	a := &pngAnimation{frames: make([]pngFrame, 0, declared)}
	seenIDAT, seenACTL, defaultFrame := false, false, false
	var seq uint32
	for at, chunks := 8, 0; at <= len(data)-12 && chunks < 1<<18; chunks++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n := int(binary.BigEndian.Uint32(data[at:]))
		if n > len(data)-at-12 {
			return nil, bad
		}
		end := at + n + 12
		p := data[at+8 : end-4]
		tag := string(data[at+4 : at+8])
		if crc32.ChecksumIEEE(data[at+4:end-4]) != binary.BigEndian.Uint32(data[end-4:end]) || at == 8 && tag != "IHDR" {
			return nil, bad
		}
		switch tag {
		case "IHDR":
			if at != 8 || n != 13 {
				return nil, bad
			}
			a.size = image.Pt(int(binary.BigEndian.Uint32(p)), int(binary.BigEndian.Uint32(p[4:])))
			if !validSize(a.size.X, a.size.Y, MaxInputPixels) {
				return nil, imageBounds(MaxInputPixels)
			}
			copy(a.header[:], p)
		case "acTL":
			if n != 8 || seenACTL || seenIDAT || int(binary.BigEndian.Uint32(p)) != declared {
				return nil, bad
			}
			seenACTL = true
			a.loop = int(binary.BigEndian.Uint32(p[4:]))
		case "fcTL":
			count := len(a.frames)
			if n != 26 || !seenACTL || count >= declared || count != 0 && len(a.frames[count-1].data) == 0 || binary.BigEndian.Uint32(p) != seq {
				return nil, bad
			}
			seq++
			w, h := int(binary.BigEndian.Uint32(p[4:])), int(binary.BigEndian.Uint32(p[8:]))
			x, y := int(binary.BigEndian.Uint32(p[12:])), int(binary.BigEndian.Uint32(p[16:]))
			if w < 1 || h < 1 || x > a.size.X-w || y > a.size.Y-h || p[24] > 2 || p[25] > 1 {
				return nil, bad
			}
			if !seenIDAT {
				defaultFrame = true
				if x != 0 || y != 0 || w != a.size.X || h != a.size.Y {
					return nil, bad
				}
			}
			num, den := int(binary.BigEndian.Uint16(p[20:])), int(binary.BigEndian.Uint16(p[22:]))
			if den == 0 {
				den = 100
			}
			a.frames = append(a.frames, pngFrame{rect: image.Rect(x, y, x+w, y+h), delay: (num*1000 + den/2) / den, dispose: p[24], blend: p[25]})
		case "IDAT":
			if !seenACTL || len(a.frames) > 1 || len(a.frames) == 1 && !defaultFrame {
				return nil, bad
			}
			seenIDAT = true
			if defaultFrame {
				a.frames[0].data = append(a.frames[0].data, p)
			}
		case "fdAT":
			count := len(a.frames)
			if n < 4 || !seenIDAT || count == 0 || defaultFrame && count == 1 || binary.BigEndian.Uint32(p) != seq {
				return nil, bad
			}
			seq++
			a.frames[count-1].data = append(a.frames[count-1].data, p[4:])
		case "IEND":
			if n != 0 || !seenIDAT || len(a.frames) != declared || len(a.frames[declared-1].data) == 0 {
				return nil, bad
			}
			return a, nil
		case "PLTE", "tRNS", "gAMA", "cHRM", "cICP", "sRGB", "iCCP", "sBIT", "eXIf":
			a.prefix = append(a.prefix, data[at:end])
		default:
			if tag[0] >= 'A' && tag[0] <= 'Z' {
				return nil, bad // Unknown critical PNG chunk.
			}
		}
		at = end
	}
	return nil, bad
}

func (a *pngAnimation) metadata(part int) frameMeta {
	return frameMeta{series: series.Info{Kind: series.Frames, Count: len(a.frames)}, delay: a.frames[part-1].delay, loop: a.loop}
}

func pngChunk(dst []byte, tag string, data []byte) []byte {
	dst = binary.BigEndian.AppendUint32(dst, uint32(len(data)))
	start := len(dst)
	dst = append(dst, tag...)
	dst = append(dst, data...)
	return binary.BigEndian.AppendUint32(dst, crc32.ChecksumIEEE(dst[start:]))
}

func (a *pngAnimation) framePixels(f pngFrame) (image.Image, int, error) {
	hdr := a.header
	binary.BigEndian.PutUint32(hdr[:], uint32(f.rect.Dx()))
	binary.BigEndian.PutUint32(hdr[4:], uint32(f.rect.Dy()))
	length := 45
	for _, p := range a.prefix {
		length += len(p)
	}
	for _, p := range f.data {
		length += 12 + len(p)
	}
	data := append(make([]byte, 0, length), pngSignature...)
	data = pngChunk(data, "IHDR", hdr[:])
	for _, p := range a.prefix {
		data = append(data, p...)
	}
	for _, p := range f.data {
		data = pngChunk(data, "IDAT", p)
	}
	data = pngChunk(data, "IEND", nil)
	var pin runtime.Pinner
	pin.Pin(&data[0])
	defer pin.Unpin()
	im, err := vips.NewImageFromBuffer(data)
	if err != nil {
		return nil, 0, err
	}
	defer im.Close()
	if err := srgb(im); err != nil {
		return nil, 0, err
	}
	pixels, err := im.ToGoImage()
	return pixels, im.GetOrientation(), err
}

func (a *pngAnimation) part(ctx context.Context, part, limit int) (*loaded, error) {
	if part < 1 || part > len(a.frames) {
		return nil, l10n.New("error.seriesChanged")
	}
	if !validSize(a.size.X, a.size.Y, limit) {
		return nil, imageBounds(limit)
	}
	if a.canvas == nil {
		a.canvas = image.NewNRGBA(image.Rectangle{Max: a.size})
	} else if a.next >= part {
		clear(a.canvas.Pix)
		a.next = 0
	}
	var result *loaded
	for ; a.next < part; a.next++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		frame := a.frames[a.next]
		wanted := a.next == part-1
		// Disposed frames only affect the next canvas through their disposal.
		// We need their pixels only when they are explicitly requested.
		if !wanted && frame.dispose != 0 {
			if frame.dispose == 1 {
				draw.Draw(a.canvas, frame.rect, image.Transparent, image.Point{}, draw.Src)
			}
			continue
		}
		pixels, orientation, err := a.framePixels(frame)
		if err != nil {
			return nil, l10n.Wrap("error.readImage", err)
		}
		var previous *image.NRGBA
		if frame.dispose == 2 {
			previous = image.NewNRGBA(frame.rect)
			draw.Draw(previous, frame.rect, a.canvas, frame.rect.Min, draw.Src)
		}
		op := draw.Src
		if frame.blend == 1 {
			op = draw.Over
		}
		draw.Draw(a.canvas, frame.rect, pixels, pixels.Bounds().Min, op)
		if wanted {
			info := Info{Format: PNG, Width: a.size.X, Height: a.size.Y, Alpha: !a.canvas.Opaque()}
			im, err := imageFromRGBA(a.canvas.Pix, info)
			if err != nil {
				return nil, err
			}
			if orientation > 1 && orientation <= 8 {
				err = im.SetOrientation(orientation)
				if err == nil {
					err = im.AutoRotate()
				}
				if err != nil {
					im.Close()
					return nil, err
				}
				info.Width, info.Height = im.Width(), im.Height()
			}
			result = &loaded{ImageRef: im, info: info, frame: a.metadata(part)}
		}
		switch frame.dispose {
		case 1:
			draw.Draw(a.canvas, frame.rect, image.Transparent, image.Point{}, draw.Src)
		case 2:
			draw.Draw(a.canvas, frame.rect, previous, frame.rect.Min, draw.Src)
		}
	}
	return result, nil
}
