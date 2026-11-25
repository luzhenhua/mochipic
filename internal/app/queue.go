package app

import (
	"maps"

	"imageflow/internal/files"
)

type indexedSource struct {
	index int
	files.Source
}

type historySelection struct {
	path  string
	part  int
	paths map[string]bool
}

func (m *Model) historySelection() historySelection {
	return historySelection{m.Selected, m.PreviewPart, maps.Clone(m.EditSelection)}
}

func (m *Model) CanEditQueue() bool {
	return !m.Working && !m.Scanning && !m.Collage.Exporting
}

// Remove only changes the in-memory batch. Undo can reinsert the complete
// source records with their previous order and per-photo edits.
func (m *Model) Remove(paths map[string]bool) int {
	if !m.CanEditQueue() || len(paths) == 0 {
		return 0
	}
	before := m.historySelection()
	var removed []indexedSource
	for i, s := range m.Sources {
		if paths[s.Path] {
			removed = append(removed, indexedSource{i, s})
		}
	}
	if len(removed) == 0 {
		return 0
	}
	m.removeSources(removed)
	m.recordQueueEdit(HistoryRemove, removed, before, m.historySelection())
	if before.path != m.Selected {
		m.requestPreview()
	} else {
		m.scheduleWarmup()
	}
	m.syncCollage()
	m.RefreshPlan()
	return len(removed)
}

func contiguousSources(start int, added []files.Source) []indexedSource {
	indexed := make([]indexedSource, len(added))
	for i, s := range added {
		indexed[i] = indexedSource{start + i, s}
	}
	return indexed
}

func (m *Model) removeSources(rows []indexedSource) {
	dropped := make(map[string]bool, len(rows))
	next := 0
	kept := m.Sources[:0]
	for i, j := 0, 0; i < len(m.Sources); i++ {
		s := m.Sources[i]
		if j < len(rows) && rows[j].index == i {
			dropped[s.Path] = true
			j++
			if s.Path == m.Selected {
				next = len(kept)
			}
		} else {
			s.Ordinal = len(kept)
			kept = append(kept, s)
		}
	}
	clear(m.Sources[len(kept):])
	m.Sources = kept
	m.removeThumbnails(dropped)
	m.preview.cache.removePaths(dropped)
	for path := range dropped {
		delete(m.EditSelection, path)
		delete(m.logoTargets, path)
		delete(m.certificationTargets, path)
	}
	if dropped[m.Selected] {
		m.Selected, m.PreviewPart = "", 1
		if len(kept) > 0 {
			m.Selected = kept[min(next, len(kept)-1)].Path
			for _, s := range kept {
				if m.EditSelection[s.Path] {
					m.Selected = s.Path
					break
				}
			}
		}
	}
	if m.Selected != "" {
		m.EditSelection[m.Selected] = true
	}
	m.ScopeRevision++
	m.loadFocus()
}

func (m *Model) insertSources(rows []indexedSource) {
	restored := make([]files.Source, len(m.Sources)+len(rows))
	for at, i, j := 0, 0, 0; at < len(restored); at++ {
		if j < len(rows) && rows[j].index == at {
			restored[at] = rows[j].Source
			j++
		} else {
			restored[at] = m.Sources[i]
			i++
		}
		restored[at].Ordinal = at
	}
	m.Sources = restored
}

func (m *Model) recordQueueEdit(action HistoryAction, rows []indexedSource, before, after historySelection) {
	s := historyStep{action: action, cost: len(rows) * 2}
	for _, row := range rows {
		s.bytes += len(row.Path) + len(row.RelativeDir) + len(row.Selection.Range) + len(row.Caption.Text)
	}
	if action == HistoryRemove {
		s.removed = len(rows)
	}
	s.patch = historyReplay(func(m *Model, forward bool) {
		from, to := before, after
		if !forward {
			from, to = to, from
		}
		keep := m.historySelection()
		if (action == HistoryImport) == forward {
			m.insertSources(rows)
		} else {
			m.removeSources(rows)
		}
		// Preserve a photo or frame explicitly chosen after the queue edit.
		if keep.path == from.path && keep.part == from.part && maps.Equal(keep.paths, from.paths) && (to.path == "" && len(m.Sources) == 0 || m.sourceIndexFor(to.path) >= 0) {
			m.Selected, m.PreviewPart = to.path, to.part
			m.EditSelection = maps.Clone(to.paths)
		} else if m.Selected == "" && len(m.Sources) > 0 {
			m.Selected, m.PreviewPart = m.Sources[0].Path, 1
			m.EditSelection[m.Selected] = true
		}
	})
	m.recordHistory(s)
}

func (m *Model) LastRemovedCount() int { return m.history.undoStep().removed }
