package app

import (
	"maps"
	"slices"

	"imageflow/internal/config"
	"imageflow/internal/files"
	"imageflow/internal/imaging"
)

func (m *Model) focusSettings() files.PhotoSettings {
	return files.PhotoSettings{Image: m.Image, Naming: m.Naming, Preset: m.AppliedPreset}
}

func (m *Model) PhotoSettings(s files.Source) files.PhotoSettings {
	if s.Settings != nil {
		return *s.Settings
	}
	return m.defaults
}

func (m *Model) loadFocus() {
	p := m.PhotoSettings(m.SelectedSource())
	m.Image, m.Naming, m.AppliedPreset = p.Image, p.Naming, m.presetID(p)
}

func (m *Model) presetID(p files.PhotoSettings) string {
	if p.Preset != "" {
		if preset, ok := m.Preferences.Preset(p.Preset); ok && p.Image == presetImage(preset.Image) && p.Naming == preset.Naming {
			return p.Preset
		}
	}
	return ""
}

func (m *Model) SelectionHasMixedPresets() bool {
	if len(m.EditSelection) < 2 {
		return false
	}
	if m.presetRevision != m.ScopeRevision {
		ref := m.SelectedSource().Settings
		m.mixedPresets = slices.ContainsFunc(m.Sources, func(s files.Source) bool {
			return m.EditSelection[s.Path] && s.Settings != ref && m.presetID(m.PhotoSettings(s)) != m.AppliedPreset
		})
		m.presetRevision = m.ScopeRevision
	}
	return m.mixedPresets
}

// Saving a preset from the preview must preserve other photos' individual settings.
func (m *Model) MarkSavedPreset(p config.Preset) {
	image := presetImage(p.Image)
	m.editPhotos(m.EditSelection, HistoryPreset, 0, func(photo files.PhotoSettings) files.PhotoSettings {
		if photo.Image == image && photo.Naming == p.Naming {
			photo.Preset = p.ID
		}
		return photo
	})
}

func (m *Model) ClearSelectedPreset() {
	m.editPhotos(m.EditSelection, HistoryPreset, 0, func(photo files.PhotoSettings) files.PhotoSettings {
		photo.Preset = ""
		return photo
	})
}

func (m *Model) Select(path string) { m.SetEditSelection(map[string]bool{path: true}, path) }

func (m *Model) SelectAll() {
	paths := make(map[string]bool, len(m.Sources))
	for _, s := range m.Sources {
		paths[s.Path] = true
	}
	m.SetEditSelection(paths, m.Selected)
}

// Changing membership retains the focused preview whenever it is still selected.
func (m *Model) SetEditSelection(paths map[string]bool, focus string) {
	if m.Working || m.Collage.Exporting || len(m.Sources) == 0 {
		return
	}
	selected := make(map[string]bool, len(paths))
	first := ""
	for _, s := range m.Sources {
		if paths[s.Path] {
			selected[s.Path] = true
			if first == "" {
				first = s.Path
			}
		}
	}
	if first == "" {
		if m.sourceIndexFor(focus) < 0 {
			focus = m.Sources[0].Path
		}
		selected[focus] = true
	} else if !selected[focus] {
		focus = first
	}
	if maps.Equal(selected, m.EditSelection) && focus == m.Selected {
		return
	}
	m.FinishHistoryEdit()
	m.EditSelection = selected
	m.ScopeRevision++
	if m.Selected != focus {
		m.Selected, m.PreviewPart = focus, 1
		m.loadFocus()
		m.pruneLogoPreviews()
		m.requestPreview()
		if m.HasCollage() {
			m.requestCollageLayers()
		}
	}
}

func (m *Model) ToggleSelected(path string) {
	paths := maps.Clone(m.EditSelection)
	if paths[path] {
		delete(paths, path)
	} else {
		paths[path] = true
	}
	m.SetEditSelection(paths, m.Selected)
}

// editPhotos replaces records, never mutating settings that a pending plan or
// retry is still reading. Identical input settings share one result per edit.
func (m *Model) editPhotos(paths map[string]bool, action HistoryAction, group HistoryGroup, edit func(files.PhotoSettings) files.PhotoSettings) bool {
	if m.Working || m.Collage.Exporting {
		return false
	}
	if len(m.Sources) == 0 {
		if len(paths) != 0 {
			return false
		}
		before := m.focusSettings()
		after := edit(before)
		if before == after {
			return false
		}
		set := func(m *Model, p files.PhotoSettings) {
			m.defaults = p
			m.Image, m.Naming, m.AppliedPreset = p.Image, p.Naming, p.Preset
		}
		recordValue(m, action, group, before, after, set)
		set(m, after)
		return true
	}
	resolved := make(map[*files.PhotoSettings]*files.PhotoSettings)
	var edits []sourceEdit[*files.PhotoSettings]
	for i := range m.Sources {
		s := &m.Sources[i]
		if !paths[s.Path] && !(len(paths) == 0 && s.Path == m.Selected) {
			continue
		}
		old := s.Settings
		next, ok := resolved[old]
		if !ok {
			before := m.PhotoSettings(*s)
			after := edit(before)
			next = old
			if after != before {
				next = &after
			}
			resolved[old] = next
		}
		if old != next {
			edits = append(edits, sourceEdit[*files.PhotoSettings]{i, old, next})
			s.Settings = next
		}
	}
	if len(edits) == 0 {
		return false
	}
	recordSources(m, action, group, edits, func(s *files.Source) **files.PhotoSettings { return &s.Settings })
	m.loadFocus()
	m.ScopeRevision++
	return true
}

// Logo layers are identified by file, independently of their slot on each photo.
func patchLogos(o *imaging.Options, before, after imaging.Options) {
	if before.Logo == after.Logo && before.AdditionalLogos == after.AdditionalLogos && before.LogoOnTop == after.LogoOnTop && before.LogosBelowText == after.LogosBelowText {
		return
	}
	old, next, marks := before.LogoMarks(), after.LogoMarks(), o.LogoMarks()
	if len(old) == 0 && len(next) == 0 {
		if len(marks) == 0 {
			o.Logo = patchLogo(o.Logo, before.Logo, after.Logo)
		}
		return
	}
	if o.Logo == before.Logo && o.AdditionalLogos == before.AdditionalLogos && o.LogoOnTop == before.LogoOnTop && o.LogosBelowText == before.LogosBelowText {
		o.Logo, o.AdditionalLogos, o.LogoOnTop, o.LogosBelowText = after.Logo, after.AdditionalLogos, after.LogoOnTop, after.LogosBelowText
		return
	}
	below := o.TextLayerPosition(len(marks))
	for i := len(marks) - 1; i >= 0; i-- {
		mark := &marks[i]
		was, now := before.LogoByPath(mark.Path), after.LogoByPath(mark.Path)
		if was == nil {
			continue
		}
		if now == nil {
			marks = slices.Delete(marks, i, i+1)
			if i < below {
				below--
			}
		} else {
			*mark = patchLogo(*mark, *was, *now)
		}
	}
	for _, mark := range next {
		if before.LogoByPath(mark.Path) == nil && !slices.ContainsFunc(marks, func(l imaging.LogoMark) bool { return l.Path == mark.Path }) && len(marks) < imaging.MaxLogoLayers {
			marks = append(marks, mark)
		}
	}
	// Reorder common layers in their own slots. Other artwork keeps its order.
	if len(old) == len(next) {
		var slots []int
		var ordered []imaging.LogoMark
		for i, l := range marks {
			if before.LogoByPath(l.Path) != nil && after.LogoByPath(l.Path) != nil {
				slots = append(slots, i)
			}
		}
		for _, l := range next {
			for _, mark := range marks {
				if mark.Path == l.Path && before.LogoByPath(l.Path) != nil {
					ordered = append(ordered, mark)
					break
				}
			}
		}
		reordered := false
		for i := range old {
			if old[i].Path != next[i].Path {
				reordered = true
				break
			}
		}
		if reordered {
			for i, slot := range slots {
				marks[slot] = ordered[i]
			}
		}
	}
	if before.TextLayerPosition(len(old)) != after.TextLayerPosition(len(next)) {
		below = after.TextLayerPosition(len(next))
	}
	o.SetLogoMarks(marks)
	o.SetTextLayerPosition(min(below, len(marks)), len(marks))
}
