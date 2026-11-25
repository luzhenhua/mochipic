package app

import (
	"slices"

	"imageflow/internal/caption"
	"imageflow/internal/files"
	"imageflow/internal/series"
)

type HistoryAction string

const (
	HistoryImport        HistoryAction = "history.import"
	HistoryRemove        HistoryAction = "history.remove"
	HistoryCaption       HistoryAction = "history.caption"
	HistoryCaptionBatch  HistoryAction = "history.captionBatch"
	HistoryText          HistoryAction = "history.text"
	HistoryLogo          HistoryAction = "history.logo"
	HistoryLogoImport    HistoryAction = "history.logoImport"
	HistoryLayers        HistoryAction = "history.layers"
	HistoryCertification HistoryAction = "history.certification"
	HistoryCertScope     HistoryAction = "history.certScope"
	HistoryCertCopy      HistoryAction = "history.certCopy"
	HistoryCrop          HistoryAction = "history.crop"
	HistoryResize        HistoryAction = "history.resize"
	HistoryFormat        HistoryAction = "history.format"
	HistorySeries        HistoryAction = "history.series"
	HistoryRename        HistoryAction = "history.rename"
	HistoryOutput        HistoryAction = "history.output"
	HistoryPreset        HistoryAction = "history.preset"
	HistoryCollageCreate HistoryAction = "history.collageCreate"
	HistoryCollageOrder  HistoryAction = "history.collageOrder"
	HistoryCollageLayout HistoryAction = "history.collageLayout"
)

// HistoryGroup joins the live edits of one control gesture or text focus
// session, leaving preview updates independent of the committed history.
type HistoryGroup uint64

type historyStep struct {
	action  HistoryAction
	group   HistoryGroup
	cost    int
	bytes   int
	removed int
	patch   historyPatch
}

type historyPatch interface {
	apply(*Model, bool)
	empty() bool
}

type historyValue[T comparable] struct {
	before, after T
	set           func(*Model, T)
}

func (v *historyValue[T]) apply(m *Model, forward bool) {
	value := v.before
	if forward {
		value = v.after
	}
	v.set(m, value)
}
func (v *historyValue[T]) empty() bool { return v.before == v.after }

type historyReplay func(*Model, bool)

func (r historyReplay) apply(m *Model, forward bool) { r(m, forward) }
func (r historyReplay) empty() bool                  { return false }

// Keep both the number of steps and the source records they hold bounded even
// when repeatedly editing batches of 10,000 photos. No decoded pixels are kept.
const (
	historyLimit      = 100
	historyCost       = 50_000
	historyTextBudget = 16 << 20
)

type editHistory struct {
	steps  []historyStep
	cursor int
	cost   int
	bytes  int
	draft  historyStep
	next   HistoryGroup
}

func (h *editHistory) push(s historyStep) {
	if s.action == "" || s.patch.empty() {
		return
	}
	for _, step := range h.steps[h.cursor:] {
		h.cost -= step.cost
		h.bytes -= step.bytes
	}
	clear(h.steps[h.cursor:])
	h.steps = append(h.steps[:h.cursor], s)
	h.cost += s.cost
	h.bytes += s.bytes
	drop := 0
	for len(h.steps)-drop > 1 && (len(h.steps)-drop > historyLimit || h.cost > historyCost || h.bytes > historyTextBudget) {
		h.cost -= h.steps[drop].cost
		h.bytes -= h.steps[drop].bytes
		drop++
	}
	h.steps = slices.Delete(h.steps, 0, drop)
	h.cursor = len(h.steps)
}

func (h *editHistory) finish() {
	s := h.draft
	h.draft = historyStep{}
	h.push(s)
}

func (h *editHistory) undoStep() historyStep {
	if s := h.draft; s.action != "" && !s.patch.empty() {
		return s
	}
	if h.cursor > 0 {
		return h.steps[h.cursor-1]
	}
	return historyStep{}
}

func (m *Model) recordHistory(s historyStep) {
	m.history.finish()
	if s.group == 0 {
		m.history.push(s)
	} else {
		m.history.draft = s
	}
}

func recordValue[T comparable](m *Model, action HistoryAction, group HistoryGroup, before, after T, set func(*Model, T)) {
	if before == after {
		return
	}
	if s := m.history.draft; group != 0 && s.group == group && s.action == action {
		if v, ok := s.patch.(*historyValue[T]); ok && v.after == before {
			v.after = after
			m.history.draft.bytes = historyValueBytes(v.before) + historyValueBytes(after)
			return
		}
	}
	m.recordHistory(historyStep{action: action, group: group, cost: 1, bytes: historyValueBytes(before) + historyValueBytes(after), patch: &historyValue[T]{before, after, set}})
}

// Source indices stay valid as later queue edits are undone before this step.
// A bulk edit stores only the values on the affected images.
type sourceEdit[T comparable] struct {
	index         int
	before, after T
}

type sourcePatch[T comparable] struct {
	edits []sourceEdit[T]
	field func(*files.Source) *T
}

func (p *sourcePatch[T]) apply(m *Model, forward bool) {
	for _, e := range p.edits {
		v := e.before
		if forward {
			v = e.after
		}
		*p.field(&m.Sources[e.index]) = v
	}
}
func (p *sourcePatch[T]) empty() bool {
	return !slices.ContainsFunc(p.edits, func(e sourceEdit[T]) bool { return !sameHistoryValue(e.before, e.after) })
}
func sameHistoryValue[T comparable](a, b T) bool {
	if a == b {
		return true
	}
	if x, ok := any(a).(*files.PhotoSettings); ok {
		y := any(b).(*files.PhotoSettings)
		return x != nil && y != nil && *x == *y
	}
	return false
}
func recordSources[T comparable](m *Model, action HistoryAction, group HistoryGroup, edits []sourceEdit[T], field func(*files.Source) *T) {
	if len(edits) == 0 {
		return
	}
	draft := &m.history.draft
	if group != 0 && draft.group == group && draft.action == action {
		if p, ok := draft.patch.(*sourcePatch[T]); ok {
			// Both lists are in queue order. Keep the original before-value even if
			// a photo first starts changing halfway through this gesture.
			merged := make([]sourceEdit[T], 0, max(len(edits), len(p.edits)))
			i, j := 0, 0
			for i < len(p.edits) || j < len(edits) {
				switch {
				case j == len(edits) || i < len(p.edits) && p.edits[i].index < edits[j].index:
					merged = append(merged, p.edits[i])
					i++
				case i == len(p.edits) || edits[j].index < p.edits[i].index:
					merged = append(merged, edits[j])
					j++
				default:
					e := p.edits[i]
					e.after = edits[j].after
					merged = append(merged, e)
					i++
					j++
				}
			}
			p.edits = merged
			draft.cost = len(merged)
			draft.bytes = sourcePatchBytes(merged)
			return
		}
	}
	m.recordHistory(historyStep{action: action, group: group, cost: len(edits), bytes: sourcePatchBytes(edits), patch: &sourcePatch[T]{edits, field}})
}
func sourcePatchBytes[T comparable](edits []sourceEdit[T]) int {
	total := 0
	for _, e := range edits {
		total += historyValueBytes(e.before) + historyValueBytes(e.after)
	}
	return total
}

func historyValueBytes[T comparable](v T) int {
	switch value := any(v).(type) {
	case caption.Caption:
		return len(value.Text)
	case *files.PhotoSettings:
		if value == nil {
			return 0
		}
		return historyValueBytes(*value)
	case files.PhotoSettings:
		n := len(value.Image.Text.Text) + len(value.Naming.Prefix) + len(value.Naming.Suffix) + len(value.Naming.Template) + len(value.Image.Logo.Path)
		for _, mark := range value.Image.AdditionalLogos {
			n += len(mark.Path)
		}
		return n
	case series.Selection:
		return len(value.Range)
	default:
		return 0
	}
}

func (m *Model) NewHistoryGroup() HistoryGroup {
	m.history.next++
	return m.history.next
}

func (m *Model) FinishHistoryGroup(group HistoryGroup) {
	if group != 0 && m.history.draft.group == group {
		m.history.finish()
	}
}

func (m *Model) FinishHistoryEdit() { m.history.finish() }

func (m *Model) CancelHistoryGroup(group HistoryGroup) {
	h := &m.history
	if group != 0 && h.draft.group == group {
		s := h.draft
		h.draft = historyStep{}
		m.applyHistoryStep(s, false)
		m.changed()
	}
}

func (m *Model) CanReplayHistory() bool {
	return m.CanEditQueue() && !m.LogoPending && !m.CertificationImporting
}

func (m *Model) UndoAction() HistoryAction { return m.history.undoStep().action }

func (m *Model) RedoAction() HistoryAction {
	h := &m.history
	if h.draft.action != "" && !h.draft.patch.empty() || h.cursor == len(h.steps) {
		return ""
	}
	return h.steps[h.cursor].action
}

func (m *Model) Undo() bool { return m.replayHistory(false) }
func (m *Model) Redo() bool { return m.replayHistory(true) }

func (m *Model) replayHistory(forward bool) bool {
	if !m.CanReplayHistory() {
		return false
	}
	h := &m.history
	h.finish()
	index := h.cursor
	if !forward {
		index--
	}
	if index < 0 || index >= len(h.steps) {
		return false
	}
	step := h.steps[index]
	m.applyHistoryStep(step, forward)
	h.cursor = index
	if forward {
		h.cursor++
	}
	return true
}

func (m *Model) applyHistoryStep(step historyStep, forward bool) {
	image, names, output := m.Image, m.Naming, m.Output
	source, part, group, selected := m.SelectedSource(), m.PreviewPart, m.Certification(), m.Selected
	step.patch.apply(m, forward)
	m.ScopeRevision++
	m.loadFocus()
	current := m.SelectedSource()
	queueChanged := step.action == HistoryImport || step.action == HistoryRemove
	if m.Image != image {
		m.pruneLogoPreviews()
		if len(m.Collage.Items) > 0 {
			m.requestCollageLayers()
		}
	}
	if m.Image != image || m.Selected != selected || m.PreviewPart != part || current.Crop != source.Crop ||
		current.Selection != source.Selection || current.Caption != source.Caption || m.Certification() != group {
		m.requestPreview()
	} else if queueChanged {
		m.scheduleWarmup()
	}
	if queueChanged || step.action == HistoryCrop {
		m.syncCollage()
	}
	switch {
	case queueChanged || m.Output.Directory != output.Directory:
		m.RefreshPlan()
	case m.Image.Format != image.Format || m.Naming != names || m.Output != output || step.action == HistorySeries || step.action == HistoryFormat || step.action == HistoryRename || step.action == HistoryPreset:
		m.schedulePlan(true)
	}
}
