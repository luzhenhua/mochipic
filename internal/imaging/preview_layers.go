package imaging

import (
	"context"
	"image"
	"image/color"

	"imageflow/internal/crop"
	"imageflow/internal/l10n"
)

// PreviewBase holds only bounded Go bitmaps; native images are released
// before the worker publishes a result. Size is in final output pixels.
type PreviewBase struct {
	Original, Canvas image.Image
	Input            Info
	Size             image.Point
}

func RenderPreviewBase(ctx context.Context, path string, r Resize) (PreviewBase, error) {
	return RenderPreviewBasePart(ctx, path, r, 1)
}

func RenderPreviewBasePart(ctx context.Context, path string, r Resize, index int) (PreviewBase, error) {
	return RenderCroppedPreviewBase(ctx, path, r, crop.Region{}, index)
}

func RenderCroppedPreviewBase(ctx context.Context, path string, r Resize, region crop.Region, index int) (PreviewBase, error) {
	var out PreviewBase
	src, err := openImagePart(ctx, path, MaxInputBytes, MaxInputPixels, index)
	if err != nil {
		return out, &StageError{Decode, err}
	}
	defer src.close()
	if err = src.normalize(); err != nil {
		return out, &StageError{Transform, err}
	}
	out.Input = src.info
	out.Original, err = src.originalPixels(PreviewEdge)
	if err != nil {
		return out, &StageError{Decode, err}
	}
	if !r.Enabled && !region.Active() {
		out.Canvas = out.Original
		out.Size = image.Pt(src.info.Width, src.info.Height)
		return out, nil
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if err := r.validate(); err != nil {
		return out, &StageError{Transform, err}
	}
	canvas, err := src.canvas(r, region)
	if err != nil {
		return out, &StageError{Transform, err}
	}
	defer canvas.Close()
	out.Size = image.Pt(canvas.Width(), canvas.Height())
	out.Canvas, err = displayPixels(canvas, PreviewEdge)
	if err != nil {
		return out, &StageError{Transform, err}
	}
	return out, nil
}

// PreviewLayer is an independently drawn bitmap. Bounds are in output pixels;
// its pixels are capped to display resolution, just like the base.
type PreviewLayer struct {
	Pixels   image.Image // Nil for a constant-color fill.
	Fill     color.NRGBA
	Bounds   image.Rectangle
	Opacity  int
	Mark     WatermarkKind // Only the main glyph or logo is a drag target.
	LogoPath string        // Stable key for the independently editable Logo.
}

type WatermarkKind uint8

const (
	TextWatermark WatermarkKind = iota + 1
	LogoWatermark
)

type TextPreview struct {
	sprites [3]image.Image
	size    image.Point
	Canvas  image.Point
}

func RenderPreviewText(ctx context.Context, canvas image.Point, t TextMark) (TextPreview, error) {
	var out TextPreview
	if !t.Enabled || t.Missing() || canvas.X <= 0 || canvas.Y <= 0 {
		return out, nil
	}
	if err := t.validate(); err != nil {
		return out, err
	}
	art, err := renderTextArtwork(canvas, t)
	if err != nil {
		return out, err
	}
	defer art.close()
	out.size, out.Canvas = art.size, canvas
	scale := float64(PreviewEdge) / float64(max(canvas.X, canvas.Y))
	for i, sprite := range art.sprites {
		if sprite == nil {
			continue
		}
		if err := ctx.Err(); err != nil {
			return TextPreview{}, err
		}
		out.sprites[i], err = displayScaled(sprite, scale)
		if err != nil {
			return TextPreview{}, err
		}
	}
	return out, nil
}

func (s TextPreview) appendLayers(layers []PreviewLayer, canvas image.Point, t TextMark) []PreviewLayer {
	if !t.Enabled || t.Missing() || t.Opacity <= 0 || !t.Placement.Valid(MaxSide) || s.Canvas != canvas || s.sprites[inkGlyph] == nil {
		return layers
	}
	pos := t.Placement.Origin(canvas, s.size)
	for _, pass := range textPasses(t) {
		if pixels := s.sprites[pass.kind]; pixels != nil && pass.opacity > 0 {
			start := pos.Add(pass.offset)
			layer := PreviewLayer{Pixels: pixels, Bounds: image.Rectangle{Min: start, Max: start.Add(s.size)}, Opacity: pass.opacity}
			if pass.kind == inkGlyph {
				layer.Mark = TextWatermark
			}
			layers = append(layers, layer)
		}
	}
	return layers
}

type LogoPreview struct {
	pixels image.Image
	size   image.Point
}

func (s LogoPreview) Pixels() image.Image { return s.pixels }

// RenderPreviewLogo also validates a newly selected PNG. Opacity and size
// are applied at draw time, so they never need to reopen this file.
func RenderPreviewLogo(ctx context.Context, path string) (LogoPreview, error) {
	var out LogoPreview
	if FormatOf(path) != PNG {
		return out, l10n.New("error.logoMustBePNG")
	}
	src, err := openLogo(ctx, path)
	if err != nil {
		return out, err
	}
	defer src.close()
	if !src.HasAlpha() {
		return out, l10n.New("error.selectedPNGWithoutAlpha")
	}
	if err := srgb(src.ImageRef); err != nil {
		return out, err
	}
	out.size = image.Pt(src.Width(), src.Height())
	out.pixels, err = displayPixels(src.ImageRef, PreviewEdge)
	return out, err
}

func (s LogoPreview) appendLayer(layers []PreviewLayer, canvas image.Point, l LogoMark) []PreviewLayer {
	if !l.Enabled || l.Missing() || !l.Placement.Valid(MaxSide) || s.pixels == nil ||
		l.Width < 1 || l.Width > 100 || !l.KeepRatio && (l.Height < 1 || l.Height > 100) {
		return layers
	}
	size := logoSize(canvas, s.size, l)
	pos := l.Placement.Origin(canvas, size)
	return append(layers, PreviewLayer{Pixels: s.pixels, Bounds: image.Rectangle{Min: pos, Max: pos.Add(size)}, Opacity: l.Opacity, Mark: LogoWatermark, LogoPath: l.Path})
}

func PreviewLayers(canvas image.Point, text TextPreview, logos map[string]LogoPreview, opts Options) []PreviewLayer {
	if canvas.X <= 0 || canvas.Y <= 0 {
		return nil
	}
	var layers []PreviewLayer
	marks := opts.LogoMarks()
	position := opts.TextLayerPosition(len(marks))
	for _, mark := range marks[:position] {
		layers = logos[mark.Path].appendLayer(layers, canvas, mark)
	}
	layers = text.appendLayers(layers, canvas, opts.Text)
	for _, mark := range marks[position:] {
		layers = logos[mark.Path].appendLayer(layers, canvas, mark)
	}
	return layers
}
