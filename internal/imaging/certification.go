package imaging

import (
	"context"
	"image"
	"math"

	"imageflow/internal/certification"
	"imageflow/internal/l10n"

	"github.com/davidbyttow/govips/v2/vips"
)

const (
	certificationPreviewEdge = 360
	certificationImportEdge  = 1600
)

type CertificationPreview struct {
	Pixels image.Image
	Size   image.Point
}

func openBadge(ctx context.Context, b *certification.Badge) (*loaded, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if path := b.LocalPath(); path != "" {
		return openLogo(ctx, path)
	}
	data, err := b.EmbeddedPNG()
	if err != nil {
		return nil, err
	}
	return loadBuffer(data, PNG, MaxLogoPixels)
}

func RenderCertificationBadge(ctx context.Context, b *certification.Badge) (CertificationPreview, error) {
	src, err := openBadge(ctx, b)
	if err != nil {
		return CertificationPreview{}, err
	}
	defer src.close()
	if err := src.normalize(); err != nil {
		return CertificationPreview{}, err
	}
	return certificationPreview(src.ImageRef)
}

func certificationPreview(src *vips.ImageRef) (CertificationPreview, error) {
	size := image.Pt(src.Width(), src.Height())
	pixels, err := displayPixels(src, certificationPreviewEdge)
	return CertificationPreview{Pixels: pixels, Size: size}, err
}

// PrepareCertificationImport validates a still PNG and returns an sRGB copy
// for the private library. The original file remains read-only.
func PrepareCertificationImport(ctx context.Context, path string) ([]byte, CertificationPreview, error) {
	if FormatOf(path) != PNG {
		return nil, CertificationPreview{}, l10n.New("error.certificationPNG")
	}
	src, err := openLogo(ctx, path)
	if err != nil {
		return nil, CertificationPreview{}, err
	}
	defer src.close()
	if err := src.normalize(); err != nil {
		return nil, CertificationPreview{}, err
	}
	if w, h := src.Width(), src.Height(); max(w, h) > certificationImportEdge {
		scale := float64(certificationImportEdge) / float64(max(w, h))
		if err := resizeTo(src.ImageRef, max(1, int(math.Round(float64(w)*scale))), max(1, int(math.Round(float64(h)*scale)))); err != nil {
			return nil, CertificationPreview{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, CertificationPreview{}, err
	}
	if src.BandFormat() != vips.BandFormatUchar {
		if err := src.Cast(vips.BandFormatUchar); err != nil {
			return nil, CertificationPreview{}, err
		}
	}
	data, _, err := src.ExportPng(&vips.PngExportParams{Compression: 6, StripMetadata: true})
	if err != nil {
		return nil, CertificationPreview{}, err
	}
	preview, err := certificationPreview(src.ImageRef)
	return data, preview, err
}

func CertificationLayers(canvas image.Point, g certification.Group, previews map[*certification.Badge]CertificationPreview) ([]PreviewLayer, image.Rectangle, error) {
	n := g.Count()
	if n == 0 || canvas.X <= 0 || canvas.Y <= 0 {
		return nil, image.Rectangle{}, nil
	}
	var sizes [certification.MaxBadges]image.Point
	for i := range n {
		sizes[i] = previews[g.Badge(i)].Size
	}
	layout, ok := g.Arrange(canvas, sizes)
	if !ok {
		return nil, image.Rectangle{}, l10n.New("error.certificationFit")
	}
	layers := make([]PreviewLayer, n)
	for i := range n {
		layers[i] = PreviewLayer{Pixels: previews[g.Badge(i)].Pixels, Bounds: layout.Badges[i], Opacity: 100}
	}
	return layers, layout.Bounds, nil
}

// The original PNG data remains pinned until the final lazy composite is
// encoded. No native badge reference is shared between workers.
type certificationArtwork [certification.MaxBadges]*loaded

func (a *certificationArtwork) close() {
	for _, src := range a {
		if src != nil {
			src.close()
		}
	}
}

func (a *certificationArtwork) composite(ctx context.Context, dst *vips.ImageRef, g certification.Group) error {
	n := g.Count()
	if n == 0 {
		return nil
	}
	var sizes [certification.MaxBadges]image.Point
	for i := range n {
		src, err := openBadge(ctx, g.Badge(i))
		if err != nil {
			return err
		}
		a[i] = src
		if err := src.normalize(); err != nil {
			return err
		}
		sizes[i] = image.Pt(src.Width(), src.Height())
	}
	layout, ok := g.Arrange(image.Pt(dst.Width(), dst.Height()), sizes)
	if !ok {
		return l10n.New("error.certificationFit")
	}
	var overlays [certification.MaxBadges]*vips.ImageComposite
	for i := range n {
		if err := ctx.Err(); err != nil {
			return err
		}
		r, im := layout.Badges[i], a[i].ImageRef
		if err := resizeTo(im, r.Dx(), r.Dy()); err != nil {
			return err
		}
		overlays[i] = &vips.ImageComposite{Image: im, BlendMode: vips.BlendModeOver, X: r.Min.X, Y: r.Min.Y}
	}
	return dst.CompositeMulti(overlays[:n])
}
