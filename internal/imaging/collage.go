package imaging

import (
	"context"
	"image"
	"io"
	"math"
	"path/filepath"

	"imageflow/internal/crop"
	"imageflow/internal/l10n"

	"github.com/davidbyttow/govips/v2/vips"
)

const MaxCollageImages = 16
const CollagePreviewEdge = 600

type CollageLayout int

const (
	CollageAcross CollageLayout = iota
	CollageDown
	CollageGrid
	CollageCustom
)

type CollageOptions struct {
	Width, Height, Gap int
	Layout             CollageLayout
	Fit                Fit
	Background         string
	Photos             [MaxCollageImages]CollagePhoto
}

func DefaultCollage() CollageOptions {
	return CollageOptions{Width: 1200, Height: 800, Gap: 16, Layout: CollageGrid, Fit: Contain, Background: "#FFFFFF"}
}

type CollageGeometry struct {
	Size          image.Point
	Columns, Rows int
	Cells         []image.Rectangle
}

func (o CollageOptions) Geometry(count int) (CollageGeometry, error) {
	var g CollageGeometry
	if count < 2 || count > MaxCollageImages {
		return g, l10n.Error("error.collageCount", l10n.Args{"limit": MaxCollageImages})
	}
	switch o.Layout {
	case CollageAcross:
		g.Columns, g.Rows = count, 1
	case CollageDown:
		g.Columns, g.Rows = 1, count
	case CollageGrid, CollageCustom:
		g.Columns = int(math.Ceil(math.Sqrt(float64(count))))
		g.Rows = (count + g.Columns - 1) / g.Columns
	default:
		return g, l10n.New("error.collageLayout")
	}
	if o.Fit != Contain && o.Fit != Cover {
		return g, l10n.New("error.chooseFit")
	}
	if _, _, _, err := ParseColor(o.Background); err != nil {
		return g, l10n.Wrap("error.backgroundColor", err)
	}
	if !validSize(o.Width, o.Height, MaxPixels) || o.Gap < 0 || o.Gap > 128 {
		return g, l10n.New("error.collageSize")
	}
	if o.Layout == CollageCustom {
		for _, p := range o.Photos[:count] {
			if p.Path == "" || p.Canvas.X < 1 || p.Canvas.Y < 1 || p.Original.X < 1 || p.Original.Y < 1 || p.Visible.Empty() || !p.Visible.In(p.Frame) || !p.Content.In(image.Rectangle{Max: p.Original}) || p.Content.Empty() {
				return g, l10n.New("error.collageLayout")
			}
		}
		g.Size = image.Pt(o.Width, o.Height)
		g.Cells = make([]image.Rectangle, count)
		return g, nil
	}
	insideW, insideH := o.Width-(g.Columns+1)*o.Gap, o.Height-(g.Rows+1)*o.Gap
	if insideW < g.Columns || insideH < g.Rows {
		return g, l10n.New("error.collageGap")
	}
	g.Size, g.Cells = image.Pt(o.Width, o.Height), make([]image.Rectangle, count)
	for i := range count {
		x, y := i%g.Columns, i/g.Columns
		left, top := (x+1)*o.Gap+x*insideW/g.Columns, (y+1)*o.Gap+y*insideH/g.Rows
		right, bottom := (x+1)*o.Gap+(x+1)*insideW/g.Columns, (y+1)*o.Gap+(y+1)*insideH/g.Rows
		g.Cells[i] = image.Rect(left, top, right, bottom)
	}
	return g, nil
}

// CollagePlacement uses source coordinates relative to the already cropped
// photo. The native preview and the exporter both draw this exact placement.
type CollagePlacement struct {
	Source, Target image.Rectangle
}

func (o CollageOptions) Place(cell image.Rectangle, src image.Point) CollagePlacement {
	if cell.Empty() || src.X <= 0 || src.Y <= 0 {
		return CollagePlacement{}
	}
	if o.Fit == Cover {
		w, h := src.X, src.Y
		if src.X*cell.Dy() > src.Y*cell.Dx() {
			w = max(1, (src.Y*cell.Dx()+cell.Dy()/2)/cell.Dy())
		} else {
			h = max(1, (src.X*cell.Dy()+cell.Dx()/2)/cell.Dx())
		}
		minPt := image.Pt((src.X-w)/2, (src.Y-h)/2)
		return CollagePlacement{image.Rectangle{Min: minPt, Max: minPt.Add(image.Pt(w, h))}, cell}
	}
	scale := min(float64(cell.Dx())/float64(src.X), float64(cell.Dy())/float64(src.Y))
	wh := image.Pt(min(cell.Dx(), max(1, int(math.Round(float64(src.X)*scale)))), min(cell.Dy(), max(1, int(math.Round(float64(src.Y)*scale)))))
	start := cell.Min.Add(cell.Size().Sub(wh).Div(2))
	return CollagePlacement{image.Rectangle{Max: src}, image.Rectangle{Min: start, Max: start.Add(wh)}}
}

type CollageInput struct {
	Path string
	Crop crop.Region
}

type CollageTile struct {
	Image image.Image
	Size  image.Point // Oriented, cropped size before downsampling.
}

func openCollageInput(ctx context.Context, path string) (*loaded, error) {
	s, err := openImage(ctx, path, MaxInputBytes, MaxInputPixels)
	if err != nil {
		return nil, &StageError{Decode, err}
	}
	if s.frame.series.Multiple() {
		s.close()
		return nil, &StageError{Decode, l10n.New("error.collageStillOnly")}
	}
	if err := s.normalize(); err != nil {
		s.close()
		return nil, &StageError{Transform, err}
	}
	return s, nil
}

func LoadCollageTile(ctx context.Context, input CollageInput) (CollageTile, error) {
	var out CollageTile
	s, err := openCollageInput(ctx, input.Path)
	if err != nil {
		return out, err
	}
	defer s.close()
	box, err := s.cropRect(input.Crop)
	if err != nil {
		return out, err
	}
	edge := CollagePreviewEdge
	im, err := s.rasterFor(box, image.Pt(edge, edge))
	if err != nil {
		return out, err
	}
	defer im.Close()
	out.Size = box.Size()
	out.Image, err = displayPixels(im, edge)
	return out, err
}

func ExportCollage(ctx context.Context, inputs []CollageInput, layout CollageOptions, opts Options, w io.Writer) (Info, error) {
	var info Info
	g, err := layout.Geometry(len(inputs))
	if err != nil {
		return info, &StageError{Transform, err}
	}
	opts.Resize, opts.Crop = Resize{}, crop.Region{}
	if err := opts.Validate(); err != nil {
		return info, &StageError{Transform, err}
	}
	r, g8, b, _ := ParseColor(layout.Background)
	bg := vips.Color{R: r, G: g8, B: b}
	canvas, err := vips.NewImageFromMemory([]byte{r, g8, b}, 1, 1, 3, vips.BandFormatUchar, vips.InterpretationSRGB)
	if err != nil {
		return info, &StageError{Transform, err}
	}
	defer canvas.Close()
	if err := canvas.Embed(0, 0, g.Size.X, g.Size.Y, vips.ExtendCopy); err != nil {
		return info, &StageError{Transform, err}
	}
	for i, input := range inputs {
		if err := ctx.Err(); err != nil {
			return info, err
		}
		if err := addCollageTile(ctx, canvas, input, layout, g.Cells[i], &bg); err != nil {
			return info, l10n.WrapWithArgs("error.collageMaterial", err, l10n.Args{"file": filepath.Base(input.Path)})
		}
	}
	err = finishCanvas(ctx, canvas, opts, nil, func(im *vips.ImageRef, got Info) error {
		info = got
		return encode(ctx, im, opts, w)
	})
	return info, err
}

func addCollageTile(ctx context.Context, canvas *vips.ImageRef, input CollageInput, layout CollageOptions, cell image.Rectangle, bg *vips.Color) error {
	s, err := openCollageInput(ctx, input.Path)
	if err != nil {
		return err
	}
	defer s.close()
	box, err := s.cropRect(input.Crop)
	if err != nil {
		return &StageError{Transform, err}
	}
	p := layout.Place(cell, box.Size())
	photo, custom := layout.Photo(input.Path)
	if layout.Layout == CollageCustom && !custom {
		return l10n.New("error.collageLayout")
	}
	custom = custom && layout.Layout == CollageCustom
	if custom {
		photo = photo.OnCanvas(image.Pt(layout.Width, layout.Height))
		p = photo.Placement(box.Size())
	}
	size := p.Target.Size()
	im, err := s.rasterFor(p.Source.Add(box.Min), size)
	if err != nil {
		return &StageError{Transform, err}
	}
	defer im.Close()
	if err := resizeTo(im, size.X, size.Y); err != nil {
		return &StageError{Transform, err}
	}
	if !custom && im.HasAlpha() {
		if err := im.Flatten(bg); err != nil {
			return &StageError{Transform, err}
		}
	}
	if err := im.Cast(vips.BandFormatUchar); err != nil {
		return &StageError{Transform, err}
	}
	// Materialize only the fitted tile. The final canvas graph can retain
	// tiles, but no longer retains any of the full-resolution input graphs.
	if custom && !im.HasAlpha() {
		if err := im.AddAlpha(); err != nil {
			return err
		}
	}
	pixels, err := im.ToBytes()
	if err != nil {
		return &StageError{Transform, err}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	bands := 3
	if custom {
		bands = 4
		maskCollageCorners(pixels, size, photo)
	}
	tile, err := vips.NewImageFromMemory(pixels, size.X, size.Y, bands, vips.BandFormatUchar, vips.InterpretationSRGB)
	if err != nil {
		return &StageError{Transform, err}
	}
	defer tile.Close()
	if custom {
		return canvas.Composite(tile, vips.BlendModeOver, p.Target.Min.X, p.Target.Min.Y)
	}
	if err := canvas.Insert(tile, p.Target.Min.X, p.Target.Min.Y, false, nil); err != nil {
		return &StageError{Transform, err}
	}
	return nil
}
