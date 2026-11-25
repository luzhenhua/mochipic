package imaging

import (
	"fmt"
	"image"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/davidbyttow/govips/v2/vips"
)

type glyphKind uint8

const (
	inkGlyph glyphKind = iota
	strokeGlyph
	shadowGlyph
)

type textPass struct {
	kind    glyphKind
	offset  image.Point
	opacity int
}

// Keep the per-pass opacity and the composition order identical in the live
// view and export, including the overlapping stroke and shadow passes.
func textPasses(t TextMark) []textPass {
	passes := make([]textPass, 0, 6)
	if t.Shadow {
		passes = append(passes, textPass{shadowGlyph, image.Pt(2, 2), t.Opacity / 2})
	}
	if t.Stroke {
		for _, offset := range [...]image.Point{{Y: -1}, {X: -1}, {X: 1}, {Y: 1}} {
			passes = append(passes, textPass{strokeGlyph, offset, t.Opacity})
		}
	}
	return append(passes, textPass{kind: inkGlyph, opacity: t.Opacity})
}

type textArtwork struct {
	sprites [3]*vips.ImageRef
	size    image.Point
}

func (a *textArtwork) close() {
	for _, sprite := range a.sprites {
		if sprite != nil {
			sprite.Close()
		}
	}
}

// Text shape depends on the final output dimensions but never on placement
// or opacity. This lets the live view reuse the sprites during a slider drag.
func renderTextArtwork(canvas image.Point, t TextMark) (*textArtwork, error) {
	render := func(color string) (*vips.ImageRef, error) {
		repl := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
		font := InstalledWatermarkFont(t.Font)
		if font == "" {
			font = "Sans"
		}
		text := "<span foreground=\"" + color + "\">" + repl.Replace(t.Text) + "</span>"
		glyphs := max(1, utf8.RuneCountInString(t.Text))
		limit := max(1, int(math.Sqrt(float64(MaxLogoPixels)/(2*float64(glyphs)))))
		fontSize := min(limit, canvas.Y, max(1, t.Size*t.Scale/100))
		mark, err := vips.Text(&vips.TextParams{Text: text, Font: fmt.Sprintf("%s %d", font, fontSize), DPI: 72, RGBA: true, Wrap: vips.TextWrapNone})
		if err != nil {
			return nil, err
		}
		scale := min(1, float64(canvas.X)/float64(mark.Width()), float64(canvas.Y)/float64(mark.Height()))
		if scale < 1 {
			if err := resizeTo(mark, max(1, int(math.Floor(float64(mark.Width())*scale))), max(1, int(math.Floor(float64(mark.Height())*scale)))); err != nil {
				mark.Close()
				return nil, err
			}
		}
		return mark, nil
	}
	art := &textArtwork{}
	ink, err := render(t.Color)
	if err != nil {
		return nil, err
	}
	art.sprites[inkGlyph] = ink
	art.size = image.Pt(ink.Width(), ink.Height())
	if t.Stroke {
		strokeColor := "#FFFFFF"
		r, g, b, _ := ParseColor(t.Color)
		if int(r)*299+int(g)*587+int(b)*114 > 150000 {
			strokeColor = "#000000"
		}
		art.sprites[strokeGlyph], err = render(strokeColor)
		if err != nil {
			art.close()
			return nil, err
		}
	}
	if t.Shadow {
		art.sprites[shadowGlyph], err = render("#000000")
		if err != nil {
			art.close()
			return nil, err
		}
	}
	return art, nil
}

func overlayText(dst *vips.ImageRef, t TextMark) error {
	if !t.Enabled || t.Opacity == 0 {
		return nil
	}
	canvas := image.Pt(dst.Width(), dst.Height())
	art, err := renderTextArtwork(canvas, t)
	if err != nil {
		return err
	}
	defer art.close()
	for kind, sprite := range art.sprites {
		if sprite != nil {
			value := t.Opacity
			if glyphKind(kind) == shadowGlyph {
				value /= 2
			}
			if err := opacity(sprite, value); err != nil {
				return err
			}
		}
	}
	origin := t.Placement.Origin(canvas, art.size)
	for _, pass := range textPasses(t) {
		pos := origin.Add(pass.offset)
		if err := dst.Composite(art.sprites[pass.kind], vips.BlendModeOver, pos.X, pos.Y); err != nil {
			return err
		}
	}
	return nil
}

func logoSize(canvas, original image.Point, l LogoMark) image.Point {
	w := max(1, (canvas.X*l.Width+50)/100)
	h := max(1, (canvas.Y*l.Height+50)/100)
	if l.KeepRatio {
		ratio := float64(original.Y) / float64(original.X)
		h = max(1, int(math.Round(float64(w)*ratio)))
		if h > canvas.Y {
			h = canvas.Y
			w = max(1, int(math.Round(float64(h)/ratio)))
		}
	}
	return image.Pt(w, h)
}

func overlayLogo(dst, mark *vips.ImageRef, l LogoMark) error {
	canvas := image.Pt(dst.Width(), dst.Height())
	size := logoSize(canvas, image.Pt(mark.Width(), mark.Height()), l)
	if err := resizeTo(mark, size.X, size.Y); err != nil {
		return err
	}
	if err := opacity(mark, l.Opacity); err != nil {
		return err
	}
	pos := l.Placement.Origin(canvas, size)
	return dst.Composite(mark, vips.BlendModeOver, pos.X, pos.Y)
}
