package app

import (
	"maps"
	"path/filepath"
	"slices"

	"imageflow/internal/certification"
	"imageflow/internal/files"
	"imageflow/internal/imaging"
	"imageflow/internal/l10n"
)

type certificationSession struct {
	badges   []*certification.Badge
	previews map[*certification.Badge]imaging.CertificationPreview
	loaded   bool
}

func (m *Model) certificationLibrary() certification.Library {
	return certification.Library(filepath.Dir(m.configPath))
}

func (m *Model) LoadCertifications() {
	if m.certification.loaded || m.CertificationLoading {
		return
	}
	m.CertificationLoading = true
	library, hasConfig := m.certificationLibrary(), m.configPath != ""
	m.wg.Go(func() {
		s := certificationSession{loaded: true, previews: make(map[*certification.Badge]imaging.CertificationPreview)}
		var libraryErr error
		add := func(b *certification.Badge) {
			preview, err := imaging.RenderCertificationBadge(m.ctx, b)
			if err != nil {
				libraryErr = l10n.Wrap("error.certificationLibraryRead", err)
				return
			}
			s.badges = append(s.badges, b)
			s.previews[b] = preview
		}
		for _, b := range certification.Builtins() {
			add(b)
		}
		if hasConfig && m.ctx.Err() == nil {
			custom, err := library.List()
			if err != nil {
				libraryErr = l10n.Wrap("error.certificationLibraryRead", err)
			}
			for _, b := range custom {
				add(b)
			}
		}
		m.onUI(func() {
			m.certification, m.CertificationLoading, m.CertificationError = s, false, libraryErr
		})
	})
}

func (m *Model) CertificationBadges() []*certification.Badge { return m.certification.badges }
func (m *Model) CertificationPreview(b *certification.Badge) imaging.CertificationPreview {
	return m.certification.previews[b]
}

func (m *Model) CanImportCertification() bool {
	return m.certification.loaded && !m.CertificationImporting && !m.Working && !m.Collage.Exporting && m.configPath != "" && m.Selected != ""
}

func (m *Model) ImportCertification(path string) {
	if !m.CanImportCertification() {
		return
	}
	m.CertificationImporting = true
	paths, library := maps.Clone(m.EditSelection), m.certificationLibrary()
	m.certificationTargets = paths
	m.wg.Go(func() {
		data, preview, err := imaging.PrepareCertificationImport(m.ctx, path)
		var badge *certification.Badge
		if err == nil {
			err = m.ctx.Err()
		}
		if err == nil {
			badge, err = library.Save(path, data)
		}
		m.onUI(func() {
			m.CertificationImporting, m.certificationTargets = false, nil
			if err != nil {
				m.Error = l10n.Wrap("error.certificationImport", err)
				return
			}
			s := &m.certification
			if i := slices.IndexFunc(s.badges, func(b *certification.Badge) bool { return b.LocalPath() == badge.LocalPath() }); i >= 0 {
				badge = s.badges[i]
				s.previews[badge] = preview
			} else {
				s.badges = append(s.badges, badge)
				s.previews[badge] = preview
			}
			m.editBadges(paths, 0, func(g *certification.Group) bool { return g.Has(badge) || g.Toggle(badge) })
		})
	})
}

func certificationGroup(g certification.Group) certification.Group {
	if g == (certification.Group{}) {
		return certification.Defaults()
	}
	return g
}

func (m *Model) Certification() certification.Group {
	return m.certificationFor(m.SelectedSource())
}

func (m *Model) certificationFor(s files.Source) certification.Group {
	return certificationGroup(s.Certification)
}
func (m *Model) SetCertification(g certification.Group) { m.EditCertification(g, 0) }
func (m *Model) EditCertification(g certification.Group, group HistoryGroup) {
	m.EditCertificationFrom(m.Certification(), g, group)
}
func (m *Model) EditCertificationFrom(before, g certification.Group, group HistoryGroup) {
	if before == g || !g.Valid(imaging.MaxSide) {
		return
	}
	m.editBadges(m.EditSelection, group, func(dst *certification.Group) bool {
		applyField(&dst.Size, before.Size, g.Size)
		applyField(&dst.Gap, before.Gap, g.Gap)
		dst.Placement = patchPlacement(dst.Placement, before.Placement, g.Placement)
		for i := 0; i < before.Count(); i++ {
			b := before.Badge(i)
			if !g.Has(b) && dst.Has(b) {
				dst.Toggle(b)
			}
		}
		for i := 0; i < g.Count(); i++ {
			b := g.Badge(i)
			if !before.Has(b) && !dst.Has(b) && !dst.Toggle(b) {
				return false
			}
		}
		return true
	})
}
func (m *Model) SetBadge(b *certification.Badge, enabled bool) {
	m.editBadges(m.EditSelection, 0, func(g *certification.Group) bool { return g.Has(b) == enabled || g.Toggle(b) })
}
func (m *Model) editBadges(paths map[string]bool, group HistoryGroup, edit func(*certification.Group) bool) {
	if m.Working || m.Collage.Exporting {
		return
	}
	var edits []sourceEdit[certification.Group]
	for i, s := range m.Sources {
		if !paths[s.Path] {
			continue
		}
		before := m.certificationFor(s)
		next := before
		if !edit(&next) {
			m.Error = l10n.Error("error.certificationBadgeLimit", l10n.Args{"count": certification.MaxBadges})
			return
		}
		if before != next {
			edits = append(edits, sourceEdit[certification.Group]{i, s.Certification, next})
		}
	}
	if len(edits) == 0 {
		return
	}
	for _, e := range edits {
		m.Sources[e.index].Certification = e.after
	}
	recordSources(m, HistoryCertification, group, edits, func(s *files.Source) *certification.Group { return &s.Certification })
	m.ScopeRevision++
	m.requestProof()
	m.updatePreviewStatus()
}
