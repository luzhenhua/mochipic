package app

import (
	"context"
	"errors"
	"image"
	"maps"
	"path/filepath"
	"slices"

	"imageflow/internal/files"
	"imageflow/internal/imaging"
	"imageflow/internal/l10n"
)

type logoPaths [imaging.MaxLogoLayers]string

type logoResult struct {
	pixels map[string]imaging.LogoPreview
	issues map[string]error
}

func loadLogoPreviews(ctx context.Context, paths logoPaths) (logoResult, error) {
	result := logoResult{pixels: make(map[string]imaging.LogoPreview), issues: make(map[string]error)}
	for _, path := range paths {
		if path == "" {
			break
		}
		if err := ctx.Err(); err != nil {
			return logoResult{}, err
		}
		if mark, err := imaging.RenderPreviewLogo(ctx, path); err != nil {
			result.issues[path] = l10n.WrapWithArgs("error.logoLayerFile", err, l10n.Args{"file": filepath.Base(path)})
		} else {
			result.pixels[path] = mark
		}
	}
	return result, nil
}

func (m *Model) pruneLogoPreviews() {
	p := &m.preview
	marks := m.Image.LogoMarks()
	keep := func(path string) bool {
		return slices.ContainsFunc(marks, func(mark imaging.LogoMark) bool { return mark.Path == path })
	}
	for path := range p.logos {
		if !keep(path) {
			delete(p.logos, path)
		}
	}
	for path := range p.logoErrors {
		if !keep(path) {
			delete(p.logoErrors, path)
		}
	}
}

func (m *Model) cancelLogoPreview() {
	p := &m.preview
	p.logoWork.Cancel()
	p.logoPaths, p.logoPending = logoPaths{}, false
}

func (m *Model) requestLogoPreviews() {
	p := &m.preview
	var q logoPaths
	n := 0
	for _, mark := range m.Image.LogoMarks() {
		if !mark.Missing() && p.logos[mark.Path].Pixels() == nil && p.logoErrors[mark.Path] == nil {
			q[n] = mark.Path
			n++
		}
	}
	slices.Sort(q[:n])
	if q == p.logoPaths {
		return
	}
	if n == 0 {
		m.cancelLogoPreview()
		return
	}
	p.logoPaths, p.logoPending = q, true
	p.logoWork.Submit(q)
}

func (m *Model) useLogoPreviews(r logoResult) {
	p := &m.preview
	if p.logos == nil {
		p.logos, p.logoErrors = make(map[string]imaging.LogoPreview), make(map[string]error)
	}
	for path, pixels := range r.pixels {
		p.logos[path] = pixels
		delete(p.logoErrors, path)
	}
	for path, err := range r.issues {
		p.logoErrors[path] = err
	}
}

func (m *Model) LogoThumbnail(path string) image.Image { return m.preview.logos[path].Pixels() }

func (m *Model) logoPreviewError() error {
	for _, mark := range m.Image.LogoMarks() {
		if err := m.preview.logoErrors[mark.Path]; err != nil {
			return err
		}
	}
	return nil
}

// AddLogos validates a bounded selection off the UI thread. A bad file does
// not prevent the other valid files in the selection from being added.
func (m *Model) AddLogos(paths []string) {
	if m.Working || m.Collage.Exporting || m.LogoPending || len(paths) == 0 || len(m.EditSelection) == 0 {
		return
	}
	targets := maps.Clone(m.EditSelection)
	var q logoPaths
	var n int
	var truncated bool
	for _, path := range paths {
		path = filepath.Clean(path)
		if slices.Contains(q[:n], path) {
			continue
		}
		if n == len(q) {
			truncated = true
			break
		}
		q[n] = path
		n++
	}
	if n == 0 {
		if truncated {
			m.Error = logoLimitError()
		}
		return
	}
	m.LogoPending, m.logoTargets = true, targets
	m.wg.Go(func() {
		r, _ := loadLogoPreviews(m.ctx, q)
		m.onUI(func() {
			m.LogoPending, m.logoTargets = false, nil
			if m.Working || m.Collage.Exporting || len(targets) == 0 {
				return
			}
			var issues []error
			var valid []string
			for _, path := range q[:n] {
				if err := r.issues[path]; err != nil {
					issues = append(issues, err)
				} else if r.pixels[path].Pixels() != nil {
					valid = append(valid, path)
				}
			}
			changed := m.editPhotos(targets, HistoryLogoImport, 0, func(p files.PhotoSettings) files.PhotoSettings {
				o := p.Image
				marks := o.LogoMarks()
				for _, path := range valid {
					if slices.ContainsFunc(marks, func(l imaging.LogoMark) bool { return l.Path == path }) {
						continue
					}
					if len(marks) == imaging.MaxLogoLayers {
						truncated = true
						break
					}
					logo := o.Logo
					if len(marks) > 0 {
						logo = imaging.DefaultLogo()
						logo.Anchor = nextLogoAnchor(marks)
					}
					logo.Path, logo.Enabled = path, true
					marks = append(marks, logo)
				}
				o.SetLogoMarks(marks)
				if o != p.Image {
					p.Image, p.Preset = o, ""
				}
				return p
			})
			if changed {
				if targets[m.Selected] {
					m.useLogoPreviews(logoResult{pixels: r.pixels})
				}
				if targets[m.Selected] && len(valid) > 0 {
					m.LogoImportRevision++
					m.LastAddedLogo = valid[len(valid)-1]
				}
				m.refreshImageEdit(false)
			}
			if truncated {
				issues = append(issues, logoLimitError())
			}
			if len(issues) != 0 {
				m.Error = errors.Join(issues...)
			}
		})
	})
}

func logoLimitError() error {
	return l10n.Error("error.logoLayerLimit", l10n.Args{"count": imaging.MaxLogoLayers})
}

func nextLogoAnchor(marks []imaging.LogoMark) int {
	var occupied [9]bool
	for _, mark := range marks {
		anchor := mark.Anchor
		if mark.Free.Active {
			axis := func(n int) int { return min(2, max(0, n*3/1_000_000)) }
			anchor = axis(mark.Free.Y)*3 + axis(mark.Free.X)
		}
		if anchor >= 0 && anchor < len(occupied) {
			occupied[anchor] = true
		}
	}
	for _, anchor := range [...]int{0, 8, 2, 6, 1, 7, 3, 5, 4} {
		if !occupied[anchor] {
			return anchor
		}
	}
	return 0
}
