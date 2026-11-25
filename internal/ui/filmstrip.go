package ui

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"

	"imageflow/internal/l10n"
)

type filmstripState struct {
	notice        bool
	tiles, footer ui.Rect
	drag          thumbnailDrag
}

type thumbnailDrag struct {
	path             string
	x, y, startY     float32
	zone             ui.Rect
	moved, cancelled bool
}

func (g thumbnailDrag) running() bool { return g.moved && !g.cancelled }

func (s filmstripState) dropZone() ui.Rect {
	y := s.tiles.Y + s.tiles.H + 3
	return ui.Rect{X: s.tiles.X, Y: y, W: s.tiles.W, H: max(0, s.footer.Y+s.footer.H-y)}
}

func (s filmstripState) overDropZone() bool {
	g := s.drag
	return g.running() && g.zone == s.dropZone() && g.zone.Contains(g.x, g.y)
}

func (d *desktop) filmstrip(c *ui.Context) {
	m, tk := d.model, theme.TokensOf(c)
	items, per := m.Sources, d.thumbSlots()
	start, end := pageRange(len(items), per, &d.thumbPage)
	cropStrip := d.cropStrip()
	height := choose(cropStrip, float32(65), 68)
	visible := pathsOf(items[start:end])
	m.ShowThumbnails(visible)
	picking := d.page == pictures
	var remove string
	strip := ui.Row(c).Height(height).Shrink(0).Gap(9).AlignItems(ui.Center)
	if picking {
		d.strip.tiles = strip.Bounds()
	}
	strip.Children(func() {
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(8).AlignItems(ui.Stretch).Children(func() {
			for _, s := range items[start:end] {
				name := filepath.Base(s.Path)
				btn := ui.ButtonBase(c.Key(s.Path)).Column().MinWidth(0).Height(height-1).Padding(2).
					Radius(tk.BorderRadius).Border(1, ui.Transparent).Clip().Label(name).Tooltip(name)
				if cropStrip {
					btn.Width(110).Shrink(0)
				} else {
					btn.Grow(1).Basis(0)
				}
				on := m.EditSelection[s.Path]
				if on {
					btn.Border(1, tk.TextSecondary).AccessSelected(true)
				}
				if m.Selected == s.Path || btn.Hovered() {
					btn.Border(1, tk.Primary)
				}
				btn.Children(func() {
					thumbH := height - 7
					if !picking {
						thumbH -= 16
					}
					d.thumbFit(c, s.Path, 0, thumbH, !cropStrip)
					if !picking {
						ui.Text(c, strings.TrimSuffix(name, filepath.Ext(name))).FontSize(10).FontWeight(choose(s.Path == m.Selected, 600, 400)).TextColor(tk.TextSecondary).AlignSelf(ui.Center).MaxWidthPercent(100).SingleLine().IgnoreInput(true)
					}
					if on {
						badge := ui.Box(c).Absolute().Right(4).Top(4).Size(15, 15).
							Radius(8).Center().Background(tk.Primary).Border(1, tk.BgContainer).IgnoreInput(true)
						badge.Children(func() { icon(c, lucide.Check(), 10, tk.BgContainer) })
					}
				})
				if btn.Shortcut(ui.Cmd, ui.KeyA) && !c.FocusTakesText() {
					m.SelectAll()
					c.Invalidate()
				}
				if !picking {
					if btn.Clicked() {
						d.pickThumbnail(s.Path, btn.ClickModifiers())
					}
					continue
				}
				dropped, dragged := d.trackThumbnail(btn, s.Path)
				if dropped || m.CanEditQueue() && !d.strip.drag.running() &&
					(btn.Shortcut(0, ui.KeyBackspace) || btn.Shortcut(0, ui.KeyDelete)) && !c.ShortcutRepeated() {
					remove = s.Path
				} else if !dragged && btn.Clicked() {
					d.pickThumbnail(s.Path, btn.ClickModifiers())
				}
			}
			if !cropStrip {
				for range per - (end - start) {
					ui.Box(c).Grow(1).Basis(0).MinWidth(0)
				}
			}
		})
		if !cropStrip || len(items) > per {
			arrows(c, len(items), per, &d.thumbPage)
		}
		if picking && d.strip.drag.running() {
			d.thumbnailGhost(c)
		}
	})
	if remove != "" {
		d.removeQueueSources(d.thumbnailTargets(remove))
	}
}

func (d *desktop) pickThumbnail(path string, mods ui.Modifiers) {
	d.strip.notice = false
	if mods&ui.Cmd == 0 {
		d.model.Select(path)
	} else if d.model.CanEditQueue() {
		d.model.ToggleSelected(path)
	}
	d.redraw()
}

func (d *desktop) thumbnailTargets(path string) map[string]bool {
	if d.model.EditSelection[path] {
		return d.model.EditSelection
	}
	return map[string]bool{path: true}
}

func (d *desktop) thumbnailTargetCount(path string) int {
	if d.model.EditSelection[path] {
		return len(d.model.EditSelection)
	}
	return 1
}

func pointerInWindow(btn ui.Element) (float32, float32) {
	x, y, _ := btn.PointerPosition()
	b := btn.Bounds()
	return b.X + x, b.Y + y
}

// Only DragEnded reports a real primary-button release; losing focus or
// pointer capture never commits a removal. The ghost does not take input.
func (d *desktop) trackThumbnail(btn ui.Element, path string) (drop, suppressClick bool) {
	g := &d.strip.drag
	_, dy, held := btn.Dragged()
	if !held {
		if g.path != path {
			return false, false
		}
		_, _, released := btn.DragEnded()
		x, y := pointerInWindow(btn)
		drop := released && g.running() && d.model.CanEditQueue() &&
			g.zone == d.strip.dropZone() && g.zone.Contains(x, y)
		dragged := g.moved || g.cancelled
		*g = thumbnailDrag{}
		return drop, dragged
	}
	x, y := pointerInWindow(btn)
	zone := d.strip.dropZone()
	if g.path != path {
		*g = thumbnailDrag{path: path, startY: y - dy, zone: zone}
	}
	g.x, g.y = x, y
	if !d.model.CanEditQueue() || g.zone != zone {
		g.cancelled = true
	}
	g.moved = g.moved || y-g.startY >= 8
	return false, g.moved || g.cancelled
}

func (d *desktop) thumbnailGhost(c *ui.Context) {
	tk, g, strip := theme.TokensOf(c), d.strip.drag, d.strip.tiles
	const w, h float32 = 66, 46
	x := max(0, min(strip.W-w-4, g.x-strip.X-w/2))
	y := min(g.y-strip.Y-h*0.7, strip.H+7-h)
	n := d.thumbnailTargetCount(g.path)
	layer := ui.Box(c.Key("drag-ghost")).Absolute().Left(x).Top(y).Size(w, h).IgnoreInput(true).ZIndex(2)
	layer.Children(func() {
		if n > 1 {
			ui.Box(c).Absolute().Left(3).Top(4).Size(w, h).Radius(7).
				Background(tk.FillTertiary).Border(1, tk.BgContainer)
		}
		ui.Box(c).Absolute().Left(0).Top(0).Size(w, h).Padding(2).Radius(7).Clip().
			Background(tk.BgContainer).Border(1, tk.BorderSecondary).
			Children(func() { d.thumb(c.Key("drag-preview"), g.path, w-4, h-4) })
		if n > 1 {
			ui.Box(c).Absolute().Top(-5).Right(-5).MinWidth(17).Height(17).PaddingX(4).Center().
				Background(tk.Primary).Border(1, tk.BgContainer).Radius(9).Children(func() {
				ui.Text(c, strconv.Itoa(n)).FontSize(10).FontWeight(600).TextColor(tk.BgContainer).SingleLine()
			})
		}
	})
}

func (d *desktop) filmstripHint(c *ui.Context) {
	tk := theme.TokensOf(c)
	ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(17, 17).Center().Radius(4).Background(tk.FillQuaternary).Children(func() {
			ui.Text(c, shortcutModifier()).FontSize(11).TextColor(tk.TextSecondary)
		})
		muted(c, tr(c, "filmstrip.multiSelect")).MinWidth(0).Shrink(1)
	})
}

func (d *desktop) removeQueueSources(paths map[string]bool) {
	before := d.model.Selected
	if d.model.Remove(paths) == 0 {
		return
	}
	d.strip.notice = true
	d.strip.drag = thumbnailDrag{}
	d.listAnchor = ""
	if d.emptyBatch() {
		d.list, d.listQuery, d.importAttempted, d.importMenu = false, "", false, false
		d.navigate(importHome)
		return
	}
	if d.list || d.model.Selected != before {
		d.showSelectedThumb()
	}
	d.redraw()
}

func (d *desktop) selectedImageCount() int {
	return len(d.model.EditSelection)
}

func (d *desktop) pictureStatusbar(c *ui.Context, inspectorWidth float32) {
	tk := theme.TokensOf(c)
	ui.Row(c).Height(statusBarHeight).Shrink(0).AlignItems(ui.Center).Children(func() {
		center := ui.Row(c).Grow(1).Basis(0).MinWidth(0).AlignSelf(ui.Stretch).PaddingX(14).AlignItems(ui.Center)
		d.strip.footer = center.Bounds()
		center.Children(func() {
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(6).AlignItems(ui.Center).Children(func() {
				d.imageTotal(c)
				if n := d.selectedImageCount(); n > 0 {
					muted(c, trf(c, "image.selectedCount", l10n.Args{"count": n})).MinWidth(0).Shrink(1)
				}
				if len(d.model.EditSelection) > 1 && !d.strip.drag.running() {
					d.clearThumbnailSelectionButton(c)
				}
				d.scanStatus(c)
			})
			d.pictureQueueAction(c)
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(6).AlignItems(ui.Center).Justify(ui.End).Children(func() {
				if inspectorWidth == 0 {
					originalsProtected(c)
				}
			})
			if d.strip.overDropZone() {
				ui.Box(c).Absolute().Left(24).Right(24).Top(-1).Height(2).Radius(1).
					Background(tk.ErrorBorder).IgnoreInput(true)
			}
		})
		if inspectorWidth > 0 {
			ui.Row(c).Width(inspectorWidth + 1).Shrink(0).PaddingX(14).Gap(8).
				AlignItems(ui.Center).Justify(ui.End).Children(func() { originalsProtected(c) })
		}
	})
}

func (d *desktop) clearThumbnailSelectionButton(c *ui.Context) {
	tk := theme.TokensOf(c)
	label := tr(c, "scope.onlyCurrent")
	b := ui.ButtonBase(c.Key("clear-selection")).Size(16, 16).Shrink(0).Radius(8).Center().
		Label(label).Tooltip(label).Cursor(ui.CursorPointer)
	b.Background(choose(b.Hovered(), tk.FillSecondary, tk.FillQuaternary))
	b.Children(func() { icon(c, lucide.X(), 10, tk.TextSecondary) })
	if b.Clicked() {
		d.model.Select(d.model.Selected)
		d.strip.notice = false
		d.redraw()
	}
}

func (d *desktop) pictureQueueAction(c *ui.Context) {
	tk := theme.TokensOf(c)
	switch {
	case d.strip.overDropZone():
		n := d.thumbnailTargetCount(d.strip.drag.path)
		ui.Row(c).Height(24).Shrink(0).PaddingX(12).Gap(6).AlignItems(ui.Center).
			Radius(tk.BorderRadius).Background(tk.ErrorBg).Border(1, tk.ErrorBorder).
			Children(func() {
				icon(c, lucide.Trash2(), 13, tk.ErrorText)
				ui.Text(c, trf(c, "filmstrip.dropCount", l10n.Args{"count": n})).
					FontSize(12).FontWeight(600).TextColor(tk.ErrorText).SingleLine()
			})
	case d.strip.drag.running():
		return
	case d.strip.notice && d.model.LastRemovedCount() > 0:
		d.queueUndoAction(c)
	case len(d.model.EditSelection) > 1:
		b := styledButtonSize(c.Key("remove-selected"), tr(c, "library.removeSelected"), lucide.Trash2(),
			action.Filled, theme.Small, !d.model.CanEditQueue(), false)
		if b.Clicked() {
			d.removeQueueSources(d.model.EditSelection)
		}
	}
}

func (d *desktop) queueUndoAction(c *ui.Context) {
	if n := d.model.LastRemovedCount(); n > 0 {
		ui.Row(c).Shrink(0).Gap(4).AlignItems(ui.Center).Children(func() {
			muted(c, trf(c, "filmstrip.removedCount", l10n.Args{"count": n}))
			if styledButtonSize(c.Key("undo-removal"), tr(c, "filmstrip.undo"), nil, action.Text, theme.Small,
				!d.model.CanEditQueue(), false).Tooltip(shortcutLabel("Z")).Clicked() {
				d.replayHistory(false)
			}
		})
	}
}
