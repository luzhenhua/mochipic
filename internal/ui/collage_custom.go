package ui

import (
	"image"
	"math"

	"imageflow/internal/app"
	"imageflow/internal/files"
	"imageflow/internal/imaging"
	"imageflow/internal/l10n"

	"github.com/egoist/mygo/ui"
)

type collageGesture struct {
	start  imaging.CollagePhoto
	dx, dy float64
	group  app.HistoryGroup
}

func (d *desktop) drawCustomPhoto(c *ui.Context, item files.Source, size image.Point, order int, sx, sy float32) {
	s := &d.model.Collage
	p, ok := s.Options.Photo(item.Path)
	tile := d.collageTile(item)
	if !ok || tile.Image == nil {
		return
	}
	p = p.OnCanvas(size)
	placement := p.Placement(tile.Size)
	b := placement.Target
	z := 10 + order*12
	body := ui.ButtonBase(c.Key(item.Path)).Absolute().Left(float32(b.Min.X)*sx).Top(float32(b.Min.Y)*sy).Size(float32(b.Dx())*sx, float32(b.Dy())*sy).Label(baseName(c, item.Path)).ZIndex(z).Cursor(ui.CursorGrab).Disabled(s.Exporting)
	bmp := bitmap(body, tile.Image)
	thumb := tile.Image.Bounds().Size()
	kx, ky := float32(thumb.X)/float32(tile.Size.X), float32(thumb.Y)/float32(tile.Size.Y)
	src := placement.Source
	region := ui.Rect{X: float32(src.Min.X) * kx, Y: float32(src.Min.Y) * ky, W: float32(src.Dx()) * kx, H: float32(src.Dy()) * ky}
	body.Draw(func(painter *ui.Painter, r ui.Rect) {
		paint := func() { painter.ImageRegion(bmp, region, r, ui.ImageOptions{}) }
		radius := float32(p.EffectiveRadius()) * min(sx, sy)
		switch p.Corner {
		case imaging.CornerRound:
			painter.Clip(r, radius, paint)
		case imaging.CornerBevel:
			var clip ui.Path
			x, y, R, B := r.X, r.Y, r.X+r.W, r.Y+r.H
			clip.MoveTo(x+radius, y).LineTo(R-radius, y).LineTo(R, y+radius).LineTo(R, B-radius).LineTo(R-radius, B).LineTo(x+radius, B).LineTo(x, B-radius).LineTo(x, y+radius).Close()
			painter.ClipPath(&clip, paint)
		default:
			paint()
		}
	})
	menuLabel := tr(c, choose(d.collage.stretch == item.Path, "collage.finishStretch", "collage.stretch"))
	body.ContextMenu(func(menu *ui.Menu) {
		if menu.Item(menuLabel).Disabled(s.Exporting).Chosen() {
			d.collage.selected = item.Path
			d.collage.stretch = choose(d.collage.stretch == item.Path, "", item.Path)
			d.redraw()
		}
	})
	if body.Pressed() || body.Clicked() {
		if d.collage.selected != p.Path {
			d.collage.stretch = ""
		}
		d.collage.selected = p.Path
	}
	d.customGesture(c, body, p, -1, sx, sy)
	if d.collage.selected != item.Path || s.Exporting {
		return
	}
	x, y, r, bottom := float32(b.Min.X)*sx, float32(b.Min.Y)*sy, float32(b.Max.X)*sx, float32(b.Max.Y)*sy
	border := ui.Box(c.Key(struct {
		Path string
		Part int
	}{item.Path, -1})).Absolute().Left(x).Top(y).Size(r-x, bottom-y).Border(1, ui.RGBA(255, 255, 255, .85)).ZIndex(221).IgnoreInput(true)
	stretch := d.collage.stretch == item.Path
	points := [][2]float32{{x, y}, {r, y}, {x, bottom}, {r, bottom}}
	if stretch {
		points = append(points, [2]float32{x, (y + bottom) / 2}, [2]float32{r, (y + bottom) / 2}, [2]float32{(x + r) / 2, y}, [2]float32{(x + r) / 2, bottom})
	}
	dragging := false
	for part, pt := range points {
		cursor := choose(part == 0 || part == 3, ui.CursorResizeNWSE, ui.CursorResizeNESW)
		if part == 4 || part == 5 {
			cursor = ui.CursorResizeEW
		}
		if part >= 6 {
			cursor = ui.CursorResizeNS
		}
		label := trf(c, "crop.handle", l10n.Args{"corner": part + 1})
		if part >= 4 {
			label = tr(c, "collage.stretch")
		}
		handle := ui.ButtonBase(c.Key(struct {
			Path string
			Part int
		}{item.Path, part})).Absolute().Left(pt[0]-8).Top(pt[1]-8).Size(16, 16).Center().ZIndex(222).Label(label).Cursor(cursor)
		if d.customGesture(c, handle, p, part, sx, sy) {
			dragging = true
		}
		handle.Children(func() {
			ui.Box(c).Size(7, 7).Radius(1).Background(ui.RGB(255, 255, 255)).Border(1, ui.RGBA(0, 0, 0, .48)).IgnoreInput(true)
		})
	}
	if dragging && !stretch {
		border.Children(func() {
			for i := 1; i <= 2; i++ {
				f := float32(i) / 3
				ui.Box(c).Absolute().Left((r-x)*f).Top(0).Size(.7, bottom-y).Background(ui.RGBA(255, 255, 255, .6)).IgnoreInput(true)
				ui.Box(c).Absolute().Left(0).Top((bottom-y)*f).Size(r-x, .7).Background(ui.RGBA(255, 255, 255, .6)).IgnoreInput(true)
			}
		})
	}
}
func (d *desktop) customGesture(c *ui.Context, e ui.Element, p imaging.CollagePhoto, part int, sx, sy float32) bool {
	g := ui.Local(e, "collage-gesture", func() collageGesture { return collageGesture{} })
	dx, dy, held := e.Dragged()
	m := d.model
	if !held {
		if g.group != 0 {
			if e.PressCanceled() {
				m.CancelHistoryGroup(g.group)
			} else {
				m.FinishHistoryGroup(g.group)
			}
			*g = collageGesture{}
			c.Invalidate()
		}
		return false
	}
	if g.group == 0 {
		g.start = p
		g.group = m.NewHistoryGroup()
		d.collage.selected = p.Path
	}
	g.dx += float64(dx / sx)
	g.dy += float64(dy / sy)
	if dx != 0 || dy != 0 {
		o := m.Collage.Options
		next := g.start.Adjust(part, image.Pt(int(math.Round(g.dx)), int(math.Round(g.dy))), d.collage.stretch == p.Path)
		o.PutPhoto(next)
		m.EditCollageOptions(o, g.group)
		c.Invalidate()
	}
	return true
}
