package imaging

import (
	"bytes"
	"context"
	"image"
	_ "image/png" // Display pixels only; PNG processing/encoding uses libvips.
	"io"
	"math"
	"path/filepath"
	"runtime"

	"imageflow/internal/crop"
	"imageflow/internal/imageformat"
	"imageflow/internal/l10n"
	"imageflow/internal/series"

	"github.com/davidbyttow/govips/v2/vips"
)

type Stage string

const (
	Decode    Stage = "stage.decode"
	Transform Stage = "stage.transform"
	Encode    Stage = "stage.encode"
)

type StageError struct {
	Stage Stage
	Err   error
}

func (e *StageError) Error() string { return e.Localized(l10n.Default) }
func (e *StageError) Unwrap() error { return e.Err }
func (e *StageError) Localized(tag string) string {
	return l10n.Format(tag, "error.stage", l10n.Args{"stage": l10n.T(tag, string(e.Stage)), "cause": l10n.ErrorText(tag, e.Err)})
}

type Info struct {
	Format        Format
	Width, Height int
	Bytes         int64
	Alpha         bool
}

type Preview struct {
	Before, After image.Image
	Input, Output Info
}

func Start() error {
	vips.LoggingSettings(nil, vips.LogLevelWarning)
	if err := vips.Startup(&vips.Config{ConcurrencyLevel: 1, MaxCacheSize: 0, MaxCacheFiles: 0, MaxCacheMem: 0}); err != nil {
		return err
	}
	for _, format := range []Format{JPEG, PNG, WebP, TIFF, GIF, HEIF, AVIF, JXL, JP2, SVG} {
		if !vips.IsTypeSupported(vipsFormat(format)) {
			vips.Shutdown()
			return l10n.Error("error.codecUnavailable", l10n.Args{"version": vips.Version, "format": format})
		}
	}
	return nil
}

func Shutdown()       { vips.Shutdown() }
func Version() string { return vips.Version }

type loaded struct {
	*vips.ImageRef
	pin     runtime.Pinner
	info    Info
	frame   frameMeta
	svg     []byte // Keep SVG vectors until the requested preview or export size is known.
	release func()
}

func (s *loaded) close() {
	if s.ImageRef != nil {
		s.ImageRef.Close()
	}
	s.pin.Unpin()
	if s.release != nil {
		s.release()
	}
}

func openImage(ctx context.Context, path string, maxBytes, maxPixels int) (*loaded, error) {
	return openImagePart(ctx, path, maxBytes, maxPixels, 1)
}

type imageBuffer struct {
	data    []byte
	format  Format
	rawHint string
	release func()
	apng    *pngAnimation
}

func (b *imageBuffer) close() {
	if b.release != nil {
		b.release()
	}
}

func readImageBuffer(ctx context.Context, path string, maxBytes int, combine bool) (*imageBuffer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	format := FormatOf(path)
	maxBytes = inputLimit(format, maxBytes)
	f, size, err := openInput(path, maxBytes)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	container, err := inspectContainer(f, size, format)
	if err != nil {
		return nil, err
	}
	// A large compressed input or an eager system raster needs its slot
	// before allocating. Large lazy decodes reserve a slot after probing.
	var release func()
	defer func() {
		if release != nil {
			release()
		}
	}()
	if combine || size > 24<<20 || container.nativeAnimation || container.apngFrames != 0 || format == SVG || format == JXL || usesSystemRaster(f, format) {
		release, err = acquire(ctx, largeInput)
		if err != nil {
			return nil, err
		}
	}
	data, err := readInput(f, maxBytes)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b := &imageBuffer{data: data, format: format, rawHint: imageformat.RawTypeHint(path), release: release}
	if container.apngFrames != 0 {
		b.apng, err = parsePNGAnimation(ctx, data, container.apngFrames)
		if err != nil {
			return nil, err
		}
	}
	release = nil
	return b, nil
}

func (b *imageBuffer) part(ctx context.Context, index, limit int) (*loaded, error) {
	if b.apng != nil {
		s, err := b.apng.part(ctx, index, limit)
		if err == nil {
			s.info.Bytes = int64(len(b.data))
		}
		return s, err
	}
	return loadBufferPart(b.data, b.format, limit, b.rawHint, index)
}

func openImagePart(ctx context.Context, path string, maxBytes, maxPixels, index int) (*loaded, error) {
	b, err := readImageBuffer(ctx, path, maxBytes, false)
	if err != nil {
		return nil, err
	}
	defer b.close()
	out, err := b.part(ctx, index, maxPixels)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		out.close()
		return nil, err
	}
	if b.release == nil && !validSize(out.info.Width, out.info.Height, MaxPixels) {
		b.release, err = acquire(ctx, largeInput)
		if err != nil {
			out.close()
			return nil, err
		}
	}
	out.release, b.release = b.release, nil
	return out, nil
}

func loadBuffer(data []byte, format Format, limit int) (*loaded, error) {
	return loadBufferPart(data, format, limit, "", 1)
}

func openLogo(ctx context.Context, path string) (*loaded, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, _, err := openInput(path, 16<<20)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := readInput(f, 16<<20)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return loadBuffer(data, PNG, MaxLogoPixels)
}

func loadBufferPart(data []byte, format Format, limit int, rawHint string, index int) (*loaded, error) {
	input := bytes.NewReader(data)
	container, err := inspectContainer(input, int64(len(data)), format)
	if err != nil {
		return nil, err
	}
	if index < 1 || index > series.MaxParts {
		return nil, l10n.New("error.seriesChanged")
	}
	if container.apngFrames != 0 {
		return nil, l10n.New("error.multipleImages")
	}
	if container.nativeAnimation {
		meta, info, im, err := systemAnimation(data, format, index, limit, true)
		if err != nil {
			return nil, err
		}
		info.Bytes = int64(len(data))
		return &loaded{ImageRef: im, info: info, frame: meta}, nil
	}
	if format == SVG {
		info, _, err := svgRaster(data, limit, 1, false)
		if err != nil {
			return nil, err
		}
		info.Bytes = int64(len(data))
		return &loaded{info: info, svg: data}, nil
	}
	if usesSystemRaster(input, format) {
		info, im, err := systemRaster(data, format, limit, true, rawHint)
		if err != nil {
			return nil, err
		}
		info.Bytes = int64(len(data))
		return &loaded{ImageRef: im, info: info}, nil
	}
	s := &loaded{}
	// The libvips buffer loader retains the Go byte slice for lazy decode.
	// Keep it pinned until all derived graphs have finished evaluating.
	s.pin.Pin(&data[0])
	p := vips.NewImportParams()
	switch format {
	case GIF, WebP, TIFF, HEIF, AVIF:
		p.Page.Set(index - 1)
		p.NumPages.Set(1)
	}
	im, err := vips.LoadImageFromBuffer(data, p)
	if err != nil {
		s.pin.Unpin()
		return nil, err
	}
	s.ImageRef = im
	if err = checkImageBounds(im, limit); err != nil {
		s.close()
		return nil, err
	}
	s.frame, err = vipsSeries(im, format, index)
	if err != nil {
		s.close()
		return nil, err
	}
	if format == JXL && index > 1 {
		meta := s.frame
		s.close()
		if !meta.series.Multiple() {
			return nil, l10n.New("error.seriesChanged")
		}
		png, err := jxlPage(data, index, limit)
		if err != nil {
			return nil, err
		}
		s, err := loadBuffer(png, PNG, limit)
		if err != nil {
			return nil, err
		}
		s.frame, s.info.Format, s.info.Bytes = meta, JXL, int64(len(data))
		return s, nil
	}
	if err = im.AutoRotate(); err != nil {
		s.close()
		return nil, err
	}
	s.info = Info{Format: format, Width: im.Width(), Height: im.Height(), Alpha: im.HasAlpha(), Bytes: int64(len(data))}
	return s, nil
}

func srgb(im *vips.ImageRef) error {
	if im.HasICCProfile() {
		fallback := "srgb"
		if im.Interpretation() == vips.InterpretationCMYK {
			fallback = "cmyk"
		}
		if err := im.TransformICCProfileWithFallback("srgb", fallback); err != nil {
			return l10n.Wrap("error.iccConversion", err)
		}
	}
	if err := im.ToColorSpace(vips.InterpretationSRGB); err != nil {
		return l10n.Wrap("error.srgbConversion", err)
	}
	return nil
}

func (s *loaded) normalize() error {
	if s.ImageRef == nil {
		return nil // librsvg produces sRGB pixels.
	}
	return srgb(s.ImageRef)
}

func (s *loaded) svgAt(width, height, limit int) (*vips.ImageRef, error) {
	w, h := float64(s.info.Width), float64(s.info.Height)
	// Keep a narrow SVG at least one pixel on its short axis. The shared
	// resize step below still enforces the exact requested canvas.
	scale := max(1/min(w, h), min(float64(width)/w, float64(height)/h))
	_, im, err := svgRaster(s.svg, limit, scale, true)
	return im, err
}

func (s *loaded) originalPixels(edge int) (image.Image, error) {
	var im *vips.ImageRef
	var err error
	if s.svg != nil {
		im, err = s.svgAt(edge, edge, MaxInputPixels)
	} else {
		im, err = s.Copy()
	}
	if err != nil {
		return nil, err
	}
	defer im.Close()
	return displayPixels(im, edge)
}

func resizeTo(im *vips.ImageRef, w, h int) error {
	if w == im.Width() && h == im.Height() {
		return nil
	}
	alpha := im.HasAlpha()
	if alpha {
		if err := im.PremultiplyAlpha(); err != nil {
			return err
		}
	}
	if err := im.ResizeWithVScale(float64(w)/float64(im.Width()), float64(h)/float64(im.Height()), vips.KernelLanczos3); err != nil {
		return err
	}
	if alpha {
		if err := im.UnpremultiplyAlpha(); err != nil {
			return err
		}
	}
	// libvips rounds intermediate reductions. Enforce the requested pixel
	// boundary even when rounding gives a one-pixel difference.
	if im.Width() > w || im.Height() > h {
		if err := im.ExtractArea(0, 0, min(w, im.Width()), min(h, im.Height())); err != nil {
			return err
		}
	}
	if im.Width() != w || im.Height() != h {
		return im.Embed(0, 0, w, h, vips.ExtendCopy)
	}
	return nil
}

func adjust(im *vips.ImageRef, r Resize, g Geometry) error {
	if err := resizeTo(im, g.ImageWidth, g.ImageHeight); err != nil {
		return err
	}
	if g.Width == im.Width() && g.Height == im.Height() {
		return nil
	}
	if r.Fit == Cover {
		return im.ExtractArea(-g.X, -g.Y, g.Width, g.Height)
	}
	if r.Transparent {
		if !im.HasAlpha() {
			if err := im.AddAlpha(); err != nil {
				return err
			}
		}
		return im.EmbedBackgroundRGBA(g.X, g.Y, g.Width, g.Height, &vips.ColorRGBA{})
	}
	rr, gg, bb, _ := ParseColor(r.PadColor)
	return im.EmbedBackground(g.X, g.Y, g.Width, g.Height, &vips.Color{R: rr, G: gg, B: bb})
}

func (s *loaded) cropRect(region crop.Region) (image.Rectangle, error) {
	box := region.Resolve(image.Pt(s.info.Width, s.info.Height))
	if box.Empty() {
		return box, l10n.New("error.invalidCrop")
	}
	return box, nil
}

func (s *loaded) rasterFor(box image.Rectangle, target image.Point) (*vips.ImageRef, error) {
	size := image.Pt(s.info.Width, s.info.Height)
	if s.svg == nil {
		im, err := s.Copy()
		if err == nil && box != (image.Rectangle{Max: size}) {
			if err = im.ExtractArea(box.Min.X, box.Min.Y, box.Dx(), box.Dy()); err != nil {
				im.Close()
				return nil, err
			}
		}
		return im, err
	}
	if box == (image.Rectangle{Max: size}) {
		return s.svgAt(target.X, target.Y, MaxPixels)
	}
	// The vector still rasterizes at a useful resolution for the chosen
	// region. Bound the full raster before extracting the selected rectangle.
	w, h := float64(size.X), float64(size.Y)
	wanted := max(float64(target.X)/float64(box.Dx()), float64(target.Y)/float64(box.Dy()))
	ceiling := min(float64(MaxSide-1)/max(w, h), math.Sqrt(float64(MaxInputPixels)/(w+1)/(h+1)))
	// A 1 px source strip must survive downsampling.
	scale := min(max(wanted, 1/min(w, h)), max(1, ceiling))
	_, im, err := svgRaster(s.svg, MaxInputPixels, scale, true)
	if err != nil {
		return nil, err
	}
	xs, ys := float64(im.Width())/w, float64(im.Height())/h
	px := func(v int, scale float64) int { return int(math.Round(float64(v) * scale)) }
	x, y := min(px(box.Min.X, xs), im.Width()-1), min(px(box.Min.Y, ys), im.Height()-1)
	right, bottom := min(im.Width(), max(x+1, px(box.Max.X, xs))), min(im.Height(), max(y+1, px(box.Max.Y, ys)))
	if err := im.ExtractArea(x, y, right-x, bottom-y); err != nil {
		im.Close()
		return nil, err
	}
	return im, nil
}

func (s *loaded) canvas(r Resize, region crop.Region) (*vips.ImageRef, error) {
	box, err := s.cropRect(region)
	if err != nil {
		return nil, err
	}
	g, err := r.Geometry(box.Dx(), box.Dy())
	if err != nil {
		return nil, err
	}
	im, err := s.rasterFor(box, image.Pt(g.ImageWidth, g.ImageHeight))
	if err != nil {
		return nil, err
	}
	if err = adjust(im, r, g); err != nil {
		im.Close()
		return nil, err
	}
	return im, nil
}

func opacity(im *vips.ImageRef, value int) error {
	if value == 100 {
		return nil
	}
	if err := im.Linear([]float64{1, 1, 1, float64(value) / 100}, []float64{0, 0, 0, 0}); err != nil {
		return err
	}
	return im.Cast(vips.BandFormatUchar)
}

// process keeps the lazy source and logo graphs alive until consume has
// finished encoding; no ImageRef is shared across goroutines.
func process(ctx context.Context, src *loaded, opts Options, logos logoSources, consume func(*vips.ImageRef, Info) error) error {
	if err := opts.Validate(); err != nil {
		return &StageError{Transform, err}
	}
	im, err := src.canvas(opts.Resize, opts.Crop)
	if err != nil {
		return &StageError{Transform, err}
	}
	defer im.Close()
	return finishCanvas(ctx, im, opts, logos, consume)
}

func finishCanvas(ctx context.Context, im *vips.ImageRef, opts Options, sources logoSources, consume func(*vips.ImageRef, Info) error) error {
	var badges certificationArtwork
	defer badges.close()
	fail := func(err error) error { return &StageError{Transform, err} }
	marks := opts.LogoMarks()
	// The batch cache holds at most one photo’s stack. Other photos can use
	// different artwork. Decode only their missing marks for this output.
	var missing []LogoMark
	for _, mark := range marks {
		if _, ok := sources[mark.Path]; !ok && mark.Opacity > 0 {
			missing = append(missing, mark)
		}
	}
	var local logoSources
	if len(missing) > 0 {
		local = prepareLogoSources(ctx, missing)
		defer local.close()
	}
	applyLogos := func(marks []LogoMark) error {
		for _, logo := range marks {
			if err := ctx.Err(); err != nil {
				return err
			}
			if logo.Opacity == 0 {
				continue
			}
			source, found := sources[logo.Path]
			if !found {
				source = local[logo.Path]
			}
			if source.err != nil {
				return source.err
			}
			mark, err := source.image.Copy()
			if err == nil {
				err = overlayLogo(im, mark, logo)
				mark.Close()
			}
			if err != nil {
				return l10n.WrapWithArgs("error.logoLayerFile", err, l10n.Args{"file": filepath.Base(logo.Path)})
			}
		}
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := overlayCaption(ctx, im, opts.Caption); err != nil {
		return fail(l10n.Wrap("error.captionRender", err))
	}
	pos := opts.TextLayerPosition(len(marks))
	if err := applyLogos(marks[:pos]); err != nil {
		return fail(err)
	}
	if err := overlayText(im, opts.Text); err != nil {
		return fail(l10n.Wrap("error.textWatermark", err))
	}
	if err := applyLogos(marks[pos:]); err != nil {
		return fail(err)
	}
	if err := badges.composite(ctx, im, opts.Certification); err != nil {
		return fail(l10n.Wrap("error.certificationRender", err))
	}
	if (opts.Format == JPEG || opts.FlattenAlpha) && im.HasAlpha() {
		r, g, b, _ := ParseColor(opts.Background)
		if err := im.Flatten(&vips.Color{R: r, G: g, B: b}); err != nil {
			return fail(err)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return consume(im, Info{Format: opts.Format, Width: im.Width(), Height: im.Height(), Alpha: im.HasAlpha()})
}

type contextWriter struct {
	ctx context.Context
	w   io.Writer
}

func (w contextWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.w.Write(p)
}

var largeAVIF = make(chan struct{}, 1)

func encode(ctx context.Context, im *vips.ImageRef, opts Options, w io.Writer) error {
	out := contextWriter{ctx, w}
	var err error
	switch opts.Format {
	case JPEG:
		err = im.SaveToWriterJpeg(out, &vips.JpegExportParams{Quality: opts.Quality, OptimizeCoding: true, Keep: vips.ForeignKeepIcc})
	case PNG:
		err = im.SaveToWriterPng(out, &vips.PngExportParams{Compression: 6, Keep: vips.ForeignKeepIcc})
	case WebP:
		p := vips.NewWebpExportParams()
		p.Quality, p.Lossless, p.Keep = opts.Quality, opts.Lossless, vips.ForeignKeepIcc
		if opts.Lossless {
			p.Quality = 85
		}
		err = im.SaveToWriterWebp(out, p)
	case AVIF:
		// The libheif encoder materializes a full frame. Keep large AVIF
		// encodes from running concurrently, including the comparison preview.
		if !validSize(im.Width(), im.Height(), 12_000_000) {
			release, err := acquire(ctx, largeAVIF)
			if err != nil {
				return err
			}
			defer release()
		}
		var data []byte
		data, _, err = im.ExportAvif(&vips.AvifExportParams{
			Quality: opts.Quality, Bitdepth: 8, Effort: 3, Keep: vips.ForeignKeepIcc,
		})
		if err == nil {
			var n int
			n, err = out.Write(data)
			if err == nil && n != len(data) {
				err = io.ErrShortWrite
			}
		}
	default:
		err = l10n.New("error.unsupportedFormat")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return &StageError{Encode, err}
	}
	return nil
}

// Export performs one transformation and encodes to w. The caller owns the
// temporary output and its atomic publication.
func Export(ctx context.Context, path string, opts Options, w io.Writer) (Info, error) {
	return ExportPart(ctx, path, opts, 1, series.Info{}, w)
}

// ExportPart processes one explicitly selected frame/page, or a still image
// when expected is empty. A caller may not silently drop the other frames.
func ExportPart(ctx context.Context, path string, opts Options, index int, expected series.Info, w io.Writer) (Info, error) {
	return exportPart(ctx, path, opts, index, expected, nil, w)
}

func exportPart(ctx context.Context, path string, opts Options, index int, expected series.Info, logos logoSources, w io.Writer) (Info, error) {
	var info Info
	if index < 1 || index > max(1, expected.Count) {
		return info, &StageError{Decode, l10n.New("error.seriesChanged")}
	}
	src, err := openImagePart(ctx, path, MaxInputBytes, MaxInputPixels, index)
	if err != nil {
		return info, &StageError{Decode, err}
	}
	defer src.close()
	if expected.Multiple() && src.frame.series != expected {
		return info, &StageError{Decode, l10n.New("error.seriesChanged")}
	}
	if !expected.Multiple() && src.frame.series.Multiple() {
		return info, &StageError{Decode, l10n.New("error.seriesScope")}
	}
	if err = src.normalize(); err != nil {
		return info, &StageError{Transform, err}
	}
	err = process(ctx, src, opts, logos, func(im *vips.ImageRef, in Info) error {
		info = in
		return encode(ctx, im, opts, w)
	})
	return info, err
}

func displayPixels(im *vips.ImageRef, edge int) (image.Image, error) {
	return displayScaled(im, float64(edge)/float64(max(im.Width(), im.Height())))
}

func displayScaled(im *vips.ImageRef, scale float64) (image.Image, error) {
	scale = min(1, scale)
	if err := resizeTo(im, max(1, int(math.Round(float64(im.Width())*scale))), max(1, int(math.Round(float64(im.Height())*scale)))); err != nil {
		return nil, err
	}
	// Most previews already use sRGB pixels. Send those to Go directly
	// instead of spending time encoding a temporary PNG and decoding it.
	bands, format := im.Bands(), im.BandFormat()
	if im.Interpretation() == vips.InterpretationSRGB && (bands == 3 || bands == 4) &&
		(format == vips.BandFormatUchar || format == vips.BandFormatFloat) {
		if format != vips.BandFormatUchar {
			if err := im.Cast(vips.BandFormatUchar); err != nil {
				return nil, err
			}
		}
		if bands == 3 {
			if err := im.AddAlpha(); err != nil {
				return nil, err
			}
		}
		pixels, err := im.ToBytes()
		if err != nil {
			return nil, err
		}
		w, h := im.Width(), im.Height()
		if len(pixels) == w*h*4 {
			rect := image.Rect(0, 0, w, h)
			if bands == 4 {
				return &image.NRGBA{Pix: pixels, Stride: w * 4, Rect: rect}, nil
			}
			// Opaque pixels are already premultiplied and MyGo can reuse them.
			return &image.RGBA{Pix: pixels, Stride: w * 4, Rect: rect}, nil
		}
	}
	// Keep the established conversion for unusual channel layouts or depths.
	out, _, err := im.ExportPng(&vips.PngExportParams{Compression: 1, StripMetadata: true})
	if err != nil {
		return nil, err
	}
	imGo, _, err := image.Decode(bytes.NewReader(out))
	return imGo, err
}

type previewBuffer struct{ bytes.Buffer }

func (b *previewBuffer) Write(p []byte) (int, error) {
	if len(p) > (100<<20)-b.Len() {
		return 0, l10n.New("error.previewTooLarge")
	}
	return b.Buffer.Write(p)
}

// RenderPreview decodes the actual encoded output before downsampling for
// display. This includes format artifacts, alpha, ICC and final pixel geometry.
func RenderPreview(ctx context.Context, path string, opts Options, before, after bool) (Preview, error) {
	return RenderPreviewPart(ctx, path, opts, before, after, 1)
}

func RenderPreviewPart(ctx context.Context, path string, opts Options, before, after bool, index int) (Preview, error) {
	opts = opts.PreviewOptions()
	var result Preview
	src, err := openImagePart(ctx, path, MaxInputBytes, MaxInputPixels, index)
	if err != nil {
		return result, &StageError{Decode, err}
	}
	defer src.close()
	result.Input = src.info
	if err = src.normalize(); err != nil {
		return result, err
	}
	if before {
		result.Before, err = src.originalPixels(PreviewEdge)
		if err != nil {
			return result, err
		}
	}
	if after {
		err = process(ctx, src, opts, nil, func(im *vips.ImageRef, info Info) error {
			var out previewBuffer
			if err := encode(ctx, im, opts, &out); err != nil {
				return err
			}
			info.Bytes = int64(out.Len())
			result.Output = info
			reloaded, err := loadBuffer(out.Bytes(), opts.Format, MaxPixels)
			if err != nil {
				return err
			}
			defer reloaded.close()
			if err = ctx.Err(); err != nil {
				return err
			}
			result.After, err = displayPixels(reloaded.ImageRef, PreviewEdge)
			return err
		})
	}
	return result, err
}

func Thumbnail(ctx context.Context, path string, edge int) (image.Image, Info, error) {
	return ThumbnailPart(ctx, path, 1, edge)
}
func ThumbnailPart(ctx context.Context, path string, part, edge int) (image.Image, Info, error) {
	src, err := openImagePart(ctx, path, MaxInputBytes, MaxInputPixels, part)
	if err != nil {
		return nil, Info{}, err
	}
	defer src.close()
	if err = src.normalize(); err != nil {
		return nil, src.info, err
	}
	im, err := src.originalPixels(min(256, max(16, edge)))
	return im, src.info, err
}
