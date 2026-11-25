package imaging

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"unicode/utf8"

	"imageflow/internal/caption"
	"imageflow/internal/l10n"

	"github.com/davidbyttow/govips/v2/vips"
)

type captionLayout struct {
	band, ink image.Rectangle
}

var escapeMarkup = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// Render text at its intended final size. A long title wraps to the available
// width; a short fixed-height strip can reduce the font to keep all lines.
func renderCaption(ctx context.Context, canvas image.Point, t caption.Typography) (*vips.ImageRef, captionLayout, error) {
	var layout captionLayout
	if canvas.X < 1 || canvas.Y < 1 || canvas.X > MaxSide || canvas.Y > MaxSide {
		return nil, layout, l10n.New("error.captionSettings")
	}
	paddingX := min((canvas.X-1)/2, canvas.X*5/100)
	width := canvas.X - paddingX*2
	limit := t.Height
	if limit == 0 {
		limit = caption.MaxHeight
	}
	bandHeight := max(1, (canvas.Y*limit+50)/100)
	paddingY := min((bandHeight-1)/4, (canvas.X*2+50)/100)
	available := bandHeight - paddingY*2
	runes := max(1, utf8.RuneCountInString(t.Text))
	sizeLimit := int(math.Sqrt(float64(MaxLogoPixels) / float64(3*runes)))
	size := max(1, min(available, sizeLimit, (canvas.X*t.Size+50)/100))
	align := vips.AlignCenter
	switch t.Alignment {
	case caption.Left:
		align = vips.AlignLow
	case caption.Right:
		align = vips.AlignHigh
	}
	// The foreground is a validated hex color; all user text is literal.
	markup := `<span foreground="` + t.Color + `">` + escapeMarkup.Replace(t.Text) + `</span>`
	var ink *vips.ImageRef
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, layout, err
		}
		var err error
		ink, err = vips.Text(&vips.TextParams{Text: markup, Font: fmt.Sprintf("Sans Bold %d", size), Width: width,
			Alignment: align, DPI: 72, RGBA: true, Wrap: vips.TextWrapWordChar})
		if err != nil {
			return nil, layout, err
		}
		if ink.Height() <= available || size == 1 || attempt == 2 {
			break
		}
		size = max(1, min(size-1, size*available/ink.Height()))
		ink.Close()
	}
	// Extremely small images and indivisible glyph clusters can exceed the
	// Pango width even at 1 px. Scale down the finished ink to stay in bounds.
	w, h := ink.Width(), ink.Height()
	if scale := min(float64(width)/float64(w), float64(available)/float64(h)); scale < 1 {
		if err := resizeTo(ink, max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))); err != nil {
			ink.Close()
			return nil, layout, err
		}
		w, h = ink.Width(), ink.Height()
	}
	if t.Height == 0 {
		bandHeight = h + paddingY*2
	}
	x, y := (canvas.X-w)/2, canvas.Y-bandHeight+(bandHeight-h)/2
	switch t.Alignment {
	case caption.Left:
		x = paddingX
	case caption.Right:
		x = canvas.X - paddingX - w
	}
	layout.band = image.Rect(0, canvas.Y-bandHeight, canvas.X, canvas.Y)
	layout.ink = image.Rect(x, y, x+w, y+h)
	return ink, layout, nil
}

func captionColor(s caption.Style) color.NRGBA {
	r, g, b, _ := ParseColor(s.Background)
	return color.NRGBA{R: r, G: g, B: b, A: uint8((s.Opacity*255 + 50) / 100)}
}

func overlayCaption(ctx context.Context, dst *vips.ImageRef, c caption.Caption) error {
	if !c.Active() {
		return nil
	}
	ink, layout, err := renderCaption(ctx, image.Pt(dst.Width(), dst.Height()), c.Typography())
	if err != nil {
		return err
	}
	defer ink.Close()
	if bg := captionColor(c.Style); bg.A != 0 {
		strip, err := vips.NewImageFromMemory([]byte{bg.R, bg.G, bg.B, bg.A}, 1, 1, 4, vips.BandFormatUchar, vips.InterpretationSRGB)
		if err != nil {
			return err
		}
		defer strip.Close()
		// A constant-color strip does not allocate an image-sized Go buffer.
		if err := strip.Embed(0, 0, layout.band.Dx(), layout.band.Dy(), vips.ExtendCopy); err != nil {
			return err
		}
		if err := dst.Composite(strip, vips.BlendModeOver, 0, layout.band.Min.Y); err != nil {
			return err
		}
	}
	return dst.Composite(ink, vips.BlendModeOver, layout.ink.Min.X, layout.ink.Min.Y)
}

type CaptionPreview struct {
	canvas image.Point
	layout captionLayout
	ink    image.Image
}

func RenderPreviewCaption(ctx context.Context, canvas image.Point, t caption.Typography) (CaptionPreview, error) {
	ink, layout, err := renderCaption(ctx, canvas, t)
	if err != nil {
		return CaptionPreview{}, err
	}
	defer ink.Close()
	if err := ctx.Err(); err != nil {
		return CaptionPreview{}, err
	}
	pixels, err := displayScaled(ink, float64(PreviewEdge)/float64(max(canvas.X, canvas.Y)))
	return CaptionPreview{canvas, layout, pixels}, err
}

func (p CaptionPreview) Layers(canvas image.Point, c caption.Caption) []PreviewLayer {
	if canvas != p.canvas || p.ink == nil || !c.Active() {
		return nil
	}
	var layers []PreviewLayer
	if bg := captionColor(c.Style); bg.A != 0 {
		layers = append(layers, PreviewLayer{Fill: bg, Bounds: p.layout.band, Opacity: 100})
	}
	return append(layers, PreviewLayer{Pixels: p.ink, Bounds: p.layout.ink, Opacity: 100})
}
