package ui

import (
	"image"
	"math"

	"imageflow/internal/app"
	"imageflow/internal/crop"
	"imageflow/internal/l10n"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

type cropDraft struct {
	active    bool
	path      string
	part      int
	size      image.Point
	box       image.Rectangle
	ratio     crop.Ratio
	alignment crop.Alignment
	gesture   cropGesture
}

type cropGesture struct {
	held   bool
	handle int
	start  image.Rectangle
	dx, dy float64
}

func (d *desktop) cropping() bool { return d.page == resize && d.resizeTab == 0 && d.crop.active }

func (d *desktop) beginCrop() {
	d.hideInspector = false
	d.crop = cropDraft{active: true}
	d.showSelectedThumb()
	d.model.SetMode(app.Before)
	d.syncCropDraft()
	d.redraw()
}

func (d *desktop) syncCropDraft() {
	if !d.cropping() {
		return
	}
	m := d.model
	if d.crop.path == m.Selected && d.crop.part == m.PreviewPart {
		return
	}
	d.crop = cropDraft{active: true}
	if !m.OriginalReady() {
		return
	}
	s := m.SelectedSource()
	size := image.Pt(m.Preview.Input.Width, m.Preview.Input.Height)
	d.crop.path, d.crop.part, d.crop.size = s.Path, m.PreviewPart, size
	d.crop.box, d.crop.ratio = s.Crop.Resolve(size), s.Crop.Ratio
	d.crop.alignment = s.Crop.Alignment
}

func (d *desktop) endCrop(apply bool) {
	draft, m := d.crop, d.model
	if apply {
		if !d.cropReady() {
			return
		}
		region := crop.Region{}
		if draft.box != (image.Rectangle{Max: draft.size}) || draft.ratio != crop.Free {
			region = crop.Region{Box: draft.box, Reference: draft.size, Ratio: draft.ratio}
		}
		m.ApplyCrop(m.EditSelection, region)
	}
	d.crop = cropDraft{}
	m.SetMode(app.After)
	d.redraw()
}

func (d *desktop) cropReady() bool {
	m, s := d.model, &d.crop
	return d.cropping() && s.path == m.Selected && s.part == m.PreviewPart && !s.box.Empty() && m.OriginalReady()
}

func (d *desktop) dimensionsInspector(c *ui.Context) {
	ui.Column(c).Grow(1).Basis(0).MinHeight(0).Children(func() {
		ui.Column(c).Padding(16, 20, 0, 20).Shrink(0).Children(func() {
			tab := d.resizeTab
			action.Segmented(c.Key("dimensions-tab"), &tab, []string{tr(c, "crop.tab"), tr(c, "crop.resizeTab")}, action.SegmentedProps{Size: theme.Small, Block: true})
			if tab != d.resizeTab {
				d.resizeTab = tab
				if tab == 0 {
					d.beginCrop()
				} else {
					d.endCrop(false)
				}
				c.Invalidate()
			}
		})
		if d.resizeTab == 0 {
			d.cropInspector(c.Key("crop-panel"))
		} else {
			d.resizeInspector(c.Key("resize-panel"))
		}
	})
}

func (d *desktop) cropInspector(c *ui.Context) {
	m := d.model
	compactInspector(c, "", nil, func() {
		if !d.cropping() {
			if s := m.SelectedSource(); s.Crop.Active() && m.OriginalReady() {
				b := s.Crop.Resolve(image.Pt(m.Preview.Input.Width, m.Preview.Input.Height))
				detailRow(c, tr(c, "crop.resultSize"), sizeText(b.Dx(), b.Dy()))
			}
			ui.Row(c).Children(func() {
				if button(c, tr(c, "crop.edit"), lucide.Crop(), action.Default, !m.OriginalReady()) {
					d.beginCrop()
				}
			})
			return
		}
		draft, ready := &d.crop, d.cropReady()
		ui.Column(c).Gap(6).Children(func() {
			strong(c, tr(c, "crop.ratio")).FontSize(12)
			ratio := int(draft.ratio)
			action.Segmented(c.Key("crop-ratio"), &ratio, []string{tr(c, "crop.free"), "1:1", "4:5", "4:3", "16:9"}, action.SegmentedProps{Size: theme.Small, Block: true}).Disabled(!ready)
			if ready && crop.Ratio(ratio) != draft.ratio {
				draft.ratio = crop.Ratio(ratio)
				draft.gesture = cropGesture{}
				if draft.ratio != crop.Free {
					draft.box = crop.Fit(image.Rectangle{Max: draft.size}, draft.ratio)
				}
				c.Invalidate()
			}
		})
		line(c)
		detailRow(c, tr(c, "crop.resultSize"), sizeText(draft.box.Dx(), draft.box.Dy()))
		ui.Row(c).Children(func() {
			if button(c, tr(c, "crop.reset"), lucide.RotateCcw(), action.Default, !ready) {
				draft.box, draft.ratio = image.Rectangle{Max: draft.size}, crop.Free
				draft.gesture = cropGesture{}
				c.Invalidate()
			}
		})
	})
}

func (d *desktop) cropOverlay(c *ui.Context, w, h float32) {
	if !d.cropReady() {
		return
	}
	s := &d.crop
	sx, sy := w/float32(s.size.X), h/float32(s.size.Y)
	x, y := float32(s.box.Min.X)*sx, float32(s.box.Min.Y)*sy
	r, b := float32(s.box.Max.X)*sx, float32(s.box.Max.Y)*sy
	shade, white := ui.RGBA(0, 0, 0, .48), ui.RGB(255, 255, 255)
	mark := func(key string, x, y, w, h float32) ui.Element {
		return ui.Box(c.Key(key)).Absolute().Left(x).Top(y).Size(w, h).ZIndex(2).IgnoreInput(true)
	}
	mark("shade-top", 0, 0, w, y).Background(shade)
	mark("shade-bottom", 0, b, w, max(0, h-b)).Background(shade)
	mark("shade-left", 0, y, x, b-y).Background(shade)
	mark("shade-right", r, y, max(0, w-r), b-y).Background(shade)
	mark("border", x, y, r-x, b-y).Border(1.2, white)
	if r-x > 32 && b-y > 32 {
		for i := range 2 {
			f := float32(i+1) / 3
			mark(choose(i == 0, "grid-x1", "grid-x2"), x+(r-x)*f, y, .65, b-y).Background(ui.RGBA(255, 255, 255, .5))
			mark(choose(i == 0, "grid-y1", "grid-y2"), x, y+(b-y)*f, r-x, .65).Background(ui.RGBA(255, 255, 255, .5))
		}
	}
	move := ui.ButtonBase(c.Key("crop-move")).Absolute().Left(x).Top(y).Size(r-x, b-y).
		ZIndex(3).Label(tr(c, "crop.move")).Cursor(ui.CursorMove)
	d.dragCrop(c, move, -1, sx, sy)
	for i, pt := range [4][2]float32{{x, y}, {r, y}, {x, b}, {r, b}} {
		label := trf(c, "crop.handle", l10n.Args{"corner": i + 1})
		handle := ui.ButtonBase(c.Key(i)).Absolute().Left(pt[0]-9).Top(pt[1]-9).Size(18, 18).ZIndex(4).
			Center().Label(label).Cursor(choose(i == 0 || i == 3, ui.CursorResizeNWSE, ui.CursorResizeNESW))
		d.dragCrop(c, handle, i, sx, sy)
		handle.Children(func() {
			g := s.gesture
			side := choose(g.held && g.handle == i || handle.Hovered(), float32(10), 8)
			ui.Box(c).Size(side, side).Radius(2).Border(.6, ui.RGBA(0, 0, 0, .4)).Background(white).IgnoreInput(true)
		})
	}
}

func (d *desktop) dragCrop(c *ui.Context, element ui.Element, handle int, sx, sy float32) {
	s := &d.crop
	dx, dy, held := element.Dragged()
	g := &s.gesture
	if held {
		if !g.held || g.handle != handle {
			*g = cropGesture{held: true, handle: handle, start: s.box}
		}
		g.dx, g.dy = g.dx+float64(dx/sx), g.dy+float64(dy/sy)
		delta := image.Pt(int(math.Round(g.dx)), int(math.Round(g.dy)))
		if dx != 0 || dy != 0 {
			d.updateCropBox(c, g.start, delta, handle)
		}
		return
	}
	if g.held && g.handle == handle {
		*g = cropGesture{}
		c.Invalidate()
	}
	if !element.Focused() {
		return
	}
	for _, k := range []struct {
		key   ui.Key
		delta image.Point
	}{{ui.KeyLeft, image.Pt(-1, 0)}, {ui.KeyRight, image.Pt(1, 0)}, {ui.KeyUp, image.Pt(0, -1)}, {ui.KeyDown, image.Pt(0, 1)}} {
		if element.Shortcut(0, k.key) {
			delta := k.delta
			if handle >= 0 {
				units := s.ratio.Units()
				delta = delta.Mul(max(1, max(units.X, units.Y)*2))
			}
			d.updateCropBox(c, s.box, delta, handle)
		}
	}
}

func (d *desktop) updateCropBox(c *ui.Context, start image.Rectangle, delta image.Point, handle int) {
	s := &d.crop
	var box image.Rectangle
	if handle == -1 {
		box = crop.Move(start, s.size, delta)
	} else {
		box = crop.ResizeCorner(start, s.size, handle, delta, s.ratio)
	}
	if box != s.box {
		s.box = box
		c.Invalidate()
	}
}
