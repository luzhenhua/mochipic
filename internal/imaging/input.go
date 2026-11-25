package imaging

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"os"

	"imageflow/internal/imageformat"
	"imageflow/internal/l10n"
	"imageflow/internal/series"

	"github.com/davidbyttow/govips/v2/vips"
)

func vipsFormat(f Format) vips.ImageType {
	switch f {
	case JPEG:
		return vips.ImageTypeJPEG
	case PNG:
		return vips.ImageTypePNG
	case WebP:
		return vips.ImageTypeWEBP
	case AVIF:
		return vips.ImageTypeAVIF
	case HEIF:
		return vips.ImageTypeHEIF
	case TIFF:
		return vips.ImageTypeTIFF
	case GIF:
		return vips.ImageTypeGIF
	case JXL:
		return vips.ImageTypeJXL
	case JP2:
		return vips.ImageTypeJP2K
	case SVG:
		return vips.ImageTypeSVG
	default:
		return vips.ImageTypeUnknown
	}
}

func imageBounds(limit int) error {
	return l10n.Error("error.imageBounds", l10n.Args{"tenThousands": limit / 10_000, "side": MaxSide})
}

func checkInputFile(f *os.File, maxBytes int) (*os.File, int64, error) {
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	if !st.Mode().IsRegular() || st.Size() <= 0 || st.Size() > int64(maxBytes) {
		f.Close()
		return nil, 0, l10n.Error("error.inputFileLimit", l10n.Args{"megabytes": maxBytes >> 20})
	}
	return f, st.Size(), nil
}

func readInput(r io.Reader, maxBytes int) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, int64(maxBytes)+1))
	if err == nil && (len(data) == 0 || len(data) > maxBytes) {
		err = l10n.New("error.inputChanged")
	}
	return data, err
}

func isJ2K(r io.ReaderAt) bool {
	var head [4]byte
	_, err := r.ReadAt(head[:], 0)
	// SOC followed by SIZ, the raw JPEG 2000 codestream header.
	return err == nil && head == [4]byte{0xff, 0x4f, 0xff, 0x51}
}

func usesSystemRaster(r io.ReaderAt, f Format) bool {
	return f == BMP || f == ICO || f == PSD || f == RAW || f == JP2 && isJ2K(r)
}

func inputLimit(format Format, maxBytes int) int {
	if format == SVG {
		return min(maxBytes, MaxSVGBytes)
	}
	return maxBytes
}

type containerInfo struct {
	nativeAnimation bool
	apngFrames      int
}

// inspectContainer matches the filename hint against the actual container.
// APNG and HEIF/AVIF sequences need an explicit animation loader; their
// still-image loaders can return a poster instead of the real frames.
func inspectContainer(r io.ReaderAt, size int64, expected Format) (info containerInfo, err error) {
	bad := l10n.New("error.corruptOrWrongExtension")
	if expected == "" {
		return info, l10n.New("error.unsupportedInput")
	}
	if expected == SVG {
		var head [2]byte
		// Only plain SVG. The restricted renderer verifies the document type.
		if _, err := r.ReadAt(head[:], 0); err != nil || head == [2]byte{0x1f, 0x8b} {
			return info, bad
		}
		if size > MaxSVGBytes {
			return info, l10n.Error("error.inputFileLimit", l10n.Args{"megabytes": MaxSVGBytes >> 20})
		}
		return info, nil
	}
	if expected == PSD {
		return info, inspectPSD(r, size)
	}
	var head [12]byte
	if _, err := r.ReadAt(head[:], 0); err != nil {
		return info, bad
	}
	switch expected {
	case RAW:
		// TIFF-based, Fuji, Olympus, Panasonic and Canon BMFF RAWs have
		// different signatures; the system loader verifies their RAW UTI.
	case JP2:
		if !isJ2K(r) && vips.DetermineImageType(head[:]) != vips.ImageTypeJP2K {
			return info, bad
		}
	case ICO:
		if !bytes.HasPrefix(head[:], []byte{0, 0, 1, 0}) {
			return info, bad
		}
	case HEIF, AVIF:
		end := int64(binary.BigEndian.Uint32(head[:4]))
		if string(head[4:8]) != "ftyp" || end < 16 || end > min(2048, size) || end%4 != 0 {
			return info, bad
		}
		var brands [2048 - 12]byte
		if _, err := r.ReadAt(brands[:end-12], 12); err != nil {
			return info, bad
		}
		var heif, avif, sequence bool
		check := func(brand string) {
			switch brand {
			case "avis":
				sequence, avif = true, true
			case "msf1", "hevc", "hevx", "hevm", "hevs":
				sequence, heif = true, true
			case "avif":
				avif = true
			case "heic", "heix", "heim", "heis", "mif1":
				heif = true
			}
		}
		check(string(head[8:]))
		for i := int64(4); i < end-12; i += 4 { // Skip the minor version.
			check(string(brands[i : i+4]))
		}
		if expected == AVIF && !avif || expected == HEIF && (!heif || avif) {
			return info, bad
		}
		info.nativeAnimation = sequence
	default:
		want := vipsFormat(expected)
		if expected == BMP {
			want = vips.ImageTypeBMP
		}
		if vips.DetermineImageType(head[:]) != want || expected == WebP && string(head[:4]) != "RIFF" {
			return info, bad
		}
		if expected == PNG {
			var chunk [8]byte
			for offset, chunks := int64(8), 0; offset+12 <= size && chunks < 4096; chunks++ {
				if _, err := r.ReadAt(chunk[:], offset); err != nil {
					return info, bad
				}
				length := int64(binary.BigEndian.Uint32(chunk[:4]))
				next := offset + 12 + length
				if next > size {
					return info, bad
				}
				switch string(chunk[4:]) {
				case "acTL":
					var data [8]byte
					if length != 8 || info.apngFrames != 0 {
						return info, bad
					}
					if _, err := r.ReadAt(data[:], offset+8); err != nil {
						return info, bad
					}
					info.apngFrames = int(binary.BigEndian.Uint32(data[:4]))
					if info.apngFrames == 0 {
						return info, bad
					}
					if info.apngFrames > series.MaxParts {
						return info, l10n.Error("error.seriesLimit", l10n.Args{"count": series.MaxParts})
					}
				case "IDAT":
					return info, nil
				case "IEND":
					return info, bad
				}
				offset = next
			}
			return info, bad
		}
	}
	return info, nil
}

func checkImageBounds(im *vips.ImageRef, limit int) error {
	if !validSize(im.Width(), im.Height(), limit) {
		return imageBounds(limit)
	}
	return nil
}

func InspectInput(ctx context.Context, path string, format Format) error {
	_, err := InspectSource(ctx, path, format)
	return err
}

// InspectSource validates headers and dimensions without decoding all pixels.
func InspectSource(ctx context.Context, path string, format Format) (series.Info, error) {
	var single series.Info
	if err := ctx.Err(); err != nil {
		return single, err
	}
	maxBytes := inputLimit(format, MaxInputBytes)
	f, size, err := openInput(path, maxBytes)
	if err != nil {
		return single, err
	}
	defer f.Close()
	container, err := inspectContainer(f, size, format)
	if err != nil {
		return single, err
	}
	if container.apngFrames != 0 || container.nativeAnimation || format == SVG || usesSystemRaster(f, format) {
		if size > 24<<20 || format == RAW || format == SVG || container.nativeAnimation || container.apngFrames != 0 {
			release, err := acquire(ctx, largeInput)
			if err != nil {
				return single, err
			}
			defer release()
		}
		data, err := readInput(f, maxBytes)
		if err != nil {
			return single, err
		}
		container, err = inspectContainer(bytes.NewReader(data), int64(len(data)), format)
		if err != nil {
			return single, err
		}
		if container.apngFrames != 0 {
			a, err := parsePNGAnimation(ctx, data, container.apngFrames)
			if err != nil {
				return single, err
			}
			return a.metadata(1).series, nil
		}
		if container.nativeAnimation {
			meta, _, _, err := systemAnimation(data, format, 1, MaxInputPixels, false)
			return meta.series, err
		}
		if format == SVG {
			_, _, err = svgRaster(data, MaxInputPixels, 1, false)
			return single, err
		}
		_, _, err = systemRaster(data, format, MaxInputPixels, false, imageformat.RawTypeHint(path))
		return single, err
	}
	p := vips.NewImportParams()
	p.Access.Set(vips.AccessSequential)
	im, err := vips.LoadImageFromReader(io.NewSectionReader(f, 0, size), p)
	if err != nil {
		return single, l10n.Wrap("error.readImage", err)
	}
	defer im.Close()
	if err := checkImageBounds(im, MaxInputPixels); err != nil {
		return single, err
	}
	meta, err := vipsSeries(im, format, 1)
	return meta.series, err
}

var largeInput = make(chan struct{}, 1)

func acquire(ctx context.Context, slots chan struct{}) (func(), error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case slots <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-slots
			return nil, err
		}
		return func() { <-slots }, nil
	}
}
