// Package imaging implements the shared libvips pipeline for previews and exports.
package imaging

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"imageflow/internal/caption"
	"imageflow/internal/certification"
	"imageflow/internal/crop"
	"imageflow/internal/imageformat"
	"imageflow/internal/l10n"
	"imageflow/internal/placement"
)

type Format = imageformat.Format

const (
	JPEG = imageformat.JPEG
	PNG  = imageformat.PNG
	WebP = imageformat.WebP
	AVIF = imageformat.AVIF
	HEIF = imageformat.HEIF
	TIFF = imageformat.TIFF
	BMP  = imageformat.BMP
	ICO  = imageformat.ICO
	GIF  = imageformat.GIF
	JXL  = imageformat.JXL
	JP2  = imageformat.JP2
	SVG  = imageformat.SVG
	PSD  = imageformat.PSD
	RAW  = imageformat.RAW
)

func FormatOf(path string) Format { return imageformat.FormatOf(path) }
func InputExtensions() []string   { return imageformat.InputExtensions() }
func OutputFormats() []Format     { return imageformat.OutputFormats() }

const (
	MaxSide        = 12000
	MaxPixels      = 24_000_000 // Output and intermediate resize canvas.
	MaxInputPixels = 50_000_000
	MaxInputBytes  = 160 << 20
	MaxSVGBytes    = 8 << 20
	MaxLogoPixels  = 4_000_000
	MaxLogoLayers  = 8
	PreviewEdge    = 1400
)

type ResizeMode string
type Fit string

const (
	Exact    ResizeMode = "exact"
	Percent  ResizeMode = "percent"
	LongEdge ResizeMode = "long-edge"
	Contain  Fit        = "contain"
	Cover    Fit        = "cover"
	Stretch  Fit        = "stretch"
)

type Resize struct {
	Enabled                       bool
	Mode                          ResizeMode
	Fit                           Fit
	Width, Height, Percent, Limit int
	LockRatio                     bool
	PadColor                      string
	Transparent                   bool
}

type Placement = placement.Placement
type FreePosition = placement.FreePosition

type TextMark struct {
	Enabled           bool
	Text, Font, Color string
	Size, Scale       int
	Placement
	Stroke, Shadow bool
}

type LogoMark struct {
	Enabled       bool
	Path          string
	KeepRatio     bool
	Width, Height int // percent of final canvas width/height
	Placement
}

type Options struct {
	Crop            crop.Region         `json:"-"` // Per source, never stored in a reusable preset.
	Certification   certification.Group `json:"-"`
	Caption         caption.Caption     `json:"-"`
	Resize          Resize
	Text            TextMark
	Logo            LogoMark
	AdditionalLogos [MaxLogoLayers - 1]LogoMark `json:",omitzero"`
	LogoOnTop       bool
	LogosBelowText  int `json:",omitzero"`
	Format          Format
	Quality         int
	Lossless        bool
	FlattenAlpha    bool
	Background      string
}

var (
	ErrTextWatermarkMissing = l10n.New("error.missingTextWatermark")
	ErrLogoWatermarkMissing = l10n.New("error.missingLogoWatermark")
)

func (t TextMark) Missing() bool { return t.Enabled && strings.TrimSpace(t.Text) == "" }
func (l LogoMark) Missing() bool { return l.Enabled && strings.TrimSpace(l.Path) == "" }

func (o Options) MissingWatermark() error {
	if o.Text.Missing() {
		return ErrTextWatermarkMissing
	}
	for _, logo := range o.LogoMarks() {
		if logo.Missing() {
			return ErrLogoWatermarkMissing
		}
	}
	return nil
}

// PreviewOptions omits only unfinished layers; export still validates the
// original options. In particular, one unfinished layer does not hide another.
func (o Options) PreviewOptions() Options {
	if o.Text.Missing() {
		o.Text.Enabled = false
	}
	if o.Logo.Missing() {
		o.Logo.Enabled = false
	}
	for i, logo := range o.AdditionalLogos {
		if logo.Missing() {
			o.AdditionalLogos[i] = LogoMark{}
		}
	}
	return o
}

func Defaults() Options {
	return Options{
		Resize:    Resize{Mode: Exact, Fit: Contain, Width: 1200, Height: 1200, Percent: 50, Limit: 2000, PadColor: "#F1F1F1"},
		Text:      TextMark{Font: "Helvetica Neue Bold", Color: "#222222", Size: 48, Scale: 100, Placement: Placement{Anchor: 8, MarginX: 44, MarginY: 36, Opacity: 68}},
		Logo:      DefaultLogo(),
		LogoOnTop: true, Format: WebP, Quality: 85, Background: "#FFFFFF",
	}
}

func ParseColor(s string) (r, g, b uint8, err error) {
	if len(s) != 7 || s[0] != '#' {
		return 0, 0, 0, l10n.New("error.invalidHex")
	}
	n, e := strconv.ParseUint(s[1:], 16, 24)
	if e != nil {
		return 0, 0, 0, l10n.New("error.invalidHex")
	}
	return uint8(n >> 16), uint8(n >> 8), uint8(n), nil
}

func validSize(w, h, limit int) bool {
	return w > 0 && h > 0 && w <= MaxSide && h <= MaxSide && w <= limit/h
}

func (o Options) UsesQuality() bool {
	return o.Format == JPEG || o.Format == AVIF || o.Format == WebP && !o.Lossless
}

func (o Options) Validate() error {
	if err := o.MissingWatermark(); err != nil {
		return err
	}
	if o.Format.Extension() == "" {
		return l10n.New("error.outputFormatChoices")
	}
	if o.UsesQuality() && (o.Quality < 1 || o.Quality > 100) {
		return l10n.New("error.qualityRange")
	}
	if o.Format == JPEG || o.FlattenAlpha {
		if _, _, _, err := ParseColor(o.Background); err != nil {
			return l10n.Wrap("error.backgroundColor", err)
		}
	}
	if err := o.Resize.validate(); err != nil {
		return err
	}
	if o.Text.Enabled {
		if err := o.Text.validate(); err != nil {
			return err
		}
	}
	marks := o.LogoMarks()
	for i, l := range marks {
		if !l.valid() {
			return l10n.New("error.invalidLogoSettings")
		}
		for _, other := range marks[:i] {
			if l.Path == other.Path {
				return l10n.New("error.invalidLogoSettings")
			}
		}
	}
	if !o.Certification.Valid(MaxSide) {
		return l10n.New("error.certificationSettings")
	}
	if o.Caption.Active() && !o.Caption.Valid() {
		return l10n.New("error.captionSettings")
	}
	return nil
}

func (r Resize) validate() error {
	if r.Enabled {
		switch r.Mode {
		case Exact:
			if !validSize(r.Width, r.Height, MaxPixels) {
				return l10n.New("error.targetDimensionsLimit")
			}
			switch r.Fit {
			case Contain:
				if !r.Transparent {
					if _, _, _, err := ParseColor(r.PadColor); err != nil {
						return l10n.Wrap("error.paddingColor", err)
					}
				}
			case Cover, Stretch:
			default:
				return l10n.New("error.chooseFit")
			}
		case Percent:
			if r.Percent < 1 || r.Percent > 400 {
				return l10n.New("error.scaleRange")
			}
		case LongEdge:
			if r.Limit < 1 || r.Limit > MaxSide {
				return l10n.New("error.longEdgeRange")
			}
		default:
			return l10n.New("error.chooseResizeMode")
		}
	}
	return nil
}

func (t TextMark) validate() error {
	if t.Missing() {
		return ErrTextWatermarkMissing
	}
	if !utf8.ValidString(t.Text) || utf8.RuneCountInString(t.Text) > 120 ||
		strings.ContainsAny(t.Text, "\x00\n\r") || len(t.Font) > 120 || strings.ContainsAny(t.Font, "\x00\n\r") || t.Size < 1 || t.Size > 512 || t.Scale < 1 || t.Scale > 200 || !t.Placement.Valid(MaxSide) {
		return l10n.New("error.invalidTextWatermark")
	}
	if _, _, _, err := ParseColor(t.Color); err != nil {
		return l10n.Wrap("error.textColor", err)
	}
	return nil
}

// Geometry describes the actual final canvas, placed/resized image and its offset.
type Geometry struct {
	Width, Height           int
	ImageWidth, ImageHeight int
	X, Y                    int
}

func (r Resize) Geometry(w, h int) (Geometry, error) {
	g := Geometry{Width: w, Height: h, ImageWidth: w, ImageHeight: h}
	if !validSize(w, h, MaxInputPixels) {
		return g, imageBounds(MaxInputPixels)
	}
	if !r.Enabled {
		if !validSize(w, h, MaxPixels) {
			return g, l10n.New("error.resizeBeforeExport")
		}
		return g, nil
	}
	round := func(x float64) int { return max(1, int(math.Round(x))) }
	switch r.Mode {
	case Exact:
		if !validSize(r.Width, r.Height, MaxPixels) {
			return g, l10n.New("error.targetDimensions")
		}
		g.Width, g.Height = r.Width, r.Height
		x, y := float64(r.Width)/float64(w), float64(r.Height)/float64(h)
		switch r.Fit {
		case Contain:
			s := min(x, y)
			g.ImageWidth, g.ImageHeight = min(r.Width, round(float64(w)*s)), min(r.Height, round(float64(h)*s))
		case Cover:
			s := max(x, y)
			g.ImageWidth, g.ImageHeight = max(r.Width, round(float64(w)*s)), max(r.Height, round(float64(h)*s))
		case Stretch:
			g.ImageWidth, g.ImageHeight = r.Width, r.Height
		default:
			return g, l10n.New("error.invalidFit")
		}
		g.X, g.Y = (g.Width-g.ImageWidth)/2, (g.Height-g.ImageHeight)/2
	case Percent, LongEdge:
		scale := float64(r.Percent) / 100
		if r.Mode == LongEdge {
			if r.Limit < 1 || r.Limit > MaxSide {
				return g, l10n.New("error.invalidLongEdge")
			}
			scale = min(1, float64(r.Limit)/float64(max(w, h)))
		} else if r.Percent < 1 || r.Percent > 400 {
			return g, l10n.New("error.invalidScale")
		}
		g.Width, g.Height = round(float64(w)*scale), round(float64(h)*scale)
		g.ImageWidth, g.ImageHeight = g.Width, g.Height
	default:
		return g, l10n.New("error.invalidResizeMode")
	}
	// Reject extreme intermediate sizes in Cover as well as excessive outputs.
	if !validSize(g.Width, g.Height, MaxPixels) || !validSize(g.ImageWidth, g.ImageHeight, MaxPixels) {
		return g, l10n.New("error.scaledDimensionsLimit")
	}
	return g, nil
}
