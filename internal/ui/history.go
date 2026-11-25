package ui

import (
	"imageflow/internal/app"
	"imageflow/internal/l10n"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/overlay"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

// Only continuous controls join a history group. Other controls commit a
// step immediately, even if they edit the same setting twice in quick succession.
type historyControls struct {
	*desktop
	group  app.HistoryGroup
	end    []app.HistoryGroup
	cancel []app.HistoryGroup
}

func (d *desktop) editControls() historyControls {
	return historyControls{desktop: d}
}

type controlHistory struct {
	model *app.Model
	id    app.HistoryGroup
}

func (h *controlHistory) Close() error {
	h.model.FinishHistoryGroup(h.id)
	return nil
}

func (h *historyControls) track(e ui.Element, active, changed bool) {
	if h == nil {
		return
	}
	g := ui.Local(e, "history-group", func() controlHistory { return controlHistory{model: h.model} })
	if active && g.id == 0 {
		g.id = h.model.NewHistoryGroup()
	}
	if changed {
		h.group = g.id
	}
	if !active && g.id != 0 {
		if e.PressCanceled() {
			h.cancel = append(h.cancel, g.id)
		} else {
			h.end = append(h.end, g.id)
		}
		g.id = 0
	}
}

func (h *historyControls) trackColor(e ui.Element, changed bool) {
	state := ui.Local(e, "color-history", func() controlHistory { return controlHistory{model: h.model} })
	if changed {
		if state.id == 0 {
			state.id = h.model.NewHistoryGroup()
		}
		h.group = state.id
	}
	// Treat a color choice as a step, including hue and saturation edits
	// inside Lobe's attached popover.
	if !changed && (e.Pressed() || e.PressedOutside() || e.FocusLeft()) {
		h.end = append(h.end, state.id)
		state.id = 0
	}
}

func (h *historyControls) finish() {
	for _, group := range h.cancel {
		h.model.CancelHistoryGroup(group)
	}
	for _, group := range h.end {
		h.model.FinishHistoryGroup(group)
	}
}

func (d *desktop) historyReady() bool {
	return !d.dialog && !d.presetModal && !d.creditsOpen && d.model.Error == nil && !d.strip.drag.running() &&
		d.model.CanReplayHistory()
}

func historyLabel(locale string, next app.HistoryAction, redo bool) string {
	if next == "" {
		return l10n.T(locale, choose(redo, "mygo.menuRedo", "mygo.menuUndo"))
	}
	return l10n.Format(locale, choose(redo, "history.redo", "history.undo"), l10n.Args{"action": l10n.T(locale, string(next))})
}

func (d *desktop) syncHistoryMenu(c *ui.Context) {
	if d.historyMenuUndo == nil {
		return
	}
	for _, item := range []struct {
		menu *mygo.MenuItem
		next app.HistoryAction
		redo bool
	}{{d.historyMenuUndo, d.model.UndoAction(), false}, {d.historyMenuRedo, d.model.RedoAction(), true}} {
		next := item.next
		if c.FocusTakesText() || !d.historyReady() {
			next = ""
		}
		label := historyLabel(d.locale(), next, item.redo)
		if label != item.menu.Label {
			item.menu.SetLabel(label)
		}
	}
}

func (d *desktop) historyShortcuts(c *ui.Context) {
	// Keep MyGo's native Edit menu roles. In a text field they are handled by
	// the editor; elsewhere MyGo routes them to these window shortcuts.
	if !d.historyReady() || c.FocusTakesText() {
		return
	}
	if d.model.UndoAction() != "" && c.Shortcut(ui.Cmd, ui.KeyZ) && !c.ShortcutRepeated() {
		d.replayHistory(false)
	}
	if d.model.RedoAction() != "" && c.Shortcut(ui.Cmd|ui.Shift, ui.KeyZ) && !c.ShortcutRepeated() {
		d.replayHistory(true)
	}
}

func (d *desktop) historyButtons(c *ui.Context) {
	m, tk := d.model, theme.TokensOf(c)
	for _, item := range []struct {
		redo            bool
		icon            *ui.SVG
		label, shortcut string
		next            app.HistoryAction
	}{
		{false, lucide.Undo2(), "mygo.menuUndo", "mod+z", m.UndoAction()},
		{true, lucide.Redo2(), "mygo.menuRedo", "mod+shift+z", m.RedoAction()},
	} {
		ctx := c.Key(item.label)
		label := tr(c, item.label)
		tip := historyLabel(c.Locale(), item.next, item.redo)
		disabled := item.next == "" || !d.historyReady()
		b := action.StyleActionIcon(ctx, ui.ButtonBase(ctx).Label(label).Disabled(disabled), item.icon,
			action.ActionIconProps{BlockSize: 28, GlyphSize: 16})
		b.TextColor(choose(disabled, tk.TextQuaternary, tk.TextSecondary))
		overlay.Tooltip(ctx, b, tip, overlay.TooltipProps{Placement: overlay.Bottom, Hotkey: item.shortcut})
		if b.Clicked() {
			d.replayHistory(item.redo)
		}
	}
}

func (d *desktop) replayHistory(redo bool) {
	if !d.historyReady() {
		return
	}
	m := d.model

	before := m.SelectedSource()
	run := m.Undo
	if redo {
		run = m.Redo
	}
	if !run() {
		return
	}

	d.strip.notice = false
	d.listAnchor, d.seriesPrompt = "", ""
	if d.cropping() && m.SelectedSource().Crop != before.Crop {
		d.crop.path = ""
	}
	if m.Selected != before.Path {
		d.showSelectedThumb()
	}
	if d.page == collage && !m.HasCollage() {
		d.leaveCollage()
	}
	d.redraw()
}
