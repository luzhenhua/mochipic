package imaging

import (
	"bytes"
	"context"
	"io"

	"imageflow/internal/l10n"
	"imageflow/internal/series"

	"github.com/davidbyttow/govips/v2/vips"
)

// MaxCombinedPixels bounds the working set of an assembled output. Individual
// frames/pages remain limited by the same geometry as ordinary stills.
const MaxCombinedPixels = 64_000_000

func ExportSeries(ctx context.Context, path string, opts Options, expected series.Info, parts []int, w io.Writer) error {
	return exportSeries(ctx, path, opts, expected, parts, nil, w)
}

func exportSeries(ctx context.Context, path string, opts Options, expected series.Info, parts []int, logos logoSources, w io.Writer) error {
	if len(parts) == 0 || len(parts) > expected.Count || !expected.Multiple() || expected.Count > series.MaxParts ||
		expected.Kind != series.Pages && expected.Kind != series.Frames {
		return &StageError{Transform, l10n.New("error.seriesScope")}
	}
	// The per-file assembly choice determines its container. PNG is the
	// lossless bridge to the native multi-page TIFF writer.
	opts.Format = PNG
	if expected.Kind == series.Frames {
		opts.Format = WebP
	}
	if err := opts.Validate(); err != nil {
		return &StageError{Transform, err}
	}
	if logos == nil {
		logos = prepareLogoSources(ctx, opts.LogoMarks())
		defer logos.close()
	}
	input, err := readImageBuffer(ctx, path, MaxInputBytes, true)
	if err != nil {
		return &StageError{Decode, err}
	}
	defer input.close()

	var book tiffBook
	if expected.Kind == series.Pages {
		book, err = newTIFFBook(len(parts))
		if err != nil {
			return &StageError{Encode, err}
		}
		defer book.close()
	}
	var animation webpAnimation
	defer func() {
		if animation.mux != nil {
			animation.close()
		}
	}()
	total, width, height, previous := 0, 0, 0, 0
	for _, index := range parts {
		if err := ctx.Err(); err != nil {
			return err
		}
		if index <= previous || index > expected.Count {
			return &StageError{Decode, l10n.New("error.seriesChanged")}
		}
		previous = index
		err := func() error {
			src, err := input.part(ctx, index, MaxInputPixels)
			if err != nil {
				return &StageError{Decode, err}
			}
			defer src.close()
			if src.frame.series != expected {
				return &StageError{Decode, l10n.New("error.seriesChanged")}
			}
			if err := src.normalize(); err != nil {
				return &StageError{Transform, err}
			}
			return process(ctx, src, opts, logos, func(im *vips.ImageRef, info Info) error {
				total += info.Width * info.Height
				if total > MaxCombinedPixels {
					return &StageError{Transform, l10n.New("error.seriesPixelLimit")}
				}
				if expected.Kind == series.Frames {
					if len(parts) == 1 {
						return encode(ctx, im, opts, w)
					}
					if src.frame.delay < 0 || src.frame.delay > 0xffffff || src.frame.loop < 0 || src.frame.loop > 0xffff {
						return &StageError{Encode, l10n.New("error.seriesTiming")}
					}
					if animation.mux == nil {
						width, height = info.Width, info.Height
						animation, err = newWebPAnimation(src.frame.loop)
						if err != nil {
							return &StageError{Encode, err}
						}
					}
					if info.Width != width || info.Height != height {
						return &StageError{Transform, l10n.New("error.seriesFrameSize")}
					}
					var webp bytes.Buffer
					if err := encode(ctx, im, opts, &webp); err != nil {
						return err
					}
					if err := animation.add(webp.Bytes(), src.frame.delay); err != nil {
						return &StageError{Encode, err}
					}
					return nil
				}
				// Materialize just this output so lazy graphs cannot keep the
				// source frame and its native buffers alive for the whole job.
				png, _, err := im.ExportPng(&vips.PngExportParams{Compression: 1, Keep: vips.ForeignKeepIcc})
				if err != nil {
					return &StageError{Encode, err}
				}
				if err := ctx.Err(); err != nil {
					return err
				}
				if err := book.add(png); err != nil {
					return &StageError{Encode, err}
				}
				return nil
			})
		}()
		if err != nil {
			return err
		}
	}
	if expected.Kind == series.Pages {
		if err := book.write(contextWriter{ctx, w}); err != nil {
			return &StageError{Encode, err}
		}
		return ctx.Err()
	}
	if animation.mux != nil {
		if err := animation.write(contextWriter{ctx, w}); err != nil {
			return &StageError{Encode, err}
		}
	}
	return ctx.Err()
}
