package app

import (
	"context"
	"image"
	"slices"
	"time"

	"imageflow/internal/caption"
	"imageflow/internal/files"
	"imageflow/internal/imaging"
	"imageflow/internal/preview"
)

type captionRequest struct {
	path   string
	canvas image.Point
	font   caption.Typography
}

type captionSession struct {
	work    *preview.Latest[captionRequest, previewResult[imaging.CaptionPreview]]
	request captionRequest
	shown   captionRequest
	art     imaging.CaptionPreview
	err     error
}

func (m *Model) initCaption() {
	s := &m.caption
	s.work = previewWorker(m, 25*time.Millisecond, func(ctx context.Context, q captionRequest) (imaging.CaptionPreview, error) {
		return imaging.RenderPreviewCaption(ctx, q.canvas, q.font)
	}, func(art imaging.CaptionPreview, err error) {
		s.art, s.shown, s.err = art, s.request, err
	})
}

func (s *captionSession) clear() {
	if s.request != (captionRequest{}) {
		s.work.Cancel()
	}
	s.request, s.shown, s.art, s.err = captionRequest{}, captionRequest{}, imaging.CaptionPreview{}, nil
}

func (m *Model) requestCaption(c caption.Caption) {
	s := &m.caption
	base := m.preview.state.base
	if !c.Active() || base.Canvas == nil {
		s.clear()
		return
	}
	q := captionRequest{m.Selected, base.Size, c.Typography()}
	if s.request != q {
		// Keep the last rendered title while editing the same photo. It must
		// never carry over to a different photo or a different canvas size.
		if q.path != s.shown.path || q.canvas != s.shown.canvas {
			s.art, s.shown = imaging.CaptionPreview{}, captionRequest{}
		}
		s.request, s.err = q, nil
		s.work.Submit(q)
	}
}

func (m *Model) Caption() caption.Caption {
	return m.CaptionFor(m.SelectedSource())
}

func (m *Model) CaptionFor(s files.Source) caption.Caption { return m.captionWithStyle(s.Caption) }

func (m *Model) captionWithStyle(c caption.Caption) caption.Caption {
	if c.Style == (caption.Style{}) {
		c.Style = m.Preferences.CaptionStyle
		if c.Style == (caption.Style{}) {
			c.Style = caption.Defaults()
		}
	}
	return c
}

func (m *Model) SetCaption(c caption.Caption) {
	m.EditCaption(c, 0)
}

func (m *Model) EditCaption(c caption.Caption, group HistoryGroup) {
	m.EditCaptionFrom(m.Caption(), c, group)
}
func (m *Model) EditCaptionFrom(before, after caption.Caption, group HistoryGroup) {
	if before == after || m.Working || m.Collage.Exporting || !after.Valid() {
		return
	}
	var edits []sourceEdit[caption.Caption]
	for i := range m.Sources {
		s := &m.Sources[i]
		if !m.EditSelection[s.Path] && s.Path != m.Selected {
			continue
		}
		effective := m.captionWithStyle(s.Caption)
		next := patchCaption(effective, before, after)
		if next == effective {
			continue
		}
		if next.Text == "" && s.Caption.Style == (caption.Style{}) && next.Style == effective.Style {
			next.Style = caption.Style{}
		}
		if draft := m.history.draft; group != 0 && draft.group == group {
			if p, ok := draft.patch.(*sourcePatch[caption.Caption]); ok {
				j, found := slices.BinarySearchFunc(p.edits, i, func(e sourceEdit[caption.Caption], i int) int { return e.index - i })
				if found && m.captionWithStyle(p.edits[j].before) == next {
					next = p.edits[j].before
				}
			}
		}
		edits = append(edits, sourceEdit[caption.Caption]{i, s.Caption, next})
		s.Caption = next
	}
	if len(edits) > 0 {
		recordSources(m, HistoryCaption, group, edits, func(s *files.Source) *caption.Caption { return &s.Caption })
		m.ScopeRevision++
		m.requestCaptionEffects(m.Caption())
	}
}

func (m *Model) requestCaptionEffects(c caption.Caption) {
	if m.Mode != Before {
		m.requestCaption(c)
	}
	m.requestProof()
	m.updatePreviewStatus()
}

func (m *Model) CanSaveCaptionStyle(s caption.Style) bool {
	return m.configPath != "" && !m.configReadOnly && !m.PrefsPending && s.Valid() &&
		(s != m.Preferences.CaptionStyle || m.PreferenceError != nil)
}

// SaveCaptionStyle sets the default for photos without an explicit style.
// It stores only style, with no title or path, and leaves existing edits alone.
func (m *Model) SaveCaptionStyle(s caption.Style) {
	if m.CanSaveCaptionStyle(s) {
		p := m.Preferences
		p.CaptionStyle = s
		m.SetPreferences(p)
		m.ScopeRevision++
	}
}
