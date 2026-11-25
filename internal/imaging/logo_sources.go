package imaging

import (
	"context"
	"io"
	"path/filepath"

	"imageflow/internal/l10n"
	"imageflow/internal/series"

	"github.com/davidbyttow/govips/v2/vips"
)

type logoSource struct {
	image *vips.ImageRef
	err   error
}

type logoSources map[string]logoSource

func prepareLogoSources(ctx context.Context, marks []LogoMark) logoSources {
	sources := make(logoSources, len(marks))
	for _, logo := range marks {
		if _, ok := sources[logo.Path]; logo.Opacity == 0 || ok {
			continue
		}
		im, err := readLogoSource(ctx, logo.Path)
		if err != nil {
			err = l10n.WrapWithArgs("error.logoLayerFile", err, l10n.Args{"file": filepath.Base(logo.Path)})
		}
		sources[logo.Path] = logoSource{im, err}
	}
	return sources
}

func readLogoSource(ctx context.Context, path string) (*vips.ImageRef, error) {
	src, err := openLogo(ctx, path)
	if err != nil {
		return nil, err
	}
	defer src.close()
	if !src.HasAlpha() {
		return nil, l10n.New("error.logoAlphaRequired")
	}
	if err := srgb(src.ImageRef); err != nil {
		return nil, err
	}
	if src.BandFormat() != vips.BandFormatUchar {
		if err := src.Cast(vips.BandFormatUchar); err != nil {
			return nil, err
		}
	}
	pixels, err := src.ToBytes()
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Materialize once. Workers only read this immutable native image; their
	// individual copy graphs live until each output is encoded.
	return vips.NewImageFromMemory(pixels, src.Width(), src.Height(), src.Bands(), vips.BandFormatUchar, vips.InterpretationSRGB)
}

func (s logoSources) close() {
	for _, source := range s {
		if source.image != nil {
			source.image.Close()
		}
	}
}

// BatchEncoder owns decoded artwork for one run. Close it only after all
// callers have finished. Per-source geometry and format stay in Options.
type BatchEncoder struct {
	logos logoSources
}

func NewBatchEncoder(ctx context.Context, opts Options) *BatchEncoder {
	return &BatchEncoder{logos: prepareLogoSources(ctx, opts.LogoMarks())}
}

func (b *BatchEncoder) Close() { b.logos.close() }

func (b *BatchEncoder) Export(ctx context.Context, path string, opts Options, w io.Writer) (Info, error) {
	return b.ExportPart(ctx, path, opts, 1, series.Info{}, w)
}

func (b *BatchEncoder) ExportPart(ctx context.Context, path string, opts Options, index int, expected series.Info, w io.Writer) (Info, error) {
	return exportPart(ctx, path, opts, index, expected, b.logos, w)
}

func (b *BatchEncoder) ExportSeries(ctx context.Context, path string, opts Options, expected series.Info, parts []int, w io.Writer) error {
	return exportSeries(ctx, path, opts, expected, parts, b.logos, w)
}
