package app

import (
	"context"
	"image"
	"time"

	"imageflow/internal/crop"
	"imageflow/internal/imaging"
	"imageflow/internal/l10n"
	"imageflow/internal/preview"
	"imageflow/internal/series"
)

type baseRequest struct {
	path   string
	part   int
	resize imaging.Resize
	crop   crop.Region
}

type textRequest struct {
	canvas image.Point
	mark   imaging.TextMark
}

type proofRequest struct {
	path    string
	part    int
	grouped bool
	format  imaging.Format
	opts    imaging.Options
}

type previewResult[T any] struct {
	value T
	err   error
}

type previewSession struct {
	baseWork    *preview.Latest[baseLoad, previewResult[baseResult]]
	warmWork    *preview.Latest[warmRequest, previewResult[[2]cachedBase]]
	warmRequest warmRequest
	cache       baseCache
	textWork    *preview.Latest[textRequest, previewResult[imaging.TextPreview]]
	logoWork    *preview.Latest[logoPaths, previewResult[logoResult]]
	logoPaths   logoPaths
	logos       map[string]imaging.LogoPreview
	logoErrors  map[string]error
	logoPending bool
	proofWork   *preview.Latest[proofRequest, previewResult[imaging.Preview]]
	state       previewState
}

type previewState struct {
	path         string
	baseRequest  baseRequest
	base         imaging.PreviewBase
	basePart     int
	basePending  bool
	baseErr      error
	textRequest  textRequest
	text         imaging.TextPreview
	textPending  bool
	textErr      error
	proofRequest proofRequest
	proofPending bool
	proofErr     error
}

// PreviewFrame is either an exact encoded still or an editable canvas.
// The UI can update opacity and geometry without replacing Image.
type PreviewFrame struct {
	Image               image.Image
	Info                imaging.Info
	Layers              []imaging.PreviewLayer
	CertificationBounds image.Rectangle
	Background          string
	Thumbnail           bool
}

func previewWorker[Q, R any](m *Model, delay time.Duration, work func(context.Context, Q) (R, error), accept func(R, error)) *preview.Latest[Q, previewResult[R]] {
	var w *preview.Latest[Q, previewResult[R]]
	w = preview.New(m.ctx, delay, func(ctx context.Context, q Q) previewResult[R] {
		v, err := work(ctx, q)
		return previewResult[R]{v, err}
	}, func(rev uint64, r previewResult[R]) {
		m.onUI(func() {
			if w.IsCurrent(rev) {
				accept(r.value, r.err)
				m.updatePreviewStatus()
			}
		})
	})
	return w
}

func (m *Model) initPreview() {
	p := &m.preview
	m.initPreviewWarmup()
	p.baseWork = previewWorker(m, 30*time.Millisecond, loadBase, func(result baseResult, err error) {
		if result.unchanged {
			return
		}
		s := &p.state
		b := result.base
		s.basePending, s.baseErr = false, err
		if result.checked {
			// The file behind a warm entry changed. Invalidate all of its
			// variants, including any proof created from the old image.
			p.cache.removePaths(map[string]bool{s.path: true})
			m.removeThumbnails(map[string]bool{s.path: true})
			m.discardProof()
		}
		if b.Original != nil {
			m.Preview.Before, m.Preview.Input = b.Original, b.Input
		} else if s.basePart != s.baseRequest.part || result.checked {
			m.Preview.Before, m.Preview.Input = nil, imaging.Info{}
		}
		if b.Canvas != nil || s.basePart != s.baseRequest.part || result.checked {
			s.base = b
		}
		s.basePart = s.baseRequest.part
		if err == nil {
			p.cache.put(result.cachedBase, false)
		}
		m.requestPreviewLayers()
		m.requestProof()
		m.scheduleWarmup()
	})
	p.textWork = previewWorker(m, 20*time.Millisecond, func(ctx context.Context, q textRequest) (imaging.TextPreview, error) {
		return imaging.RenderPreviewText(ctx, q.canvas, q.mark)
	}, func(t imaging.TextPreview, err error) {
		p.state.text, p.state.textErr, p.state.textPending = t, err, false
	})
	p.logoWork = previewWorker(m, 0, loadLogoPreviews, func(result logoResult, _ error) {
		p.logoPaths, p.logoPending = logoPaths{}, false
		m.useLogoPreviews(result)
		m.pruneLogoPreviews()
	})
	// Encoding is useful for compression and file-size inspection, but should
	// never run for every event of a continuous edit.
	p.proofWork = previewWorker(m, 450*time.Millisecond, func(ctx context.Context, q proofRequest) (imaging.Preview, error) {
		v, err := imaging.RenderPreviewPart(ctx, q.path, q.opts, false, true, q.part)
		if q.grouped {
			v.Output.Format, v.Output.Bytes = q.format, 0 // This is one frame, not the assembled file.
		}
		return v, err
	}, func(result imaging.Preview, err error) {
		p.state.proofPending, p.state.proofErr = false, err
		if err == nil {
			m.Preview.After, m.Preview.Output = result.After, result.Output
		}
	})
}

func (p *previewSession) close() {
	p.baseWork.Close()
	p.warmWork.Close()
	p.textWork.Close()
	p.logoWork.Close()
	p.proofWork.Close()
}

func (m *Model) clearPreview() {
	p := &m.preview
	p.baseWork.Cancel()
	p.warmWork.Cancel()
	p.warmRequest = warmRequest{}
	p.textWork.Cancel()
	m.cancelLogoPreview()
	p.proofWork.Cancel()
	m.caption.clear()
	p.state = previewState{}
	m.Preview, m.PreviewErr, m.Rendering = imaging.Preview{}, nil, false
}

func (m *Model) requestPreview() {
	p, s := &m.preview, &m.preview.state
	if m.Working || m.Selected == "" {
		m.clearPreview()
		return
	}
	if s.path != m.Selected {
		s.path = m.Selected
		// Glyphs from the last photo cannot stand in for a different watermark.
		if t := m.Image.Text; s.textRequest.mark.Text != t.Text || s.textRequest.mark.Font != t.Font || s.textRequest.mark.Size != t.Size || s.textRequest.mark.Color != t.Color || s.textRequest.mark.Scale != t.Scale || s.textRequest.mark.Stroke != t.Stroke || s.textRequest.mark.Shadow != t.Shadow {
			p.textWork.Cancel()
			s.text, s.textRequest, s.textErr, s.textPending = imaging.TextPreview{}, textRequest{}, nil, false
		}
		s.base, s.basePart = imaging.PreviewBase{}, 0
		m.Preview.Before, m.Preview.Input = nil, imaging.Info{}
		m.discardProof()
	}
	r := m.Image.Resize
	r.LockRatio = false // Editing aid, not a pixel transform.
	region := m.SelectedSource().Crop
	if m.Mode == Before {
		region = crop.Region{}
	}
	if !r.Enabled || m.Mode == Before {
		r = imaging.Resize{}
	}
	q := baseRequest{m.Selected, max(1, m.PreviewPart), r, region}
	if s.baseRequest != q {
		skipDelay := s.baseRequest.path != q.path || s.baseRequest.part != q.part
		s.baseRequest, s.basePending, s.baseErr = q, true, nil
		if cached, ok := p.cache.get(q); ok {
			s.base, s.basePart, s.basePending = cached.base, q.part, false
			m.Preview.Before, m.Preview.Input = cached.base.Original, cached.base.Input
			p.baseWork.SubmitNow(baseLoad{key: q, stamp: cached.stamp})
		} else if skipDelay {
			p.baseWork.SubmitNow(baseLoad{key: q})
		} else {
			p.baseWork.Submit(baseLoad{key: q})
		}
	}
	m.requestPreviewLayers()
	m.requestProof()
	m.updatePreviewStatus()
	m.scheduleWarmup()
}

func (m *Model) requestPreviewLayers() {
	if m.Mode == Before {
		return
	}
	p, s := &m.preview, &m.preview.state
	t := m.Image.Text
	if t.Enabled && !t.Missing() {
		if s.base.Canvas != nil {
			t.Placement = imaging.Placement{} // Shape is independent of placement and opacity.
			q := textRequest{s.base.Size, t}
			if q != s.textRequest {
				s.textRequest, s.textPending, s.textErr = q, true, nil
				p.textWork.Submit(q)
			}
		}
	} else if s.textRequest != (textRequest{}) {
		p.textWork.Cancel()
		s.textRequest, s.text, s.textPending, s.textErr = textRequest{}, imaging.TextPreview{}, false, nil
	}
	m.requestLogoPreviews()
	m.requestCaption(m.Caption())
}

func (m *Model) requestProof() {
	p, s := &m.preview, &m.preview.state
	opts := m.Image.PreviewOptions()
	format := m.previewOutputFormat()
	opts.Format = format
	if format == imaging.TIFF {
		opts.Format = imaging.PNG
	}
	opts.Resize = s.baseRequest.resize
	opts.Crop = s.baseRequest.crop
	if !opts.Text.Enabled {
		opts.Text = imaging.TextMark{}
	}
	if !opts.Logo.Enabled {
		opts.Logo = imaging.LogoMark{}
	}
	if len(opts.LogoMarks()) == 0 {
		opts.LogoOnTop, opts.LogosBelowText = false, 0
	}
	src := m.SelectedSource()
	if src.Caption.Active() {
		opts.Caption = src.Caption
	}
	if g := m.certificationFor(src); g.Count() != 0 {
		opts.Certification = g
	}
	q := proofRequest{m.Selected, s.baseRequest.part, src.Series.Multiple() && !src.Selection.Separate, format, opts}
	if s.proofRequest != q {
		m.discardProof()
	}
	// Only the format screen needs to show the actual compression.
	if m.Mode != Encoded || s.basePending || s.baseErr != nil || s.base.Canvas == nil {
		if s.proofPending {
			m.discardProof()
		}
		return
	}
	if q == s.proofRequest {
		return
	}
	s.proofRequest, s.proofPending = q, true
	p.proofWork.Submit(q)
}

// OriginalReady distinguishes the selected frame/page from the previous
// picture we may keep on screen while a newly selected part is decoded.
func (m *Model) OriginalReady() bool {
	s := &m.preview.state
	return s.path == m.Selected && s.basePart == max(1, m.PreviewPart) && m.Preview.Before != nil
}

func (m *Model) BeforeFrame() PreviewFrame {
	if m.Preview.Before != nil {
		return PreviewFrame{Image: m.Preview.Before, Info: m.Preview.Input}
	}
	// A thumbnail is only a stand-in for its own photo's first part.
	// Keep it out of Preview.Before so crop and export use exact data.
	if m.preview.state.basePending && m.PreviewPart == 1 {
		if t := m.Thumbnail(m.Selected); t.Image != nil {
			return PreviewFrame{Image: t.Image, Info: t.Info, Thumbnail: true}
		}
	}
	return PreviewFrame{}
}

func (m *Model) previewOutputFormat() imaging.Format {
	s := m.SelectedSource()
	if s.Series.Multiple() && !s.Selection.Separate {
		if s.Series.Kind == series.Pages {
			return imaging.TIFF
		}
		return imaging.WebP
	}
	return m.Image.Format
}

func (m *Model) discardProof() {
	s := &m.preview.state
	if s.proofPending {
		m.preview.proofWork.Cancel()
	}
	s.proofRequest, s.proofPending, s.proofErr = proofRequest{}, false, nil
	m.Preview.After, m.Preview.Output = nil, imaging.Info{}
}

func (m *Model) updatePreviewStatus() {
	s := &m.preview.state
	m.Rendering = s.basePending
	m.PreviewErr = s.baseErr
	if m.Mode == Before {
		if m.Preview.Before != nil && s.basePart == s.baseRequest.part {
			m.PreviewErr = nil
		}
		return
	}
	t := m.Image.Text
	textActive := t.Enabled && !t.Missing()
	logoErr := m.logoPreviewError()
	m.Rendering = m.Rendering || (m.Mode == Encoded && s.proofPending) || (textActive && s.textPending) || m.preview.logoPending
	switch {
	case m.PreviewErr != nil:
	case s.base.Canvas != nil && m.Certification().Count() > s.base.Size.X:
		m.PreviewErr = l10n.New("error.certificationFit")
	case textActive && s.textErr != nil:
		m.PreviewErr = l10n.Wrap("error.textWatermark", s.textErr)
	case logoErr != nil:
		m.PreviewErr = logoErr
	case m.caption.err != nil:
		m.PreviewErr = l10n.Wrap("error.captionRender", m.caption.err)
	case m.Mode == Encoded:
		m.PreviewErr = s.proofErr
	}
}

// AfterFrame prefers the stable layers while editing, even after the exact
// proof has finished. Elsewhere, use the real encoded still when current.
func (m *Model) AfterFrame(editing bool) PreviewFrame {
	s := &m.preview.state
	if m.Preview.After != nil && (!editing || s.base.Canvas == nil) {
		return PreviewFrame{Image: m.Preview.After, Info: m.Preview.Output}
	}
	if s.base.Canvas == nil {
		if s.baseRequest.resize == (imaging.Resize{}) && !s.baseRequest.crop.Active() {
			opts := m.Image.PreviewOptions()
			if !opts.Text.Enabled && len(opts.LogoMarks()) == 0 {
				frame := m.BeforeFrame()
				if frame.Thumbnail {
					frame.Info.Format, frame.Info.Bytes = m.previewOutputFormat(), 0
					if frame.Info.Format == imaging.JPEG || opts.FlattenAlpha {
						frame.Background = opts.Background
					}
					return frame
				}
			}
		}
		return PreviewFrame{Image: m.Preview.Before, Info: m.Preview.Input}
	}
	format := m.previewOutputFormat()
	info := imaging.Info{Width: s.base.Size.X, Height: s.base.Size.Y, Format: format}
	if m.Preview.After != nil {
		info = m.Preview.Output
	}
	frame := PreviewFrame{Image: s.base.Canvas, Info: info}
	if m.caption.shown.path == m.Selected {
		frame.Layers = m.caption.art.Layers(s.base.Size, m.Caption())
	}
	frame.Layers = append(frame.Layers, imaging.PreviewLayers(s.base.Size, s.text, m.preview.logos, m.Image)...)
	badges, bounds, _ := imaging.CertificationLayers(s.base.Size, m.Certification(), m.certification.previews)
	frame.Layers = append(frame.Layers, badges...)
	frame.CertificationBounds = bounds
	if format == imaging.JPEG || m.Image.FlattenAlpha {
		frame.Background = m.Image.Background
	}
	return frame
}
